import { useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { postApiAuthAcceptInvite } from "@/client/sdk.gen";
import { GuestAuthLayout } from "@/components/guest-auth-layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";
import { extractApiErrorMessage } from "@/lib/api-error";

export default function AcceptInvitePage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const schema = useMemo(
    () =>
      z
        .object({
          password: z.string().min(8, { error: t("validation.passwordMin") }),
          confirmPassword: z.string().min(1, { error: t("validation.required") }),
        })
        .refine((data) => data.password === data.confirmPassword, {
          error: t("validation.passwordMismatch"),
          path: ["confirmPassword"],
        }),
    [t],
  );

  type FormValues = z.infer<typeof schema>;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { password: "", confirmPassword: "" },
  });

  const mutation = useMutation({
    mutationFn: async (values: FormValues) => {
      const response = await postApiAuthAcceptInvite({
        body: { token, password: values.password },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("auth.acceptInviteFailed"));
      }
    },
  });

  if (!token) {
    return (
      <GuestAuthLayout title={t("auth.acceptInviteTitle")}>
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
        title={t("auth.acceptInviteTitle")}
        description={t("auth.acceptInviteSuccess")}
      >
        <Button asChild className="w-full">
          <Link to="/login">{t("auth.login")}</Link>
        </Button>
      </GuestAuthLayout>
    );
  }

  return (
    <GuestAuthLayout
      title={t("auth.acceptInviteTitle")}
      description={t("auth.acceptInviteDescription")}
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
          <Field data-invalid={Boolean(form.formState.errors.password)}>
            <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
            <PasswordInput
              id="password"
              autoComplete="new-password"
              {...form.register("password")}
            />
          </Field>
          <Field data-invalid={Boolean(form.formState.errors.confirmPassword)}>
            <FieldLabel htmlFor="confirmPassword">{t("auth.confirmPassword")}</FieldLabel>
            <PasswordInput
              id="confirmPassword"
              autoComplete="new-password"
              {...form.register("confirmPassword")}
            />
          </Field>
          <Field>
            <Button type="submit" disabled={mutation.isPending} className="w-full">
              {mutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {mutation.isPending ? t("auth.accepting") : t("auth.acceptInvite")}
            </Button>
          </Field>
        </FieldGroup>
      </form>
    </GuestAuthLayout>
  );
}
