import { clearAccessToken } from "@/lib/auth-storage";

const LOGIN_REDIRECT_FLAG = "auth.loginRedirect";

function isGuestAuthPath(pathname: string): boolean {
  return (
    pathname === "/login" ||
    pathname.startsWith("/login/") ||
    pathname === "/forgot-password" ||
    pathname === "/reset-password" ||
    pathname === "/confirm-email" ||
    pathname === "/accept-invite" ||
    pathname === "/invite"
  );
}

/** Clears the in-memory access token. */
export function invalidateSession(): void {
  clearAccessToken();
}

/** Clears the one-shot login redirect guard (call after reaching guest auth routes). */
export function clearLoginRedirectFlag(): void {
  sessionStorage.removeItem(LOGIN_REDIRECT_FLAG);
}

/**
 * Ends the session and navigates to login once.
 * Skips redirect when already on a guest auth route.
 */
export function redirectToLogin(): void {
  invalidateSession();

  const { pathname } = window.location;
  if (isGuestAuthPath(pathname)) {
    return;
  }

  if (sessionStorage.getItem(LOGIN_REDIRECT_FLAG) === "1") {
    return;
  }

  sessionStorage.setItem(LOGIN_REDIRECT_FLAG, "1");
  window.location.replace("/login");
}
