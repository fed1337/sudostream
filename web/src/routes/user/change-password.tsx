import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { z } from "zod";

import { postApiAuthChangePassword } from "@/client/sdk.gen";
import { NewPasswordForm } from "@/components/auth/new-password-form";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { TotpCodeInput } from "@/components/totp-code-input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { completeLoginSession, useCurrentUser } from "@/hooks/use-auth";
import { extractApiErrorMessage } from "@/lib/api-error";

const twoFactorRequiredError = "two factor required";
const invalidTwoFactorError = "invalid two factor code";

type Step = "password" | "totp";

/** Forced first-login password change — same new-password UI as reset-password. */
export default function UserChangePasswordPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data: me } = useCurrentUser();
  const has2FA = Boolean(me?.user?.has2FA);
  const [step, setStep] = useState<Step>("password");
  const [pendingPassword, setPendingPassword] = useState("");

  const totpSchema = useMemo(
    () =>
      z.object({
        totp: z.string().length(6, { error: t("validation.totpCode") }),
      }),
    [t],
  );
  type TotpValues = z.infer<typeof totpSchema>;

  const totpForm = useForm<TotpValues>({
    resolver: zodResolver(totpSchema),
    defaultValues: { totp: "" },
  });

  const mutation = useMutation({
    mutationFn: async (input: { password: string; totp?: string }) => {
      const response = await postApiAuthChangePassword({
        body: { newPassword: input.password, totp: input.totp || undefined },
      });

      if (response.error) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("account.changePasswordFailed"),
        );
      }

      return response.data;
    },
    onSuccess: (data) => {
      const accessToken =
        data && typeof data === "object" && "accessToken" in data
          ? (data.accessToken as string | undefined)
          : undefined;
      const user =
        data && typeof data === "object" && "user" in data
          ? (data.user as
              | {
                  id?: string;
                  email?: string;
                  role?: string;
                  mustChangePassword?: boolean;
                  has2FA?: boolean;
                }
              | undefined)
          : undefined;

      // Seed /me before navigate so ProtectedRoute does not bounce back here.
      if (accessToken) {
        completeLoginSession(queryClient, accessToken, user);
      }

      void navigate("/", { replace: true });
    },
  });

  const errorMessage = mutation.error instanceof Error ? mutation.error.message : t("common.error");
  const isInvalidTotp = errorMessage === invalidTwoFactorError;

  return (
    <GuestAuthLayout
      title={t("account.changePassword")}
      description={
        step === "totp"
          ? t("account.changePasswordTotpDescription")
          : t("account.changePasswordForced")
      }
    >
      {step === "password" ? (
        <NewPasswordForm
          submitLabel={has2FA ? t("account.continueToTwoFactor") : t("auth.resetPassword")}
          pendingLabel={t("auth.resetting")}
          isPending={mutation.isPending}
          errorMessage={
            mutation.isError && errorMessage !== twoFactorRequiredError ? errorMessage : null
          }
          onSubmit={(values) => {
            mutation.reset();
            if (has2FA) {
              setPendingPassword(values.password);
              setStep("totp");
              totpForm.reset({ totp: "" });
              return;
            }
            mutation.mutate(
              { password: values.password },
              {
                onError: (error) => {
                  if (error instanceof Error && error.message === twoFactorRequiredError) {
                    setPendingPassword(values.password);
                    setStep("totp");
                    totpForm.reset({ totp: "" });
                    mutation.reset();
                  }
                },
              },
            );
          }}
        />
      ) : (
        <form
          className="flex flex-col gap-4"
          onSubmit={totpForm.handleSubmit((values) => {
            mutation.reset();
            mutation.mutate({ password: pendingPassword, totp: values.totp });
          })}
        >
          {mutation.isError && errorMessage !== twoFactorRequiredError ? (
            <Alert variant="destructive">
              <AlertTitle>
                {isInvalidTotp ? t("account.invalidTotp") : t("account.changePasswordFailed")}
              </AlertTitle>
              {!isInvalidTotp ? <AlertDescription>{errorMessage}</AlertDescription> : null}
            </Alert>
          ) : null}

          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="forced-change-totp">{t("account.totpCode")}</FieldLabel>
              <TotpCodeInput control={totpForm.control} name="totp" id="forced-change-totp" />
            </Field>
          </FieldGroup>
          <Button type="submit" disabled={mutation.isPending} className="w-full">
            {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
            {mutation.isPending ? t("auth.resetting") : t("auth.resetPassword")}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="w-full"
            disabled={mutation.isPending}
            onClick={() => {
              mutation.reset();
              setStep("password");
              totpForm.reset({ totp: "" });
            }}
          >
            {t("common.back")}
          </Button>
        </form>
      )}
    </GuestAuthLayout>
  );
}
