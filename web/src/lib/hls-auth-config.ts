import { getAccessToken } from "@/lib/auth-storage";
import { redirectToLogin } from "@/lib/auth-session";
import { refreshAccessToken } from "@/lib/session-restore";

type HlsAuthConfigOptions = {
  onAuthFailure?: () => void;
};

let recoverPromise: Promise<boolean> | null = null;

function stripAccessTokenParam(url: string): string {
  const queryIndex = url.indexOf("?");
  if (queryIndex < 0) {
    return url;
  }

  const params = new URLSearchParams(url.slice(queryIndex + 1));
  if (!params.has("access_token")) {
    return url;
  }

  params.delete("access_token");
  const base = url.slice(0, queryIndex);
  const rest = params.toString();

  return rest ? `${base}?${rest}` : base;
}

function applyBearerToken(xhr: XMLHttpRequest): void {
  const token = getAccessToken();
  if (!token) {
    return;
  }

  xhr.setRequestHeader("Authorization", `Bearer ${token}`);
}

async function recoverFromUnauthorized(onAuthFailure?: () => void): Promise<boolean> {
  if (!recoverPromise) {
    recoverPromise = (async () => {
      const token = await refreshAccessToken();
      if (!token) {
        redirectToLogin();

        return false;
      }

      onAuthFailure?.();

      return true;
    })().finally(() => {
      recoverPromise = null;
    });
  }

  return recoverPromise;
}

/** HLS.js xhr: Bearer auth on every request; refresh once on 401 before remount or login redirect. */
export function createHlsAuthXhrSetup(onAuthFailure?: () => void) {
  return (xhr: XMLHttpRequest, url: string) => {
    const authedUrl = stripAccessTokenParam(url);
    if (authedUrl !== url) {
      xhr.open("GET", authedUrl, true);
    }

    xhr.addEventListener(
      "loadend",
      () => {
        if (xhr.status !== 401) {
          return;
        }

        void recoverFromUnauthorized(onAuthFailure);
      },
      { once: true },
    );

    applyBearerToken(xhr);
  };
}

export function createHlsAuthConfig(options: HlsAuthConfigOptions = {}) {
  return {
    xhrSetup: createHlsAuthXhrSetup(options.onAuthFailure),
  };
}
