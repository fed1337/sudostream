import { useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { AlertCircleIcon, InfoIcon } from "lucide-react";
import { z } from "zod";

import { postApiAuthLogin } from "@/client/sdk.gen";
import { clearLoginRedirectFlag } from "@/lib/auth-session";
import { cn } from "@/lib/utils";
import { completeLoginSession } from "@/hooks/use-auth";
import { extractApiErrorMessage } from "@/lib/api-error";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Field,
  FieldDescription,
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

  if (errors.email?.message) {
    alerts.push({
      id: "email",
      variant: "destructive",
      message: errors.email.message,
    });
  }

  if (errors.password?.message) {
    alerts.push({
      id: "password",
      variant: "destructive",
      message: errors.password.message,
    });
  }

  if (loginMutation.error instanceof Error) {
    alerts.push({
      id: "root",
      variant: "destructive",
      message: loginMutation.error.message,
    });
  } else if (errors.root?.message) {
    alerts.push({
      id: "root",
      variant: errors.root.message === t("auth.twoFactorRequired") ? "default" : "destructive",
      message: errors.root.message,
    });
  }

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <div className="flex justify-end px-4">
          <AppPreferences />
        </div>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">{t("auth.welcomeBack")}</CardTitle>
          <CardDescription>{t("auth.loginDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={form.handleSubmit((values) => {
              loginMutation.reset();
              loginMutation.mutate(values);
            })}
          >
            {alerts.length > 0 ? (
              <div className="flex flex-col gap-2">
                {alerts.map((alert) => (
                  <Alert key={alert.id} variant={alert.variant}>
                    {alert.variant === "destructive" ? <AlertCircleIcon /> : <InfoIcon />}
                    <AlertTitle>
                      {alert.id === "root"
                        ? t("auth.loginError")
                        : alert.id === "email"
                          ? t("auth.email")
                          : t("auth.password")}
                    </AlertTitle>
                    <AlertDescription>{alert.message}</AlertDescription>
                  </Alert>
                ))}
              </div>
            ) : null}

            <FieldGroup>
              <Field data-invalid={Boolean(form.formState.errors.email)}>
                <FieldLabel htmlFor="email">{t("auth.email")}</FieldLabel>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  aria-invalid={Boolean(form.formState.errors.email)}
                  {...form.register("email")}
                />
              </Field>
              <Field data-invalid={Boolean(form.formState.errors.password)}>
                <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
                <PasswordInput
                  id="password"
                  autoComplete="current-password"
                  aria-invalid={Boolean(form.formState.errors.password)}
                  {...form.register("password")}
                />
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
        </CardContent>
      </Card>
    </div>
  );
}
