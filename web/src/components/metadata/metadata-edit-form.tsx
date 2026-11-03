import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import type { MetadataResponse, VideoFields } from "@/api/metadata";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { usePatchMetadata } from "@/hooks/use-metadata";
import { translateApiError } from "@/lib/api-error";

type MetadataEditFormProps = {
  path: string;
  metadata: MetadataResponse;
  size?: "default" | "lg";
};

type EditTarget = "override" | "file";

type FieldDef = {
  key: keyof VideoFields;
  labelKey: string;
  kind: "string" | "number" | "list";
};

const EDITABLE_FIELDS: FieldDef[] = [
  { key: "title", labelKey: "metadata.fieldTitle", kind: "string" },
  { key: "sort_title", labelKey: "metadata.fieldSortTitle", kind: "string" },
  { key: "original_title", labelKey: "metadata.fieldOriginalTitle", kind: "string" },
  { key: "show", labelKey: "metadata.fieldShow", kind: "string" },
  { key: "season", labelKey: "metadata.fieldSeason", kind: "number" },
  { key: "episode", labelKey: "metadata.fieldEpisode", kind: "number" },
  { key: "episode_title", labelKey: "metadata.fieldEpisodeTitle", kind: "string" },
  { key: "year", labelKey: "metadata.fieldYear", kind: "number" },
  { key: "release_date", labelKey: "metadata.fieldReleaseDate", kind: "string" },
  { key: "description", labelKey: "metadata.fieldDescription", kind: "string" },
  { key: "genres", labelKey: "metadata.fieldGenres", kind: "list" },
  { key: "directors", labelKey: "metadata.fieldDirectors", kind: "list" },
  { key: "actors", labelKey: "metadata.fieldActors", kind: "list" },
  { key: "writers", labelKey: "metadata.fieldWriters", kind: "list" },
  { key: "producers", labelKey: "metadata.fieldProducers", kind: "list" },
  { key: "studio", labelKey: "metadata.fieldStudio", kind: "string" },
  { key: "composer", labelKey: "metadata.fieldComposer", kind: "string" },
  { key: "language", labelKey: "metadata.fieldLanguage", kind: "string" },
  { key: "country", labelKey: "metadata.fieldCountry", kind: "string" },
  { key: "content_rating", labelKey: "metadata.fieldContentRating", kind: "string" },
  { key: "imdb_id", labelKey: "metadata.fieldImdbId", kind: "string" },
  { key: "tmdb_id", labelKey: "metadata.fieldTmdbId", kind: "string" },
];

const TECHNICAL_FIELDS: Array<{ key: keyof VideoFields; labelKey: string }> = [
  { key: "duration_seconds", labelKey: "metadata.fieldDuration" },
  { key: "video_codec", labelKey: "metadata.fieldVideoCodec" },
  { key: "audio_codec", labelKey: "metadata.fieldAudioCodec" },
  { key: "width", labelKey: "metadata.fieldWidth" },
  { key: "height", labelKey: "metadata.fieldHeight" },
  { key: "frame_rate", labelKey: "metadata.fieldFrameRate" },
  { key: "bitrate", labelKey: "metadata.fieldBitrate" },
  { key: "audio_channels", labelKey: "metadata.fieldAudioChannels" },
  { key: "audio_languages", labelKey: "metadata.fieldAudioLanguages" },
];

