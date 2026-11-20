import { useEffect, useMemo, useRef } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { AlertCircleIcon, InfoIcon } from "lucide-react";
import { z } from "zod";

import { getApiAuthOidcStatus, postApiAuthLogin } from "@/client/sdk.gen";
import { clearLoginRedirectFlag } from "@/lib/auth-session";
import { cn } from "@/lib/utils";
import { completeLoginSession } from "@/hooks/use-auth";
import { extractApiErrorMessage, translateApiError } from "@/lib/api-error";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";
import { AppPreferences } from "@/components/app-preferences";

type LoginAlert = {
  id: string;
  variant: "default" | "destructive";
  message: string;
};

export function LoginForm({ className, ...props }: React.ComponentProps<"div">) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const autoRedirected = useRef(false);

  const oidcQuery = useQuery({
    queryKey: ["auth", "oidc-status"],
    queryFn: async () => {
      const response = await getApiAuthOidcStatus();
      if (response.error) {
        return { enabled: false, only: false, autoRedirect: false, issuerName: "" };
      }
      return response.data ?? { enabled: false, only: false, autoRedirect: false, issuerName: "" };
    },
    staleTime: 60_000,
  });

  const oidc = oidcQuery.data;
  const oidcEnabled = Boolean(oidc?.enabled);
  const oidcOnly = Boolean(oidc?.only);
  const oidcIssuerName = oidc?.issuerName || "SSO";

  useEffect(() => {
    if (!oidcEnabled || !oidc?.autoRedirect || autoRedirected.current) {
      return;
    }
    if (searchParams.get("oidc_error")) {
      return;
    }
    autoRedirected.current = true;
    window.location.assign("/api/auth/oidc/start");
  }, [oidc?.autoRedirect, oidcEnabled, searchParams]);

  const loginSchema = useMemo(
    () =>
      z.object({
        email: z.email({ error: t("validation.email") }),
        password: z.string().min(1, { error: t("validation.required") }),
      }),
    [t],
  );

  type LoginValues = z.infer<typeof loginSchema>;

  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  const loginMutation = useMutation({
    mutationFn: async (values: LoginValues) => {
      const response = await postApiAuthLogin({ body: values });

      if (response.error) {
        const message = extractApiErrorMessage(response.error) ?? t("auth.loginFailed");
        throw new Error(message);
      }

      return response.data;
    },
    onSuccess: (data) => {
      if (!data) {
        form.setError("root", { message: t("auth.loginFailed") });
        return;
      }

      if (data.status === "2fa_required" || data.status === "2fa_setup_required") {
        void navigate("/login/2fa", {
          replace: true,
          state: {
            pendingToken: data.pendingToken,
            status: data.status,
            email: form.getValues("email"),
          },
        });
        return;
      }

      if (!data.accessToken) {
        form.setError("root", { message: t("auth.loginFailed") });
        return;
      }

      completeLoginSession(queryClient, data.accessToken);
      clearLoginRedirectFlag();
      void navigate("/", { replace: true });
    },
  });

  const { errors } = form.formState;

  const alerts: LoginAlert[] = [];
  const oidcError = searchParams.get("oidc_error");
  if (oidcError) {
    alerts.push({
      id: "oidc",
      variant: "destructive",
      message: translateApiError(new Error(oidcError), t, { fallbackKey: "auth.oidcLoginFailed" }),
    });
  }

  if (loginMutation.error instanceof Error) {
    alerts.push({
      id: "root",
      variant: "destructive",
      message: translateApiError(loginMutation.error, t, { fallbackKey: "auth.loginFailed" }),
    });
  } else if (errors.root?.message) {
    alerts.push({
      id: "root",
      variant: errors.root.message === t("auth.twoFactorRequired") ? "default" : "destructive",
      message: errors.root.message,
    });
  }

  const showPasswordForm = !oidcOnly;

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <div className="flex justify-end px-4">
          <AppPreferences />
        </div>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">{t("auth.welcomeBack")}</CardTitle>
          <CardDescription>
            {oidcOnly
              ? t("auth.oidcOnlyDescription", { name: oidcIssuerName })
              : t("auth.loginDescription")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {alerts.length > 0 ? (
            <div className="mb-4 flex flex-col gap-2">
              {alerts.map((alert) => (
                <Alert key={alert.id} variant={alert.variant}>
                  {alert.variant === "destructive" ? <AlertCircleIcon /> : <InfoIcon />}
                  <AlertTitle>{t("auth.loginError")}</AlertTitle>
                  <AlertDescription>{alert.message}</AlertDescription>
                </Alert>
              ))}
            </div>
          ) : null}

          {oidcEnabled ? (
            <div className="mb-4 flex flex-col gap-3">
              <Button asChild className="w-full" variant={oidcOnly ? "default" : "outline"}>
                <a href="/api/auth/oidc/start">
                  {t("auth.continueWithSso", { name: oidcIssuerName })}
                </a>
              </Button>
              {showPasswordForm ? <FieldSeparator /> : null}
            </div>
          ) : null}

          {showPasswordForm ? (
            <form
              className="flex flex-col gap-4"
              onSubmit={form.handleSubmit((values) => {
                loginMutation.reset();
                loginMutation.mutate(values);
              })}
            >
              <FieldGroup>
                <Field data-invalid={Boolean(errors.email)}>
                  <FieldLabel htmlFor="email">{t("auth.email")}</FieldLabel>
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    aria-invalid={Boolean(errors.email)}
                    {...form.register("email")}
                  />
                  <FieldError errors={[errors.email]} />
                </Field>
                <Field data-invalid={Boolean(errors.password)}>
                  <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
                  <PasswordInput
                    id="password"
                    autoComplete="current-password"
                    aria-invalid={Boolean(errors.password)}
                    {...form.register("password")}
                  />
                  <FieldError errors={[errors.password]} />
                </Field>
                <FieldSeparator />
                <Field>
                  <FieldDescription>
                    <Link to="/forgot-password">{t("auth.forgotPassword")}</Link>
                  </FieldDescription>
                </Field>
                <Field>
                  <Button type="submit" disabled={loginMutation.isPending} className="w-full">
                    {loginMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                    {loginMutation.isPending ? t("auth.loggingIn") : t("auth.login")}
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
