import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { PlayIcon, SquareIcon } from "lucide-react";
import { z } from "zod";

import type { SudoStreamInternalMaintenanceActionStatus } from "@/client/types.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { SUBTITLE_LANGUAGE_OPTIONS } from "@/lib/subtitle-languages";

type ActionStatus = SudoStreamInternalMaintenanceActionStatus;

export const LIBRARY_CARD_TITLE_CLASS = "text-lg font-semibold";

type ScheduleValues = {
  cron: string;
  enabled: boolean;
};

type MaintenanceActionCardProps = {
  title: string;
  description: string;
  cron: string;
  enabled: boolean;
  status?: ActionStatus;
  saving: boolean;
  running: boolean;
  canceling?: boolean;
  onSave: (values: ScheduleValues) => void;
  onRun: () => void;
  onCancel?: () => void;
};

export function MaintenanceActionCard({
  title,
  description,
  cron,
  enabled,
  status,
  saving,
  running,
  canceling,
  onSave,
  onRun,
  onCancel,
}: MaintenanceActionCardProps) {
  const { t } = useTranslation();
  const latest = status?.latestRun;
  const canCancel = Boolean(running && latest?.id && onCancel);
  const enabledId = `maintenance-enabled-${title.replace(/\s+/g, "-").toLowerCase()}`;

  const schema = useMemo(
    () =>
      z
        .object({
          cron: z.string(),
          enabled: z.boolean(),
        })
        .superRefine((data, ctx) => {
          if (data.enabled && data.cron.trim() === "") {
            ctx.addIssue({
              code: "custom",
              path: ["cron"],
              message: t("admin.maintenanceCronRequired"),
            });
          }
        }),
    [t],
  );

  const form = useForm<ScheduleValues>({
    resolver: zodResolver(schema),
    defaultValues: { cron, enabled },
  });

  useEffect(() => {
    form.reset({ cron, enabled });
  }, [cron, enabled, form]);

  const errors = form.formState.errors;

  return (
    <Card>
      <CardHeader>
        <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-4"
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
                    <Switch id={enabledId} checked={field.value} onCheckedChange={field.onChange} />
                  )}
                />
                <FieldLabel htmlFor={enabledId}>{t("admin.maintenanceEnabled")}</FieldLabel>
              </div>
            </Field>
            <Field data-invalid={Boolean(errors.cron)}>
              <FieldLabel htmlFor={`${enabledId}-cron`}>{t("admin.maintenanceCron")}</FieldLabel>
              <Input
                id={`${enabledId}-cron`}
                placeholder="0 3 * * *"
                aria-invalid={Boolean(errors.cron)}
                {...form.register("cron")}
              />
              <FieldDescription>{t("admin.maintenanceCronHelp")}</FieldDescription>
              <FieldError errors={[errors.cron]} />
            </Field>
          </FieldGroup>

          <p className="text-sm text-muted-foreground">
            {status?.running
              ? t("admin.maintenanceRunning")
              : latest
                ? t("admin.maintenanceLastRun", {
                    status: latest.status,
                    at: latest.finishedAt ?? latest.startedAt,
                  })
                : t("admin.maintenanceNeverRun")}
          </p>

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={saving || running}>
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {t("admin.maintenanceSave")}
            </Button>
            <Button type="button" variant="outline" onClick={onRun} disabled={running || saving}>
              {running ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <PlayIcon data-icon="inline-start" />
              )}
              {t("admin.maintenanceRunNow")}
            </Button>
            {canCancel ? (
              <Button
                type="button"
                variant="destructive"
                onClick={onCancel}
                disabled={canceling || saving}
              >
                {canceling ? (
                  <Spinner data-icon="inline-start" />
                ) : (
                  <SquareIcon data-icon="inline-start" />
                )}
                {t("admin.maintenanceCancel")}
              </Button>
            ) : null}
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

export type ProviderTaskValues = {
  cron: string;
  enabled: boolean;
  provider: string;
  metadataWriteTarget?: string;
  metadataApplyMode?: string;
  allowOverrideUserMetadata?: boolean;
  subtitleLanguages?: string[];
};

type ProviderTaskCardProps = {
  title: string;
  description: string;
  providerLabel: string;
  providerValue: string;
  providerOptions: string[];
  noneLabel: string;
  cron: string;
  enabled: boolean;
  status?: ActionStatus;
  saving: boolean;
  running: boolean;
  canceling?: boolean;
  saveError?: string | null;
  onSave: (values: ProviderTaskValues) => void;
  onRun: () => void;
  onCancel?: () => void;
  metadataWriteTarget?: string;
  metadataApplyMode?: string;
  allowOverrideUserMetadata?: boolean;
  subtitleLanguages?: string[];
  requireSubtitleLanguages?: boolean;
};

