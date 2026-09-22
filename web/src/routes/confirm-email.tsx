import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { getApiAdminUsersQueryKey, getApiAuthMeQueryKey } from "@/client/@tanstack/react-query.gen";
import { getApiAuthConfirmEmail } from "@/client/sdk.gen";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useCurrentUser } from "@/hooks/use-auth";
import { extractApiErrorMessage } from "@/lib/api-error";
import { getAccessToken } from "@/lib/auth-storage";

type ConfirmState = "idle" | "loading" | "success" | "error";

type ConfirmResult = { ok: true } | { ok: false; error: string };

/** Survives React Strict Mode remounts so a one-time token is only confirmed once. */
const confirmAttempts = new Map<string, Promise<ConfirmResult>>();

export default function ConfirmEmailPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";
  const hasSession = Boolean(getAccessToken());
  const { data: me } = useCurrentUser({ enabled: hasSession });

  const [state, setState] = useState<ConfirmState>(token ? "loading" : "idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      return;
    }

    let attempt = confirmAttempts.get(token);
    if (!attempt) {
      attempt = (async (): Promise<ConfirmResult> => {
        const response = await getApiAuthConfirmEmail({ query: { token } });
        if (response.error) {
          return {
            ok: false,
            error: extractApiErrorMessage(response.error) ?? t("auth.confirmEmailFailed"),
          };
        }
        return { ok: true };
      })();
      confirmAttempts.set(token, attempt);
    }

    void attempt.then((result) => {
      if (result.ok) {
        setState("success");
        void queryClient.invalidateQueries({ queryKey: getApiAuthMeQueryKey() });
        void queryClient.invalidateQueries({ queryKey: getApiAdminUsersQueryKey() });
        return;
      }
      setErrorMessage(result.error);
      setState("error");
    });
  }, [token, queryClient, t]);

  if (!token) {
    return (
      <GuestAuthLayout title={t("auth.confirmEmailTitle")}>
        <Alert variant="destructive">
          <AlertTitle>{t("auth.invalidToken")}</AlertTitle>
          <AlertDescription>{t("auth.invalidTokenDescription")}</AlertDescription>
        </Alert>
        <Button variant="outline" asChild className="mt-4 w-full">
          <Link to={hasSession ? "/" : "/login"}>
            {hasSession ? t("nav.home") : t("auth.backToLogin")}
          </Link>
        </Button>
      </GuestAuthLayout>
    );
  }

  if (state === "loading" || state === "idle") {
    return (
      <GuestAuthLayout title={t("auth.confirmEmailTitle")}>
        <div className="flex justify-center py-8">
          <Spinner className="size-8" />
        </div>
      </GuestAuthLayout>
    );
  }

  if (state === "error") {
    return (
      <GuestAuthLayout title={t("auth.confirmEmailTitle")}>
        <Alert variant="destructive">
          <AlertTitle>{t("auth.confirmEmailFailed")}</AlertTitle>
          <AlertDescription>{errorMessage ?? t("auth.confirmEmailFailed")}</AlertDescription>
        </Alert>
        <Button variant="outline" asChild className="mt-4 w-full">
          <Link to={hasSession ? "/" : "/login"}>
            {hasSession ? t("nav.home") : t("auth.backToLogin")}
          </Link>
        </Button>
      </GuestAuthLayout>
    );
  }

  return (
    <GuestAuthLayout
      title={t("auth.confirmEmailTitle")}
      description={t("auth.confirmEmailSuccess")}
    >
      <Alert variant="success">
        <AlertTitle>{t("auth.emailConfirmed")}</AlertTitle>
        <AlertDescription>
          {hasSession
            ? t("auth.confirmEmailSuccessSignedIn", { email: me?.user?.email ?? "" })
            : t("auth.confirmEmailSuccessDescription")}
        </AlertDescription>
      </Alert>
      <Button asChild className="mt-4 w-full">
        <Link to={hasSession ? "/" : "/login"}>{hasSession ? t("nav.home") : t("auth.login")}</Link>
      </Button>
    </GuestAuthLayout>
  );
}
