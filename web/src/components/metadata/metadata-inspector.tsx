import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDownIcon } from "lucide-react";

import type { MetadataResponse, VideoFields } from "@/api/metadata";
import { MetadataEditForm } from "@/components/metadata/metadata-edit-form";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useMetadata } from "@/hooks/use-metadata";
import { cn } from "@/lib/utils";

type MetadataInspectorProps = {
  path: string;
  editable?: boolean;
  variant?: "card" | "sidebar";
};

const VIEW_FIELDS: Array<{ key: keyof VideoFields; labelKey: string }> = [
  { key: "title", labelKey: "metadata.fieldTitle" },
  { key: "sort_title", labelKey: "metadata.fieldSortTitle" },
  { key: "original_title", labelKey: "metadata.fieldOriginalTitle" },
  { key: "show", labelKey: "metadata.fieldShow" },
  { key: "season", labelKey: "metadata.fieldSeason" },
  { key: "episode", labelKey: "metadata.fieldEpisode" },
  { key: "episode_title", labelKey: "metadata.fieldEpisodeTitle" },
  { key: "year", labelKey: "metadata.fieldYear" },
  { key: "release_date", labelKey: "metadata.fieldReleaseDate" },
  { key: "description", labelKey: "metadata.fieldDescription" },
  { key: "genres", labelKey: "metadata.fieldGenres" },
  { key: "directors", labelKey: "metadata.fieldDirectors" },
  { key: "actors", labelKey: "metadata.fieldActors" },
  { key: "writers", labelKey: "metadata.fieldWriters" },
  { key: "producers", labelKey: "metadata.fieldProducers" },
  { key: "studio", labelKey: "metadata.fieldStudio" },
  { key: "composer", labelKey: "metadata.fieldComposer" },
  { key: "language", labelKey: "metadata.fieldLanguage" },
  { key: "country", labelKey: "metadata.fieldCountry" },
  { key: "content_rating", labelKey: "metadata.fieldContentRating" },
  { key: "imdb_id", labelKey: "metadata.fieldImdbId" },
  { key: "tmdb_id", labelKey: "metadata.fieldTmdbId" },
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

function formatValue(value: unknown): string | null {
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

function PathValue({ path, className }: { path: string; className?: string }) {
  return (
    <p
      className={cn("min-w-0 max-w-full break-all whitespace-normal font-mono", className)}
      title={path}
    >
      {path}
    </p>
  );
}

function EffectiveTable({
  metadata,
  path,
  density,
}: {
  metadata: MetadataResponse;
  path: string;
  density: "default" | "lg";
}) {
  const { t } = useTranslation();
  const rows = VIEW_FIELDS.flatMap((field) => {
    const value = formatValue(metadata.effective?.[field.key]);
    if (!value) {
      return [];
    }
    return [{ label: t(field.labelKey), value }];
  });

  if (rows.length === 0 && !path) {
    return (
      <p className={cn("text-muted-foreground", density === "lg" ? "text-base" : "text-sm")}>
        {t("metadata.emptyEffective")}
      </p>
    );
  }

  return (
    <Table className="table-fixed">
      <TableHeader>
        <TableRow>
          <TableHead className="w-[40%]">{t("metadata.columnField")}</TableHead>
          <TableHead>{t("metadata.columnValue")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.label}>
            <TableCell className="font-medium whitespace-normal">{row.label}</TableCell>
            <TableCell className="min-w-0 whitespace-normal break-words">{row.value}</TableCell>
          </TableRow>
        ))}
        {path ? (
          <TableRow>
            <TableCell className="align-top font-medium whitespace-normal text-muted-foreground">
              {t("metadata.fieldPath")}
            </TableCell>
            <TableCell className="min-w-0 whitespace-normal">
              <PathValue
                path={path}
                className={
                  density === "lg"
                    ? "text-sm text-muted-foreground"
                    : "text-xs text-muted-foreground"
                }
              />
            </TableCell>
          </TableRow>
        ) : null}
      </TableBody>
    </Table>
  );
}

export function MetadataInspector({
  path,
  editable = false,
  variant = "card",
}: MetadataInspectorProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const metadataQuery = useMetadata(path);
  const isSidebar = variant === "sidebar";
  const controlSize = isSidebar ? "lg" : "default";

  const body = (
    <>
      {metadataQuery.isLoading ? <Skeleton className="h-24 w-full" /> : null}

      {metadataQuery.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {metadataQuery.error instanceof Error
              ? metadataQuery.error.message
              : t("metadata.loadError")}
          </AlertDescription>
        </Alert>
      ) : null}

      {metadataQuery.data ? (
        <>
          <div className="flex min-w-0 flex-col gap-3">
            <p className={cn("text-muted-foreground", isSidebar ? "text-base" : "text-sm")}>
              {metadataQuery.data.uses_metadata_for_display
                ? t("metadata.displayUsesMetadata")
                : t("metadata.displayUsesFilename")}
            </p>

            {editable ? (
              <MetadataEditForm path={path} metadata={metadataQuery.data} size={controlSize} />
            ) : (
              <EffectiveTable
                metadata={metadataQuery.data}
                path={path}
                density={isSidebar ? "lg" : "default"}
              />
            )}

            {metadataQuery.data.probed_at ? (
              <p className={cn("text-muted-foreground", isSidebar ? "text-sm" : "text-xs")}>
                {t("metadata.probedAt", {
                  value: new Date(metadataQuery.data.probed_at).toLocaleString(),
                })}
              </p>
            ) : null}
          </div>
        </>
      ) : path ? (
        <div className="flex min-w-0 flex-col gap-1">
          <p className={cn("font-medium text-muted-foreground", isSidebar ? "text-sm" : "text-xs")}>
            {t("metadata.fieldPath")}
          </p>
          <PathValue path={path} className={isSidebar ? "text-sm" : "text-xs"} />
        </div>
      ) : null}
    </>
  );

  if (isSidebar) {
    return <div className="flex min-w-0 flex-col gap-4">{body}</div>;
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-xl border">
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          size="default"
          className="flex h-auto w-full items-center justify-between rounded-xl px-4 py-3"
        >
          <span className="text-base font-medium">{t("metadata.inspectorTitle")}</span>
          <ChevronDownIcon className={`size-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-4 border-t px-4 py-4">
        {body}
      </CollapsibleContent>
    </Collapsible>
  );
}
