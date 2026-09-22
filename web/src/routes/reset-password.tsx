import { useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { postApiAuthResetPassword } from "@/client/sdk.gen";
import { NewPasswordForm } from "@/components/auth/new-password-form";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { extractApiErrorMessage } from "@/lib/api-error";

export default function ResetPasswordPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const mutation = useMutation({
    mutationFn: async (password: string) => {
      const response = await postApiAuthResetPassword({
        body: { token, password },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.resetPasswordFailed"));
      }
    },
  });

  if (!token) {
    return (
      <GuestAuthLayout title={t("auth.resetPasswordTitle")}>
        <Alert variant="destructive">
          <AlertTitle>{t("auth.invalidToken")}</AlertTitle>
          <AlertDescription>{t("auth.invalidTokenDescription")}</AlertDescription>
        </Alert>
        <Button variant="outline" asChild className="mt-4 w-full">
          <Link to="/login">{t("auth.backToLogin")}</Link>
        </Button>
      </GuestAuthLayout>
    );
  }

  if (mutation.isSuccess) {
    return (
      <GuestAuthLayout
        title={t("auth.resetPasswordTitle")}
        description={t("auth.resetPasswordSuccess")}
      >
        <Button asChild className="w-full">
          <Link to="/login">{t("auth.login")}</Link>
        </Button>
      </GuestAuthLayout>
    );
  }

  return (
    <GuestAuthLayout
      title={t("auth.resetPasswordTitle")}
      description={t("auth.resetPasswordDescription")}
    >
      <NewPasswordForm
        submitLabel={t("auth.resetPassword")}
        pendingLabel={t("auth.resetting")}
        isPending={mutation.isPending}
        errorMessage={
          mutation.isError
            ? mutation.error instanceof Error
              ? mutation.error.message
              : t("common.error")
            : null
        }
        onSubmit={(values) => {
          mutation.reset();
          mutation.mutate(values.password);
        }}
      />
    </GuestAuthLayout>
  );
}
