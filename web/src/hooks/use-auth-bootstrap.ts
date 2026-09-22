import { useEffect, useState } from "react";

import { isAuthenticated } from "@/lib/auth-storage";
import { ensureAccessTokenFromRefreshCookie } from "@/lib/session-restore";

/** Restores access token from refresh cookie before auth guards decide routing. */
export function useAuthBootstrap() {
  const [sessionReady, setSessionReady] = useState(isAuthenticated());
  const [hasSession, setHasSession] = useState(isAuthenticated());

  useEffect(() => {
    if (isAuthenticated()) {
      return;
    }

    let active = true;

    void ensureAccessTokenFromRefreshCookie().then((restored) => {
      if (!active) {
        return;
      }

      setHasSession(restored);
      setSessionReady(true);
    });

    return () => {
      active = false;
    };
  }, []);

  return { sessionReady, hasSession };
}
