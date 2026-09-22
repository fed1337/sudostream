import ky from "ky";

import { client } from "@/client/client.gen";
import { getAccessToken } from "@/lib/auth-storage";
import { redirectToLogin } from "@/lib/auth-session";
import { refreshAccessToken } from "@/lib/session-restore";

const AUTH_RETRY_HEADER = "X-SudoStream-Auth-Retry";
const AUTH_REQUIRED_ERROR = "authentication required";

/** Public auth routes that must not trigger refresh-or-login. */
function isAuthExemptUrl(url: string): boolean {
  return (
    url.includes("/api/auth/refresh") ||
    url.includes("/api/auth/login") ||
    url.includes("/api/auth/logout")
  );
}

function shouldAttemptRefresh(request: Request, response: Response): boolean {
  if (response.status !== 401 && response.status !== 403) {
    return false;
  }
  if (request.headers.get(AUTH_RETRY_HEADER) === "1") {
    return false;
  }
  return !isAuthExemptUrl(request.url);
}

async function isSessionAuthFailure(response: Response): Promise<boolean> {
  try {
    const payload: unknown = await response.clone().json();
    if (!payload || typeof payload !== "object" || !("error" in payload)) {
      return true;
    }
    return payload.error === AUTH_REQUIRED_ERROR;
  } catch {
    return true;
  }
}

client.setConfig({
  // Disable ky's built-in retries; auth refresh owns 401 handling.
  retry: 0,
});

client.interceptors.request.use(async (request) => {
  if (isAuthExemptUrl(request.url)) {
    return request;
  }

  let token = getAccessToken();
  if (!token) {
    token = await refreshAccessToken();
    if (!token) {
      redirectToLogin();
      return request;
    }
  }

  request.headers.set("Authorization", `Bearer ${token}`);
  return request;
});

client.interceptors.response.use(async (response, request) => {
  if (!shouldAttemptRefresh(request, response)) {
    return response;
  }
  if (!(await isSessionAuthFailure(response))) {
    return response;
  }

  const newToken = await refreshAccessToken();
  if (!newToken) {
    redirectToLogin();
    return response;
  }

  const headers = new Headers(request.headers);
  headers.set("Authorization", `Bearer ${newToken}`);
  headers.set(AUTH_RETRY_HEADER, "1");

  // Re-issue with a bare ky instance so interceptors do not recurse.
  const retryResponse = await ky(request.url, {
    method: request.method,
    headers,
    body:
      request.method === "GET" || request.method === "HEAD"
        ? undefined
        : await request.clone().arrayBuffer(),
    credentials: "include",
    throwHttpErrors: false,
    retry: 0,
  });

  if (retryResponse.status === 401) {
    redirectToLogin();
  }

  return retryResponse;
});

export { client };
