import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { getApiAuthMeQueryKey } from "@/client/@tanstack/react-query.gen";
import { deleteApiAuth2Fa, postApiAuth2FaConfirm, postApiAuth2FaSetup } from "@/client/sdk.gen";
import { TotpCodeInput } from "@/components/totp-code-input";
import { TotpQrCode } from "@/components/totp-qr-code";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";
import { useCurrentUser } from "@/hooks/use-auth";
import { extractApiErrorMessage } from "@/lib/api-error";

export function TwoFactorSection() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: me } = useCurrentUser();
  const has2FA = Boolean(me?.user?.has2FA);

  const [setupSecret, setSetupSecret] = useState<string | null>(null);
  const [setupOtpauthURL, setSetupOtpauthURL] = useState<string | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[] | null>(null);
  const [enabled, setEnabled] = useState(has2FA);
  const [prevHas2FA, setPrevHas2FA] = useState(has2FA);
  const [showDisable, setShowDisable] = useState(false);

  if (has2FA !== prevHas2FA) {
    setPrevHas2FA(has2FA);
    setEnabled(has2FA);
  }

  const confirmSchema = useMemo(
    () =>
      z.object({
        code: z.string().length(6, { error: t("validation.totpCode") }),
      }),
    [t],
  );

  const disableSchema = useMemo(
    () =>
      z.object({
        password: z.string().min(1, { error: t("validation.required") }),
        code: z.string().length(6, { error: t("validation.totpCode") }),
      }),
    [t],
  );

  type ConfirmValues = z.infer<typeof confirmSchema>;
  type DisableValues = z.infer<typeof disableSchema>;

  const confirmForm = useForm<ConfirmValues>({
    resolver: zodResolver(confirmSchema),
    defaultValues: { code: "" },
  });

  const disableForm = useForm<DisableValues>({
    resolver: zodResolver(disableSchema),
    defaultValues: { password: "", code: "" },
  });

  const invalidateMe = () => queryClient.invalidateQueries({ queryKey: getApiAuthMeQueryKey() });

  const setupMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAuth2FaSetup({ body: {} });

      if (response.error) {
        if (response.response?.status === 409) {
          setEnabled(true);
          setShowDisable(true);
          void invalidateMe();
          return null;
        }
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.twoFactorSetupFailed"));
      }

      return response.data;
    },
    onSuccess: (data) => {
      if (!data) {
        return;
      }
      setSetupSecret(data.secret ?? null);
      setSetupOtpauthURL(data.otpauthURL ?? null);
    },
  });

  const confirmMutation = useMutation({
    mutationFn: async (values: ConfirmValues) => {
      const response = await postApiAuth2FaConfirm({
        body: { code: values.code },
      });

      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.twoFactorVerifyFailed"));
      }

      return response.data;
    },
    onSuccess: (data) => {
      setEnabled(true);
      setSetupSecret(null);
      setSetupOtpauthURL(null);
      if (data.backupCodes?.length) {
        setBackupCodes(data.backupCodes);
      }
      confirmForm.reset();
      void invalidateMe();
    },
  });

  const disableMutation = useMutation({
    mutationFn: async (values: DisableValues) => {
      const response = await deleteApiAuth2Fa({
        body: values,
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("account.disable2FAFailed"));
      }
    },
    onSuccess: () => {
      setEnabled(false);
      setShowDisable(false);
      setBackupCodes(null);
      disableForm.reset();
      void invalidateMe();
    },
  });

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{t("account.twoFactor")}</CardTitle>
        <CardDescription>{t("account.twoFactorDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {backupCodes ? (
          <Alert variant="success">
            <AlertTitle>{t("auth.twoFactorBackupCodesTitle")}</AlertTitle>
            <AlertDescription>
              <div className="mt-2 font-mono text-sm">
                {backupCodes.map((code) => (
                  <div key={code}>{code}</div>
                ))}
              </div>
            </AlertDescription>
          </Alert>
        ) : null}

        {setupMutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("auth.twoFactorSetupFailed")}</AlertTitle>
            <AlertDescription>
              {setupMutation.error instanceof Error
                ? setupMutation.error.message
                : t("common.error")}
            </AlertDescription>
          </Alert>
        ) : null}

        {confirmMutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("auth.twoFactorVerifyFailed")}</AlertTitle>
            <AlertDescription>
              {confirmMutation.error instanceof Error
                ? confirmMutation.error.message
                : t("common.error")}
            </AlertDescription>
          </Alert>
        ) : null}

        {disableMutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("account.disable2FAFailed")}</AlertTitle>
            <AlertDescription>
              {disableMutation.error instanceof Error
                ? disableMutation.error.message
                : t("common.error")}
            </AlertDescription>
          </Alert>
        ) : null}

        {!enabled && !setupSecret ? (
          <Button
            variant="outline"
            disabled={setupMutation.isPending}
            onClick={() => {
              setupMutation.mutate();
            }}
          >
            {setupMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
            {t("account.enable2FA")}
          </Button>
        ) : null}

        {setupSecret ? (
          <div className="flex flex-col gap-4">
            {setupOtpauthURL ? (
              <TotpQrCode otpauthURL={setupOtpauthURL} label={t("auth.twoFactorSetupTitle")} />
            ) : null}
            <Alert>
              <AlertTitle>{t("auth.twoFactorSecret")}</AlertTitle>
              <AlertDescription className="break-all font-mono">{setupSecret}</AlertDescription>
            </Alert>
            {setupOtpauthURL ? (
              <FieldDescription>
                <a href={setupOtpauthURL} target="_blank" rel="noreferrer">
                  {t("auth.twoFactorOpenAuthenticator")}
                </a>
              </FieldDescription>
            ) : null}
            <form
              className="flex flex-col gap-4"
              onSubmit={confirmForm.handleSubmit((values) => {
                confirmMutation.mutate(values);
              })}
            >
              <FieldGroup>
                <Field data-invalid={Boolean(confirmForm.formState.errors.code)}>
                  <FieldLabel htmlFor="confirm-code">{t("auth.twoFactorCode")}</FieldLabel>
                  <TotpCodeInput control={confirmForm.control} name="code" id="confirm-code" />
                </Field>
                <Field>
                  <Button type="submit" disabled={confirmMutation.isPending}>
                    {confirmMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                    {t("account.confirm2FA")}
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          </div>
        ) : null}

        {enabled || showDisable ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">{t("account.twoFactorEnabled")}</p>
            {!showDisable ? (
              <Button variant="outline" onClick={() => setShowDisable(true)}>
                {t("account.disable2FA")}
              </Button>
            ) : (
              <form
                className="flex flex-col gap-4"
                onSubmit={disableForm.handleSubmit((values) => {
                  disableMutation.mutate(values);
                })}
              >
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="disable-password">{t("auth.password")}</FieldLabel>
                    <PasswordInput
                      id="disable-password"
                      autoComplete="current-password"
                      {...disableForm.register("password")}
                    />
                  </Field>
                  <Field data-invalid={Boolean(disableForm.formState.errors.code)}>
                    <FieldLabel htmlFor="disable-code">{t("auth.twoFactorCode")}</FieldLabel>
                    <TotpCodeInput control={disableForm.control} name="code" id="disable-code" />
                  </Field>
                  <Field>
                    <Button
                      type="submit"
                      variant="destructive"
                      disabled={disableMutation.isPending}
                    >
                      {disableMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                      {t("account.disable2FA")}
                    </Button>
                  </Field>
                </FieldGroup>
              </form>
            )}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
