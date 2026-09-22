import { useEffect } from "react";
import { Navigate, Outlet, useLocation } from "react-router";

import { clearLoginRedirectFlag, invalidateSession } from "@/lib/auth-session";
import { useAuthBootstrap } from "@/hooks/use-auth-bootstrap";
import { useCurrentUser } from "@/hooks/use-auth";
import { Spinner } from "@/components/ui/spinner";

const guestPathsAllowedWhileLoggedIn = new Set([
  "/confirm-email",
  "/reset-password",
  "/accept-invite",
  "/invite",
]);

export function ProtectedRoute() {
  const location = useLocation();
  const { sessionReady, hasSession } = useAuthBootstrap();
  const { isLoading, isError, data } = useCurrentUser({ enabled: sessionReady && hasSession });

  if (!sessionReady) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-8" />
      </div>
    );
  }

  if (!hasSession) {
    return <Navigate to="/login" replace />;
  }

  if (isLoading) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-8" />
      </div>
    );
  }

  if (isError) {
    invalidateSession();
    return <Navigate to="/login" replace />;
  }

  const mustChangePassword = data?.user?.mustChangePassword;
  const isChangePasswordRoute = location.pathname === "/user/change-password";

  if (mustChangePassword && !isChangePasswordRoute) {
    return <Navigate to="/user/change-password" replace />;
  }

  return <Outlet />;
}

export function GuestRoute() {
  const location = useLocation();
  const { sessionReady, hasSession } = useAuthBootstrap();
  const { data, isLoading, isFetched, isError } = useCurrentUser({
    enabled: sessionReady && hasSession,
  });

  useEffect(() => {
    clearLoginRedirectFlag();
  }, []);

  if (!sessionReady) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-8" />
      </div>
    );
  }

  if (!hasSession) {
    return <Outlet />;
  }

  if (isLoading || !isFetched) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-8" />
      </div>
    );
  }

  if (data && !isError && !guestPathsAllowedWhileLoggedIn.has(location.pathname)) {
    return <Navigate to="/" replace />;
  }

  if (isError) {
    invalidateSession();
  }

  return <Outlet />;
}

export function AdminRoute() {
  const { data, isLoading, isError } = useCurrentUser();

  if (isLoading) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-8" />
      </div>
    );
  }

  if (isError || data?.user?.role !== "admin") {
    return <Navigate to="/" replace />;
  }

  return <Outlet />;
}
