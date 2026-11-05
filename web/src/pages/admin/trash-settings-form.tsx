import { useEffect, useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

export type TrashSettingsValues = {
  enabled: boolean;
  retentionDays: number;
};

type TrashSettingsFormProps = {
  enabled: boolean;
  retentionDays: number;
  pending: boolean;
  onSave: (values: TrashSettingsValues) => void;
};

export function TrashSettingsForm({
  enabled,
  retentionDays,
  pending,
  onSave,
}: TrashSettingsFormProps) {
  const { t } = useTranslation();
  const schema = useMemo(
    () =>
      z.object({
        enabled: z.boolean(),
        retentionDays: z
          .number({ error: t("validation.required") })
          .int({ error: t("validation.required") })
          .min(1, { error: t("apiErrors.invalidRetentionDays") }),
      }),
    [t],
  );
  type Values = z.infer<typeof schema>;

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { enabled, retentionDays },
  });

  useEffect(() => {
    form.reset({ enabled, retentionDays });
  }, [enabled, retentionDays, form]);

  const errors = form.formState.errors;

  return (
    <form
      onSubmit={form.handleSubmit((values) => {
        onSave(values);
      })}
    >
      <FieldGroup>
        <Field orientation="horizontal">
          <div className="flex items-center gap-2">
            <Controller
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="trashRetentionEnabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="trashRetentionEnabled">
              {t("admin.trashRetentionEnabled")}
            </FieldLabel>
          </div>
        </Field>
        <FieldDescription>{t("admin.trashRetentionEnabledDescription")}</FieldDescription>

        <Field data-invalid={Boolean(errors.retentionDays)}>
          <FieldLabel htmlFor="retention-days">{t("admin.trashRetentionDays")}</FieldLabel>
          <Input
            id="retention-days"
            type="number"
            min={1}
            className="max-w-md"
            aria-invalid={Boolean(errors.retentionDays)}
            {...form.register("retentionDays", { valueAsNumber: true })}
          />
          <FieldDescription>{t("admin.trashRetentionHint")}</FieldDescription>
          <FieldError errors={[errors.retentionDays]} />
        </Field>

        <Field>
          <Button type="submit" disabled={pending}>
            {pending ? <Spinner data-icon="inline-start" /> : null}
            {pending ? t("admin.saving") : t("admin.saveTrashSettings")}
          </Button>
        </Field>
      </FieldGroup>
    </form>
  );
}