export function ProviderTaskCard({
  title,
  description,
  providerLabel,
  providerValue,
  providerOptions,
  noneLabel,
  cron,
  enabled,
  status,
  saving,
  running,
  canceling,
  saveError,
  onSave,
  onRun,
  onCancel,
  metadataWriteTarget,
  metadataApplyMode,
  allowOverrideUserMetadata,
  subtitleLanguages,
  requireSubtitleLanguages,
}: ProviderTaskCardProps) {
  const { t } = useTranslation();
  const latest = status?.latestRun;
  const canCancel = Boolean(running && latest?.id && onCancel);
  const enabledId = `provider-enabled-${title.replace(/\s+/g, "-").toLowerCase()}`;
  const [languagesOpen, setLanguagesOpen] = useState(false);
  const showMetadataFields =
    metadataWriteTarget !== undefined &&
    metadataApplyMode !== undefined &&
    allowOverrideUserMetadata !== undefined;
  const showSubtitleLanguages = subtitleLanguages !== undefined;
  const allowOverrideId = `${enabledId}-allow-override`;

  const schema = useMemo(
    () =>
      z
        .object({
          cron: z.string(),
          enabled: z.boolean(),
          provider: z.string(),
          metadataWriteTarget: z.string().optional(),
          metadataApplyMode: z.string().optional(),
          allowOverrideUserMetadata: z.boolean().optional(),
          subtitleLanguages: z.array(z.string()).optional(),
        })
        .superRefine((data, ctx) => {
          if (data.enabled && data.cron.trim() === "") {
            ctx.addIssue({
              code: "custom",
              path: ["cron"],
              message: t("admin.maintenanceCronRequired"),
            });
          }
          if (
            requireSubtitleLanguages &&
            data.provider !== "none" &&
            (data.subtitleLanguages?.length ?? 0) === 0
          ) {
            ctx.addIssue({
              code: "custom",
              path: ["subtitleLanguages"],
              message: t("admin.providersSubtitleLanguagesRequired"),
            });
          }
        }),
    [requireSubtitleLanguages, t],
  );

  const form = useForm<ProviderTaskValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      cron,
      enabled,
      provider: providerValue,
      metadataWriteTarget,
      metadataApplyMode,
      allowOverrideUserMetadata,
      subtitleLanguages,
    },
  });

  useEffect(() => {
    form.reset({
      cron,
      enabled,
      provider: providerValue,
      metadataWriteTarget,
      metadataApplyMode,
      allowOverrideUserMetadata,
      subtitleLanguages,
    });
  }, [
    allowOverrideUserMetadata,
    cron,
    enabled,
    form,
    metadataApplyMode,
    metadataWriteTarget,
    providerValue,
    subtitleLanguages,
  ]);

  const errors = form.formState.errors;
  const provider = form.watch("provider");
  const languages = form.watch("subtitleLanguages") ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-4"
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
                    <Switch id={enabledId} checked={field.value} onCheckedChange={field.onChange} />
                  )}
                />
                <FieldLabel htmlFor={enabledId}>{t("admin.maintenanceEnabled")}</FieldLabel>
              </div>
            </Field>
            <Field>
              <FieldLabel>{providerLabel}</FieldLabel>
              <Controller
                control={form.control}
                name="provider"
                render={({ field }) => (
                  <Select
                    value={field.value}
                    onValueChange={(value) => {
                      field.onChange(value);
                      if (value === "none" && showSubtitleLanguages) {
                        form.setValue("subtitleLanguages", []);
                      }
                    }}
                  >
                    <SelectTrigger className="w-56">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="none">{noneLabel}</SelectItem>
                        {providerOptions.map((key) => (
                          <SelectItem key={key} value={key}>
                            {key}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                )}
              />
            </Field>
            <Field data-invalid={Boolean(errors.cron)}>
              <FieldLabel htmlFor={`${enabledId}-cron`}>{t("admin.maintenanceCron")}</FieldLabel>
              <Input
                id={`${enabledId}-cron`}
                placeholder="0 3 * * *"
                aria-invalid={Boolean(errors.cron)}
                {...form.register("cron")}
              />
              <FieldDescription>{t("admin.maintenanceCronHelp")}</FieldDescription>
              <FieldError errors={[errors.cron]} />
            </Field>

            {showMetadataFields ? (
              <>
                <Field>
                  <FieldLabel>{t("admin.providersWriteTarget")}</FieldLabel>
                  <Controller
                    control={form.control}
                    name="metadataWriteTarget"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger className="w-48">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="db">{t("admin.providersWriteTargetDb")}</SelectItem>
                          <SelectItem value="file">
                            {t("admin.providersWriteTargetFile")}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                </Field>
                <Field>
                  <FieldLabel>{t("admin.providersApplyMode")}</FieldLabel>
                  <Controller
                    control={form.control}
                    name="metadataApplyMode"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger className="w-56">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="fill_missing">
                            {t("admin.providersApplyModeFillMissing")}
                          </SelectItem>
                          <SelectItem value="full_rewrite">
                            {t("admin.providersApplyModeFullRewrite")}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                </Field>
                <Field>
                  <div className="flex items-center gap-2">
                    <Controller
                      control={form.control}
                      name="allowOverrideUserMetadata"
                      render={({ field }) => (
                        <Switch
                          id={allowOverrideId}
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      )}
                    />
                    <FieldLabel htmlFor={allowOverrideId}>
                      {t("admin.providersAllowOverride")}
                    </FieldLabel>
                  </div>
                  <FieldDescription>{t("admin.providersAllowOverrideHelp")}</FieldDescription>
                </Field>
              </>
            ) : null}

            {showSubtitleLanguages ? (
              <Field data-invalid={Boolean(errors.subtitleLanguages)}>
                <FieldLabel>{t("admin.providersSubtitleLanguages")}</FieldLabel>
                <Button
                  type="button"
                  variant="outline"
                  className="w-fit"
                  disabled={provider === "none"}
                  onClick={() => {
                    setLanguagesOpen(true);
                  }}
                >
                  {languages.length > 0
                    ? t("admin.providersSubtitleLanguagesSelected", {
                        count: languages.length,
                      })
                    : t("admin.providersSubtitleLanguagesNone")}
                </Button>
                <FieldDescription>{t("admin.providersSubtitleLanguagesHelp")}</FieldDescription>
                <FieldError errors={[errors.subtitleLanguages]} />
              </Field>
            ) : null}
          </FieldGroup>

          <p className="text-sm text-muted-foreground">
            {status?.running
              ? t("admin.maintenanceRunning")
              : latest
                ? t("admin.maintenanceLastRun", {
                    status: latest.status,
                    at: latest.finishedAt ?? latest.startedAt,
                  })
                : t("admin.maintenanceNeverRun")}
          </p>

          {saveError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("common.error")}</AlertTitle>
              <AlertDescription>{saveError}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={saving || running}>
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {t("admin.maintenanceSave")}
            </Button>
            <Button type="button" variant="outline" onClick={onRun} disabled={running || saving}>
              {running ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <PlayIcon data-icon="inline-start" />
              )}
              {t("admin.maintenanceRunNow")}
            </Button>
            {canCancel ? (
              <Button
                type="button"
                variant="destructive"
                onClick={onCancel}
                disabled={canceling || saving}
              >
                {canceling ? (
                  <Spinner data-icon="inline-start" />
                ) : (
                  <SquareIcon data-icon="inline-start" />
                )}
                {t("admin.maintenanceCancel")}
              </Button>
            ) : null}
          </div>
        </form>

        {showSubtitleLanguages ? (
          <Dialog open={languagesOpen} onOpenChange={setLanguagesOpen}>
            <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-md">
              <DialogHeader>
                <DialogTitle>{t("admin.providersSubtitleLanguagesDialogTitle")}</DialogTitle>
                <DialogDescription>{t("admin.providersSubtitleLanguagesHelp")}</DialogDescription>
              </DialogHeader>
              <div className="flex min-h-0 flex-col gap-2 overflow-y-auto">
                {SUBTITLE_LANGUAGE_OPTIONS.map(({ code, label }) => {
                  const checked = languages.includes(code);
                  const inputId = `subtitle-lang-${code}`;

                  return (
                    <label
                      key={code}
                      htmlFor={inputId}
                      className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 hover:bg-muted/50"
                    >
                      <Checkbox
                        id={inputId}
                        checked={checked}
                        disabled={provider === "none"}
                        onCheckedChange={(next) => {
                          const selected = next === true;
                          const updated = selected
                            ? [...languages, code]
                            : languages.filter((item) => item !== code);
                          form.setValue("subtitleLanguages", updated.sort(), {
                            shouldValidate: true,
                          });
                        }}
                      />
                      <span className="text-sm">
                        {label} ({code})
                      </span>
                    </label>
                  );
                })}
              </div>
              <DialogFooter>
                <Button
                  type="button"
                  onClick={() => {
                    setLanguagesOpen(false);
                  }}
                >
                  {t("common.save")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        ) : null}
      </CardContent>
    </Card>
  );
}
