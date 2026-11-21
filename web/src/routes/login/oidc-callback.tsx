import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { postApiAuthRefresh } from "@/client/sdk.gen";
import { GuestShell } from "@/components/guest-shell";
import { clearLoginRedirectFlag } from "@/lib/auth-session";
import { completeLoginSession } from "@/hooks/use-auth";
import { Spinner } from "@/components/ui/spinner";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function LoginOidcCallbackPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    void (async () => {
      const response = await postApiAuthRefresh();
      if (cancelled) {
        return;
      }
      if (response.error || !response.data?.accessToken) {
        setError(t("auth.oidcCallbackFailed"));
        return;
      }
      completeLoginSession(queryClient, response.data.accessToken, response.data.user);
      clearLoginRedirectFlag();
      void navigate("/", { replace: true });
    })();

    return () => {
      cancelled = true;
    };
  }, [navigate, queryClient, t]);

  return (
    <GuestShell>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">{t("auth.oidcCompleting")}</CardTitle>
          <CardDescription>{error ?? t("auth.oidcCompletingDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex justify-center py-6">{error ? null : <Spinner />}</CardContent>
      </Card>
    </GuestShell>
  );
}