function fieldToInput(value: unknown, kind: FieldDef["kind"]): string {
  if (value === null || value === undefined) {
    return "";
  }
  if (kind === "list" && Array.isArray(value)) {
    return value.filter((item) => typeof item === "string" || typeof item === "number").join(", ");
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

function formatReadonly(value: unknown): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (Array.isArray(value)) {
    const parts = value.filter((item) => typeof item === "string" || typeof item === "number");
    return parts.length === 0 ? null : parts.join(", ");
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    const text = String(value).trim();
    return text === "" ? null : text;
  }
  return null;
}

type DraftValues = Record<(typeof EDITABLE_FIELDS)[number]["key"], string>;

function buildDraft(effective: VideoFields | undefined): DraftValues {
  const draft = {} as DraftValues;
  for (const field of EDITABLE_FIELDS) {
    draft[field.key] = fieldToInput(effective?.[field.key], field.kind);
  }
  return draft;
}

function buildPatchFields(draft: DraftValues): Record<string, string | number | string[] | null> {
  const fields: Record<string, string | number | string[] | null> = {};
  for (const field of EDITABLE_FIELDS) {
    const raw = draft[field.key]?.trim() ?? "";
    if (raw === "") {
      fields[field.key] = null;
      continue;
    }
    if (field.kind === "number") {
      const parsed = Number(raw);
      fields[field.key] = Number.isFinite(parsed) ? parsed : null;
      continue;
    }
    if (field.kind === "list") {
      fields[field.key] = raw
        .split(/[,;/]/)
        .map((part) => part.trim())
        .filter(Boolean);
      continue;
    }
    fields[field.key] = raw;
  }
  return fields;
}

export function MetadataEditForm({ path, metadata, size = "default" }: MetadataEditFormProps) {
  const { t } = useTranslation();
  const patchMutation = usePatchMetadata(path);
  const [target, setTarget] = useState<EditTarget>("override");

  const schema = useMemo(() => {
    const shape = {} as { [K in keyof DraftValues]: z.ZodString };
    for (const field of EDITABLE_FIELDS) {
      shape[field.key] = z.string();
    }
    return z.object(shape);
  }, []);

  const form = useForm<DraftValues>({
    resolver: zodResolver(schema),
    defaultValues: buildDraft(metadata.effective),
  });

  useEffect(() => {
    form.reset(buildDraft(metadata.effective));
  }, [metadata.effective, form]);

  const fromFilename =
    metadata.filename_hint?.season != null &&
    metadata.filename_hint?.episode != null &&
    metadata.source?.original?.season == null &&
    metadata.source?.original?.episode == null;

  const saveLabel =
    target === "file"
      ? patchMutation.isPending
        ? t("metadata.saving")
        : t("metadata.saveFile")
      : patchMutation.isPending
        ? t("metadata.saving")
        : t("metadata.saveOverride");

  const errors = form.formState.errors;

  return (
    <form
      className="flex min-w-0 flex-col gap-4"
      onSubmit={form.handleSubmit((values) => {
        patchMutation.mutate({
          target,
          fields: buildPatchFields(values),
        });
      })}
    >
      {fromFilename ? (
        <p
          className={
            size === "lg" ? "text-sm text-muted-foreground" : "text-xs text-muted-foreground"
          }
        >
          {t("metadata.fromFilenameHint")}
        </p>
      ) : null}

      {metadata.overridden_by?.startsWith("user:") ? (
        <p
          className={
            size === "lg" ? "text-sm text-muted-foreground" : "text-xs text-muted-foreground"
          }
        >
          {t("metadata.manuallyEdited")}
        </p>
      ) : null}

      <div className="flex flex-col gap-2">
        <ButtonGroup>
          <Button
            type="button"
            size={size}
            variant={target === "override" ? "default" : "outline"}
            onClick={() => setTarget("override")}
            disabled={patchMutation.isPending}
          >
            {t("metadata.targetOverride")}
          </Button>
          <Button
            type="button"
            size={size}
            variant={target === "file" ? "default" : "outline"}
            onClick={() => setTarget("file")}
            disabled={patchMutation.isPending}
          >
            {t("metadata.targetFile")}
          </Button>
        </ButtonGroup>
        <p
          className={
            size === "lg" ? "text-sm text-muted-foreground" : "text-xs text-muted-foreground"
          }
        >
          {target === "file" ? t("metadata.targetFileHint") : t("metadata.targetOverrideHint")}
        </p>
      </div>

      <Table className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[40%]">{t("metadata.columnField")}</TableHead>
            <TableHead>{t("metadata.columnValue")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {EDITABLE_FIELDS.map((field) => (
            <TableRow key={field.key}>
              <TableCell className="align-middle font-medium whitespace-normal">
                {t(field.labelKey)}
              </TableCell>
              <TableCell className="min-w-0">
                <Field data-invalid={Boolean(errors[field.key])}>
                  <Input
                    inputMode={field.kind === "number" ? "numeric" : undefined}
                    disabled={patchMutation.isPending}
                    className={size === "lg" ? "h-10 text-base" : undefined}
                    aria-invalid={Boolean(errors[field.key])}
                    {...form.register(field.key)}
                  />
                  <FieldError errors={[errors[field.key]]} />
                </Field>
              </TableCell>
            </TableRow>
          ))}
          {TECHNICAL_FIELDS.map((field) => {
            const value = formatReadonly(metadata.effective?.[field.key]);
            if (!value) {
              return null;
            }
            return (
              <TableRow key={field.key}>
                <TableCell className="align-middle font-medium whitespace-normal text-muted-foreground">
                  {t(field.labelKey)}
                </TableCell>
                <TableCell className="min-w-0 whitespace-normal break-words text-muted-foreground">
                  {value}
                </TableCell>
              </TableRow>
            );
          })}
          <TableRow>
            <TableCell className="align-top font-medium whitespace-normal text-muted-foreground">
              {t("metadata.fieldPath")}
            </TableCell>
            <TableCell className="min-w-0 whitespace-normal">
              <p
                className={
                  size === "lg"
                    ? "min-w-0 max-w-full break-all font-mono text-sm text-muted-foreground"
                    : "min-w-0 max-w-full break-all font-mono text-xs text-muted-foreground"
                }
                title={path}
              >
                {path}
              </p>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <div className="flex items-center gap-2">
        <Button type="submit" size={size} disabled={patchMutation.isPending}>
          {patchMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          {saveLabel}
        </Button>
      </div>

      {patchMutation.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {translateApiError(patchMutation.error, t, { fallbackKey: "metadata.saveFailed" })}
          </AlertDescription>
        </Alert>
      ) : null}
    </form>
  );
}
