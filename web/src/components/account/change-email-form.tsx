import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { postApiAuthChangeEmail } from "@/client/sdk.gen";
import { TotpCodeInput } from "@/components/totp-code-input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";
import { useCurrentUser } from "@/hooks/use-auth";
import {
  ApiErrorMessage,
  extractApiErrorMessage,
  isApiErrorMessage,
  translateApiError,
} from "@/lib/api-error";

type Step = "details" | "totp";

export function ChangeEmailForm() {
  const { t } = useTranslation();
  const { data: me } = useCurrentUser();
  const has2FA = Boolean(me?.user?.has2FA);
  const [step, setStep] = useState<Step>("details");

  const detailsSchema = useMemo(
    () =>
      z.object({
        newEmail: z.email({ error: t("validation.email") }),
        password: z.string().min(1, { error: t("validation.required") }),
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
    defaultValues: { newEmail: "", password: "" },
  });
  const totpForm = useForm<TotpValues>({
    resolver: zodResolver(totpSchema),
    defaultValues: { totp: "" },
  });

  const mutation = useMutation({
    mutationFn: async (input: { newEmail: string; password: string; totp?: string }) => {
      const response = await postApiAuthChangeEmail({
        body: {
          newEmail: input.newEmail,
          password: input.password,
          totp: input.totp || undefined,
        },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("account.changeEmailFailed"));
      }
    },
    onSuccess: () => {
      detailsForm.reset({ newEmail: "", password: "" });
      totpForm.reset({ totp: "" });
      setStep("details");
    },
  });

  const isEmailInUse = isApiErrorMessage(mutation.error, ApiErrorMessage.emailAlreadyUsed);
  const isInvalidPassword = isApiErrorMessage(
    mutation.error,
    ApiErrorMessage.invalidCurrentPassword,
  );
  const isInvalidTotp = isApiErrorMessage(mutation.error, ApiErrorMessage.invalidTwoFactorCode);
  const showApiError =
    mutation.isError && !isApiErrorMessage(mutation.error, ApiErrorMessage.twoFactorRequired);
  const translatedError = translateApiError(mutation.error, t, {
    fallbackKey: "account.changeEmailFailed",
  });

  const submitDetails = (values: DetailsValues) => {
    mutation.reset();
    if (has2FA) {
      setStep("totp");
      totpForm.reset({ totp: "" });
      return;
    }
    // No 2FA on profile: submit now. If the server still requires 2FA, advance to the code step.
    mutation.mutate(
      { newEmail: values.newEmail, password: values.password },
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
      newEmail: details.newEmail,
      password: details.password,
      totp: values.totp,
    });
  };

  const detailsErrors = detailsForm.formState.errors;
  const totpErrors = totpForm.formState.errors;

  return (
    <Card className="h-full w-full">
      <CardHeader>
        <CardTitle className="text-lg">{t("account.changeEmail")}</CardTitle>
        <CardDescription>
          {step === "totp"
            ? t("account.changeEmailTotpDescription")
            : t("account.changeEmailDescription")}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-4">
        {mutation.isSuccess ? (
          <Alert variant="success">
            <AlertTitle>{t("account.changeEmailSent")}</AlertTitle>
            <AlertDescription>{t("account.changeEmailSentDescription")}</AlertDescription>
          </Alert>
        ) : null}

        {showApiError ? (
          <Alert variant="destructive">
            <AlertTitle>
              {isEmailInUse
                ? t("account.emailAlreadyUsed")
                : isInvalidPassword
                  ? t("account.invalidCurrentPassword")
                  : isInvalidTotp
                    ? t("account.invalidTotp")
                    : t("account.changeEmailFailed")}
            </AlertTitle>
            {!isEmailInUse && !isInvalidPassword && !isInvalidTotp ? (
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
              <Field data-invalid={Boolean(detailsErrors.newEmail)}>
                <FieldLabel htmlFor="change-email-new">{t("account.newEmail")}</FieldLabel>
                <Input
                  id="change-email-new"
                  type="email"
                  aria-invalid={Boolean(detailsErrors.newEmail)}
                  {...detailsForm.register("newEmail")}
                />
                <FieldError errors={[detailsErrors.newEmail]} />
              </Field>
              <Field data-invalid={Boolean(detailsErrors.password)}>
                <FieldLabel htmlFor="change-email-password">
                  {t("account.currentPassword")}
                </FieldLabel>
                <PasswordInput
                  id="change-email-password"
                  autoComplete="current-password"
                  aria-invalid={Boolean(detailsErrors.password)}
                  {...detailsForm.register("password")}
                />
                <FieldError errors={[detailsErrors.password]} />
              </Field>
            </FieldGroup>
            <Button type="submit" disabled={mutation.isPending} className="mt-auto w-full">
              {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {has2FA ? t("account.continueToTwoFactor") : t("account.requestEmailChange")}
            </Button>
          </form>
        ) : null}

        {step === "totp" && !mutation.isSuccess ? (
          <form className="flex flex-1 flex-col gap-4" onSubmit={totpForm.handleSubmit(submitTotp)}>
            <FieldGroup>
              <Field data-invalid={Boolean(totpErrors.totp)}>
                <FieldLabel htmlFor="change-email-totp">{t("account.totpCode")}</FieldLabel>
                <TotpCodeInput control={totpForm.control} name="totp" id="change-email-totp" />
                <FieldError errors={[totpErrors.totp]} />
              </Field>
            </FieldGroup>
            <div className="mt-auto flex flex-col gap-2">
              <Button type="submit" disabled={mutation.isPending} className="w-full">
                {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {t("account.requestEmailChange")}
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
