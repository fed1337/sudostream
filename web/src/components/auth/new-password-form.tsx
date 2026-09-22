import { useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { PasswordInput } from "@/components/password-input";
import { Spinner } from "@/components/ui/spinner";

export type NewPasswordFormValues = {
  password: string;
  confirmPassword: string;
};

type NewPasswordFormProps = {
  onSubmit: (values: NewPasswordFormValues) => void;
  isPending?: boolean;
  errorMessage?: string | null;
  submitLabel: string;
  pendingLabel: string;
};

export function NewPasswordForm({
  onSubmit,
  isPending = false,
  errorMessage,
  submitLabel,
  pendingLabel,
}: NewPasswordFormProps) {
  const { t } = useTranslation();

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

  const form = useForm<NewPasswordFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { password: "", confirmPassword: "" },
  });

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={form.handleSubmit((values) => {
        onSubmit(values);
      })}
    >
      {errorMessage ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>{errorMessage}</AlertDescription>
        </Alert>
      ) : null}

      <FieldGroup>
        <Field data-invalid={Boolean(form.formState.errors.password)}>
          <FieldLabel htmlFor="password">{t("auth.newPassword")}</FieldLabel>
          <PasswordInput id="password" autoComplete="new-password" {...form.register("password")} />
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
          <Button type="submit" disabled={isPending} className="w-full">
            {isPending ? <Spinner data-icon="inline-start" /> : null}
            {isPending ? pendingLabel : submitLabel}
          </Button>
        </Field>
      </FieldGroup>
    </form>
  );
}
