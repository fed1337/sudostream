import { useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { postApiAuthForgotPassword } from "@/client/sdk.gen";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { extractApiErrorMessage } from "@/lib/api-error";

export default function ForgotPasswordPage() {
  const { t } = useTranslation();

  const schema = useMemo(
    () =>
      z.object({
        email: z.email({ error: t("validation.email") }),
      }),
    [t],
  );

  type FormValues = z.infer<typeof schema>;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "" },
  });

  const mutation = useMutation({
    mutationFn: async (values: FormValues) => {
      const response = await postApiAuthForgotPassword({ body: values });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.forgotPasswordFailed"));
      }
    },
  });

  if (mutation.isSuccess) {
    return (
      <GuestAuthLayout
        title={t("auth.forgotPasswordTitle")}
        description={t("auth.forgotPasswordSent")}
      >
        <div className="flex flex-col gap-4">
          <Alert>
            <AlertTitle>{t("auth.checkYourEmail")}</AlertTitle>
            <AlertDescription>{t("auth.forgotPasswordSentDescription")}</AlertDescription>
          </Alert>
          <Button variant="outline" asChild>
            <Link to="/login">{t("auth.backToLogin")}</Link>
          </Button>
        </div>
      </GuestAuthLayout>
    );
  }

  return (
    <GuestAuthLayout
      title={t("auth.forgotPasswordTitle")}
      description={t("auth.forgotPasswordDescription")}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={form.handleSubmit((values) => {
          mutation.reset();
          mutation.mutate(values);
        })}
      >
        {mutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("common.error")}</AlertTitle>
            <AlertDescription>
              {mutation.error instanceof Error ? mutation.error.message : t("common.error")}
            </AlertDescription>
          </Alert>
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
          <Field>
            <Button type="submit" disabled={mutation.isPending} className="w-full">
              {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {mutation.isPending ? t("auth.sending") : t("auth.sendResetLink")}
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
