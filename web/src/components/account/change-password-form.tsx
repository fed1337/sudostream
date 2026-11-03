import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { getApiAuthMeQueryKey } from "@/client/@tanstack/react-query.gen";
import { postApiAuthChangePassword } from "@/client/sdk.gen";
import { TotpCodeInput } from "@/components/totp-code-input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";
import { completeLoginSession, useCurrentUser } from "@/hooks/use-auth";
import {
  ApiErrorMessage,
  extractApiErrorMessage,
  isApiErrorMessage,
  translateApiError,
} from "@/lib/api-error";

type Step = "details" | "totp";

/** Signed-in user page form: requires current password; 2FA as a second step when enabled. */
export function ChangePasswordForm() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: me } = useCurrentUser();
  const has2FA = Boolean(me?.user?.has2FA);
  const [step, setStep] = useState<Step>("details");

  const detailsSchema = useMemo(
    () =>
      z
        .object({
          currentPassword: z.string().min(1, { error: t("validation.required") }),
          newPassword: z.string().min(8, { error: t("validation.passwordMin") }),
          confirmPassword: z.string().min(1, { error: t("validation.required") }),
        })
        .refine((data) => data.newPassword === data.confirmPassword, {
          error: t("validation.passwordMismatch"),
          path: ["confirmPassword"],
        }),
    [t],
  );
  const totpSchema = useMemo(
    () =>
      z.object({
        totp: z.string().length(6, { error: t("validation.totpCode") }),
      }),
    [t],
  );

  type DetailsValues = z.infer<typeof detailsSchema>;
  type TotpValues = z.infer<typeof totpSchema>;

  const detailsForm = useForm<DetailsValues>({
    resolver: zodResolver(detailsSchema),
    defaultValues: { currentPassword: "", newPassword: "", confirmPassword: "" },
  });
  const totpForm = useForm<TotpValues>({
    resolver: zodResolver(totpSchema),
    defaultValues: { totp: "" },
  });

  const mutation = useMutation({
    mutationFn: async (input: { currentPassword: string; newPassword: string; totp?: string }) => {
      const response = await postApiAuthChangePassword({
        body: {
          currentPassword: input.currentPassword,
          newPassword: input.newPassword,
          totp: input.totp || undefined,
        },
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
      if (accessToken) {
        completeLoginSession(queryClient, accessToken, user);
      } else {
        void queryClient.invalidateQueries({ queryKey: getApiAuthMeQueryKey() });
      }
      detailsForm.reset({ currentPassword: "", newPassword: "", confirmPassword: "" });
      totpForm.reset({ totp: "" });
      setStep("details");
    },
  });

  const isInvalidCurrentPassword = isApiErrorMessage(
    mutation.error,
    ApiErrorMessage.invalidCurrentPassword,
  );
  const isInvalidTotp = isApiErrorMessage(mutation.error, ApiErrorMessage.invalidTwoFactorCode);
  const showApiError =
    mutation.isError && !isApiErrorMessage(mutation.error, ApiErrorMessage.twoFactorRequired);
  const translatedError = translateApiError(mutation.error, t, {
    fallbackKey: "account.changePasswordFailed",
  });

  const submitDetails = (values: DetailsValues) => {
    mutation.reset();
    if (has2FA) {
      setStep("totp");
      totpForm.reset({ totp: "" });
      return;
    }
    mutation.mutate(
      { currentPassword: values.currentPassword, newPassword: values.newPassword },
      {
        onError: (error) => {
          if (isApiErrorMessage(error, ApiErrorMessage.twoFactorRequired)) {
            setStep("totp");
            totpForm.reset({ totp: "" });
            mutation.reset();
          }
        },
      },
    );
  };

  const submitTotp = (values: TotpValues) => {
    const details = detailsForm.getValues();
    mutation.reset();
    mutation.mutate({
      currentPassword: details.currentPassword,
      newPassword: details.newPassword,
      totp: values.totp,
    });
  };

  const detailsErrors = detailsForm.formState.errors;
  const totpErrors = totpForm.formState.errors;

  return (
    <Card className="h-full w-full">
      <CardHeader>
        <CardTitle className="text-lg">{t("account.changePassword")}</CardTitle>
        {step === "totp" ? (
          <CardDescription>{t("account.changePasswordTotpDescription")}</CardDescription>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-4">
        {mutation.isSuccess ? (
          <Alert variant="success">
            <AlertTitle>{t("account.changePasswordSuccess")}</AlertTitle>
          </Alert>
        ) : null}

        {showApiError ? (
          <Alert variant="destructive">
            <AlertTitle>
              {isInvalidCurrentPassword
                ? t("account.invalidCurrentPassword")
                : isInvalidTotp
                  ? t("account.invalidTotp")
                  : t("account.changePasswordFailed")}
            </AlertTitle>
            {!isInvalidCurrentPassword && !isInvalidTotp ? (
              <AlertDescription>{translatedError}</AlertDescription>
            ) : null}
          </Alert>
        ) : null}

        {step === "details" && !mutation.isSuccess ? (
          <form
            className="flex flex-1 flex-col gap-4"
            onSubmit={detailsForm.handleSubmit(submitDetails)}
          >
            <FieldGroup>
              <Field data-invalid={Boolean(detailsErrors.currentPassword)}>
                <FieldLabel htmlFor="currentPassword">{t("account.currentPassword")}</FieldLabel>
                <PasswordInput
                  id="currentPassword"
                  autoComplete="current-password"
                  aria-invalid={Boolean(detailsErrors.currentPassword)}
                  {...detailsForm.register("currentPassword")}
                />
                <FieldError errors={[detailsErrors.currentPassword]} />
              </Field>
              <Field data-invalid={Boolean(detailsErrors.newPassword)}>
                <FieldLabel htmlFor="newPassword">{t("auth.newPassword")}</FieldLabel>
                <PasswordInput
                  id="newPassword"
                  autoComplete="new-password"
                  aria-invalid={Boolean(detailsErrors.newPassword)}
                  {...detailsForm.register("newPassword")}
                />
                <FieldError errors={[detailsErrors.newPassword]} />
              </Field>
              <Field data-invalid={Boolean(detailsErrors.confirmPassword)}>
                <FieldLabel htmlFor="confirmPassword">{t("auth.confirmPassword")}</FieldLabel>
                <PasswordInput
                  id="confirmPassword"
                  autoComplete="new-password"
                  aria-invalid={Boolean(detailsErrors.confirmPassword)}
                  {...detailsForm.register("confirmPassword")}
                />
                <FieldError errors={[detailsErrors.confirmPassword]} />
              </Field>
            </FieldGroup>
            <Button type="submit" disabled={mutation.isPending} className="mt-auto w-full">
              {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {has2FA
                ? t("account.continueToTwoFactor")
                : mutation.isPending
                  ? t("account.changingPassword")
                  : t("account.changePassword")}
            </Button>
          </form>
        ) : null}

        {step === "totp" && !mutation.isSuccess ? (
          <form className="flex flex-1 flex-col gap-4" onSubmit={totpForm.handleSubmit(submitTotp)}>
            <FieldGroup>
              <Field data-invalid={Boolean(totpErrors.totp)}>
                <FieldLabel htmlFor="change-password-totp">{t("account.totpCode")}</FieldLabel>
                <TotpCodeInput control={totpForm.control} name="totp" id="change-password-totp" />
                <FieldError errors={[totpErrors.totp]} />
              </Field>
            </FieldGroup>
            <div className="mt-auto flex flex-col gap-2">
              <Button type="submit" disabled={mutation.isPending} className="w-full">
                {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {mutation.isPending ? t("account.changingPassword") : t("account.changePassword")}
              </Button>
              <Button
                type="button"
                variant="outline"
                className="w-full"
                disabled={mutation.isPending}
                onClick={() => {
                  mutation.reset();
                  setStep("details");
                  totpForm.reset({ totp: "" });
                }}
              >
                {t("common.back")}
              </Button>
            </div>
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}
