import { useEffect, useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

type TrashSettingsFormProps = {
  retentionDays: number;
  pending: boolean;
  onSave: (days: number) => void;
};

export function TrashSettingsForm({ retentionDays, pending, onSave }: TrashSettingsFormProps) {
  const { t } = useTranslation();
  const schema = useMemo(
    () =>
      z.object({
        retentionDays: z
          .number({ error: t("validation.required") })
          .int({ error: t("validation.required") })
          .min(0, { error: t("apiErrors.invalidRetentionDays") }),
      }),
    [t],
  );
  type Values = z.infer<typeof schema>;

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { retentionDays },
  });

  useEffect(() => {
    form.reset({ retentionDays });
  }, [retentionDays, form]);

  const errors = form.formState.errors;

  return (
    <form
      onSubmit={form.handleSubmit((values) => {
        onSave(values.retentionDays);
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(errors.retentionDays)}>
          <FieldLabel htmlFor="retention-days">{t("admin.trashRetentionDays")}</FieldLabel>
          <div className="flex max-w-sm items-center gap-2">
            <Input
              id="retention-days"
              type="number"
              min={0}
              aria-invalid={Boolean(errors.retentionDays)}
              {...form.register("retentionDays", { valueAsNumber: true })}
            />
            <Button type="submit" disabled={pending}>
              {pending ? <Spinner data-icon="inline-start" /> : null}
              {t("common.save")}
            </Button>
          </div>
          <FieldDescription>{t("admin.trashRetentionHint")}</FieldDescription>
          <FieldError errors={[errors.retentionDays]} />
        </Field>
      </FieldGroup>
    </form>
  );
}
