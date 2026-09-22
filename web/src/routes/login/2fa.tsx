import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link, Navigate, useLocation, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { postApiAuth2FaConfirm, postApiAuth2FaSetup, postApiAuth2FaVerify } from "@/client/sdk.gen";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { TotpCodeInput } from "@/components/totp-code-input";
import { TotpQrCode } from "@/components/totp-qr-code";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { completeLoginSession } from "@/hooks/use-auth";
import { clearLoginRedirectFlag } from "@/lib/auth-session";
import { extractApiErrorMessage } from "@/lib/api-error";

type TwoFactorLocationState = {
  pendingToken?: string;
  status?: string;
  email?: string;
};

const backupCodePattern = /^[A-Z2-9]{8}$/;

export default function LoginTwoFactorPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const location = useLocation();
  const state = (location.state as TwoFactorLocationState | null) ?? {};

  const pendingToken = state.pendingToken ?? "";
  const isSetupRequired = state.status === "2fa_setup_required";

  const [setupSecret, setSetupSecret] = useState<string | null>(null);
  const [setupOtpauthURL, setSetupOtpauthURL] = useState<string | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[] | null>(null);
  const [useBackupCode, setUseBackupCode] = useState(false);

  const schema = useMemo(
    () =>
      z.object({
        code: useBackupCode
          ? z
              .string()
              .trim()
              .transform((value) => value.toUpperCase())
              .pipe(z.string().regex(backupCodePattern, { error: t("validation.backupCode") }))
          : z.string().length(6, { error: t("validation.totpCode") }),
      }),
    [t, useBackupCode],
  );

  type FormValues = z.infer<typeof schema>;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { code: "" },
  });

  useEffect(() => {
    form.clearErrors();
    form.setValue("code", "");
  }, [form, useBackupCode]);

  const setupMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAuth2FaSetup({
        body: { pendingToken },
      });

      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.twoFactorSetupFailed"));
      }

      return response.data;
    },
    onSuccess: (data) => {
      setSetupSecret(data.secret ?? null);
      setSetupOtpauthURL(data.otpauthURL ?? null);
    },
  });

  useEffect(() => {
    if (
      !isSetupRequired ||
      !pendingToken ||
      setupSecret ||
      setupMutation.isPending ||
      setupMutation.isSuccess
    ) {
      return;
    }

    setupMutation.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run setup once per pending token
  }, [isSetupRequired, pendingToken, setupSecret]);

  const verifyMutation = useMutation({
    mutationFn: async (values: FormValues) => {
      if (isSetupRequired) {
        const response = await postApiAuth2FaConfirm({
          body: { pendingToken, code: values.code },
        });

        if (response.error) {
          throw new Error(
            extractApiErrorMessage(response.error) ?? t("auth.twoFactorVerifyFailed"),
          );
        }

        const data = response.data as {
          accessToken?: string;
          backupCodes?: string[];
        };

        if (data.backupCodes?.length) {
          setBackupCodes(data.backupCodes);
        }

        return data;
      }

      const response = await postApiAuth2FaVerify({
        body: { pendingToken, code: values.code },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.twoFactorVerifyFailed"));
      }

      return response.data;
    },
    onSuccess: (data) => {
      if (data?.backupCodes?.length) {
        if (data.accessToken) {
          completeLoginSession(queryClient, data.accessToken);
        }
        setBackupCodes(data.backupCodes);
        return;
      }

      if (!data?.accessToken) {
        form.setError("root", { message: t("auth.twoFactorVerifyFailed") });
        return;
      }

      completeLoginSession(queryClient, data.accessToken);
      clearLoginRedirectFlag();
      void navigate("/", { replace: true });
    },
  });

  if (!pendingToken) {
    return <Navigate to="/login" replace />;
  }

  if (backupCodes) {
    return (
      <GuestAuthLayout
        title={t("auth.twoFactorBackupCodesTitle")}
        description={t("auth.twoFactorBackupCodesDescription")}
      >
        <div className="flex flex-col gap-4">
          <div className="rounded-lg border bg-muted/50 p-4 font-mono text-sm">
            {backupCodes.map((code) => (
              <div key={code}>{code}</div>
            ))}
          </div>
          <Button
            className="w-full"
            onClick={() => {
              clearLoginRedirectFlag();
              void navigate("/", { replace: true });
            }}
          >
            {t("auth.continue")}
          </Button>
        </div>
      </GuestAuthLayout>
    );
  }

  const title = isSetupRequired ? t("auth.twoFactorSetupTitle") : t("auth.twoFactorTitle");
  const description = isSetupRequired
    ? t("auth.twoFactorSetupDescription")
    : t("auth.twoFactorDescription", { email: state.email ?? "" });

  return (
    <GuestAuthLayout title={title} description={description}>
      <form
        className="flex flex-col gap-4"
        onSubmit={form.handleSubmit((values) => {
          verifyMutation.reset();
          verifyMutation.mutate(values);
        })}
      >
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

        {verifyMutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("auth.twoFactorVerifyFailed")}</AlertTitle>
            <AlertDescription>
              {verifyMutation.error instanceof Error
                ? verifyMutation.error.message
                : t("common.error")}
            </AlertDescription>
          </Alert>
        ) : null}

        {isSetupRequired && setupMutation.isPending ? (
          <div className="flex justify-center py-4">
            <Spinner className="size-8" />
          </div>
        ) : null}

        {isSetupRequired && setupOtpauthURL ? (
          <TotpQrCode otpauthURL={setupOtpauthURL} label={t("auth.twoFactorSetupTitle")} />
        ) : null}

        {isSetupRequired && setupSecret ? (
          <Alert>
            <AlertTitle>{t("auth.twoFactorSecret")}</AlertTitle>
            <AlertDescription className="break-all font-mono">{setupSecret}</AlertDescription>
          </Alert>
        ) : null}

        {isSetupRequired && setupOtpauthURL ? (
          <FieldDescription>
            <a href={setupOtpauthURL} target="_blank" rel="noreferrer">
              {t("auth.twoFactorOpenAuthenticator")}
            </a>
          </FieldDescription>
        ) : null}

        <FieldGroup>
          <Field data-invalid={Boolean(form.formState.errors.code)}>
            <FieldLabel htmlFor="code">
              {useBackupCode ? t("auth.twoFactorBackupCode") : t("auth.twoFactorCode")}
            </FieldLabel>
            {useBackupCode ? (
              <Input
                id="code"
                autoComplete="one-time-code"
                className="font-mono uppercase"
                maxLength={8}
                aria-invalid={Boolean(form.formState.errors.code)}
                {...form.register("code")}
              />
            ) : (
              <TotpCodeInput control={form.control} name="code" id="code" />
            )}
          </Field>
          {!isSetupRequired ? (
            <Field>
              <Button
                type="button"
                variant="link"
                className="w-full"
                onClick={() => {
                  setUseBackupCode((current) => !current);
                }}
              >
                {useBackupCode
                  ? t("auth.twoFactorUseAuthenticatorCode")
                  : t("auth.twoFactorUseBackupCode")}
              </Button>
            </Field>
          ) : null}
          <Field>
            <Button
              type="submit"
              disabled={verifyMutation.isPending || (isSetupRequired && !setupSecret)}
              className="w-full"
            >
              {verifyMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {verifyMutation.isPending ? t("auth.verifying") : t("auth.verify")}
            </Button>
          </Field>
          <Field>
            <Button variant="link" asChild className="w-full">
              <Link to="/login">{t("auth.backToLogin")}</Link>
            </Button>
          </Field>
        </FieldGroup>
      </form>
    </GuestAuthLayout>
  );
}
