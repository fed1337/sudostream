import ky from "ky";

import { getAccessToken, setAccessToken } from "@/lib/auth-storage";

const host = import.meta.env.VITE_HOST ?? "";

let rotatePromise: Promise<string | null> | null = null;

/** Bare ky — no Hey API interceptors — so refresh cannot recurse into 401 handling. */
async function postRefresh(): Promise<string | null> {
  try {
    const refreshRes = await ky.post(`${host}/api/auth/refresh`, {
      credentials: "include",
      throwHttpErrors: false,
      retry: 0,
    });

    if (!refreshRes.ok) {
      return null;
    }

    const data = (await refreshRes.json()) as { accessToken?: string };
    if (!data.accessToken) {
      return null;
    }

    setAccessToken(data.accessToken);
    return data.accessToken;
  } catch {
    return null;
  }
}

/** Returns true when an access token is available (existing or restored from refresh cookie). */
export async function ensureAccessTokenFromRefreshCookie(): Promise<boolean> {
  if (getAccessToken()) {
    return true;
  }

  return (await rotateAccessToken()) !== null;
}

/** Rotates the access token via the httpOnly refresh cookie (deduped across callers). */
export async function rotateAccessToken(): Promise<string | null> {
  if (!rotatePromise) {
    rotatePromise = postRefresh().finally(() => {
      rotatePromise = null;
    });
  }

  return rotatePromise;
}

/** Attempts one refresh before giving up; used by the API client on 401 responses. */
export async function refreshAccessToken(): Promise<string | null> {
  return rotateAccessToken();
}
