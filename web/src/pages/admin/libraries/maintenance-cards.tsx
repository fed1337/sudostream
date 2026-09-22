import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PlayIcon } from "lucide-react";

import type { SudoStreamInternalMaintenanceActionStatus } from "@/client/types.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SUBTITLE_LANGUAGE_OPTIONS } from "@/lib/subtitle-languages";

type ActionStatus = SudoStreamInternalMaintenanceActionStatus;

export const LIBRARY_CARD_TITLE_CLASS = "text-lg font-semibold";

type MaintenanceActionCardProps = {
  title: string;
  description: string;
  cron: string;
  enabled: boolean;
  status?: ActionStatus;
  saving: boolean;
  running: boolean;
  validationError?: string | null;
  onCronChange: (value: string) => void;
  onEnabledChange: (value: boolean) => void;
  onSave: () => void;
  onRun: () => void;
};

export function MaintenanceActionCard({
  title,
  description,
  cron,
  enabled,
  status,
  saving,
  running,
  validationError,
  onCronChange,
  onEnabledChange,
  onSave,
  onRun,
}: MaintenanceActionCardProps) {
  const { t } = useTranslation();
  const latest = status?.latestRun;
  const enabledId = `maintenance-enabled-${title.replace(/\s+/g, "-").toLowerCase()}`;

  return (
    <Card>
      <CardHeader>
        <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <FieldGroup>
          <Field orientation="horizontal">
            <div className="flex items-center gap-2">
              <Switch id={enabledId} checked={enabled} onCheckedChange={onEnabledChange} />
              <FieldLabel htmlFor={enabledId}>{t("admin.maintenanceEnabled")}</FieldLabel>
            </div>
          </Field>
          <Field>
            <FieldLabel>{t("admin.maintenanceCron")}</FieldLabel>
            <Input
              value={cron}
              placeholder="0 3 * * *"
              onChange={(event) => {
                onCronChange(event.target.value);
              }}
            />
            <FieldDescription>{t("admin.maintenanceCronHelp")}</FieldDescription>
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

        {validationError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("common.error")}</AlertTitle>
            <AlertDescription>{validationError}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button onClick={onSave} disabled={saving || running}>
            {saving ? <Spinner data-icon="inline-start" /> : null}
            {t("admin.maintenanceSave")}
          </Button>
          <Button variant="outline" onClick={onRun} disabled={running || saving}>
            {running ? <Spinner data-icon="inline-start" /> : <PlayIcon data-icon="inline-start" />}
            {t("admin.maintenanceRunNow")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

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
  validationError?: string | null;
  onProviderChange: (value: string) => void;
  onCronChange: (value: string) => void;
  onEnabledChange: (value: boolean) => void;
  onSave: () => void;
  onRun: () => void;
  metadataWriteTarget?: string;
  metadataApplyMode?: string;
  allowOverrideUserMetadata?: boolean;
  onMetadataWriteTargetChange?: (value: string) => void;
  onMetadataApplyModeChange?: (value: string) => void;
  onAllowOverrideUserMetadataChange?: (value: boolean) => void;
  subtitleLanguages?: string[];
  onSubtitleLanguagesChange?: (value: string[]) => void;
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
  validationError,
  onProviderChange,
  onCronChange,
  onEnabledChange,
  onSave,
  onRun,
  metadataWriteTarget,
  metadataApplyMode,
  allowOverrideUserMetadata,
  onMetadataWriteTargetChange,
  onMetadataApplyModeChange,
  onAllowOverrideUserMetadataChange,
  subtitleLanguages,
  onSubtitleLanguagesChange,
}: ProviderTaskCardProps) {
  const { t } = useTranslation();
  const latest = status?.latestRun;
  const enabledId = `provider-enabled-${title.replace(/\s+/g, "-").toLowerCase()}`;
  const [languagesOpen, setLanguagesOpen] = useState(false);
  const showMetadataFields =
    metadataWriteTarget !== undefined &&
    metadataApplyMode !== undefined &&
    allowOverrideUserMetadata !== undefined;
  const showSubtitleLanguages =
    subtitleLanguages !== undefined && onSubtitleLanguagesChange !== undefined;
  const allowOverrideId = `${enabledId}-allow-override`;

  return (
    <Card>
      <CardHeader>
        <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <FieldGroup>
          <Field orientation="horizontal">
            <div className="flex items-center gap-2">
              <Switch id={enabledId} checked={enabled} onCheckedChange={onEnabledChange} />
              <FieldLabel htmlFor={enabledId}>{t("admin.maintenanceEnabled")}</FieldLabel>
            </div>
          </Field>
          <Field>
            <FieldLabel>{providerLabel}</FieldLabel>
            <Select value={providerValue} onValueChange={onProviderChange}>
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
          </Field>
          <Field>
            <FieldLabel>{t("admin.maintenanceCron")}</FieldLabel>
            <Input
              value={cron}
              placeholder="0 3 * * *"
              onChange={(event) => {
                onCronChange(event.target.value);
              }}
            />
            <FieldDescription>{t("admin.maintenanceCronHelp")}</FieldDescription>
          </Field>

          {showMetadataFields ? (
            <>
              <Field>
                <FieldLabel>{t("admin.providersWriteTarget")}</FieldLabel>
                <Select
                  value={metadataWriteTarget}
                  onValueChange={(value) => {
                    onMetadataWriteTargetChange?.(value);
                  }}
                >
                  <SelectTrigger className="w-48">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="db">{t("admin.providersWriteTargetDb")}</SelectItem>
                    <SelectItem value="file">{t("admin.providersWriteTargetFile")}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel>{t("admin.providersApplyMode")}</FieldLabel>
                <Select
                  value={metadataApplyMode}
                  onValueChange={(value) => {
                    onMetadataApplyModeChange?.(value);
                  }}
                >
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
              </Field>
              <Field>
                <div className="flex items-center gap-2">
                  <Switch
                    id={allowOverrideId}
                    checked={allowOverrideUserMetadata}
                    onCheckedChange={(checked) => {
                      onAllowOverrideUserMetadataChange?.(checked);
                    }}
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
            <Field>
              <FieldLabel>{t("admin.providersSubtitleLanguages")}</FieldLabel>
              <Button
                type="button"
                variant="outline"
                className="w-fit"
                disabled={providerValue === "none"}
                onClick={() => {
                  setLanguagesOpen(true);
                }}
              >
                {subtitleLanguages.length > 0
                  ? t("admin.providersSubtitleLanguagesSelected", {
                      count: subtitleLanguages.length,
                    })
                  : t("admin.providersSubtitleLanguagesNone")}
              </Button>
              <FieldDescription>{t("admin.providersSubtitleLanguagesHelp")}</FieldDescription>
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

        {validationError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("common.error")}</AlertTitle>
            <AlertDescription>{validationError}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button onClick={onSave} disabled={saving || running}>
            {saving ? <Spinner data-icon="inline-start" /> : null}
            {t("admin.maintenanceSave")}
          </Button>
          <Button variant="outline" onClick={onRun} disabled={running || saving}>
            {running ? <Spinner data-icon="inline-start" /> : <PlayIcon data-icon="inline-start" />}
            {t("admin.maintenanceRunNow")}
          </Button>
        </div>

        {showSubtitleLanguages ? (
          <Dialog open={languagesOpen} onOpenChange={setLanguagesOpen}>
            <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-md">
              <DialogHeader>
                <DialogTitle>{t("admin.providersSubtitleLanguagesDialogTitle")}</DialogTitle>
                <DialogDescription>{t("admin.providersSubtitleLanguagesHelp")}</DialogDescription>
              </DialogHeader>
              <div className="flex min-h-0 flex-col gap-2 overflow-y-auto">
                {SUBTITLE_LANGUAGE_OPTIONS.map(({ code, label }) => {
                  const checked = subtitleLanguages.includes(code);
                  const inputId = `subtitle-lang-${code}`;

                  return (
                    <label
                      key={code}
                      htmlFor={inputId}
                      className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 hover:bg-muted/50"
                    >
                      <input
                        id={inputId}
                        type="checkbox"
                        className="size-4 rounded border"
                        checked={checked}
                        disabled={providerValue === "none"}
                        onChange={(event) => {
                          const next = event.target.checked
                            ? [...subtitleLanguages, code]
                            : subtitleLanguages.filter((item) => item !== code);
                          onSubtitleLanguagesChange?.(next.sort());
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
