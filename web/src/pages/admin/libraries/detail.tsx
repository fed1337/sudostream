import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeftIcon, FolderPlusIcon, Trash2Icon } from "lucide-react";

import type {
  InternalHttpapiProviderAvailability,
  SudoStreamInternalMaintenanceScheduleInput,
} from "@/client/types.gen";
import {
  getApiAdminFolderTreeQueryKey,
  getApiAdminLibrariesByIdMaintenanceOptions,
  getApiAdminLibrariesByIdProvidersOptions,
  getApiAdminLibrariesOptions,
} from "@/client/@tanstack/react-query.gen";
import {
  deleteApiAdminLibrariesById,
  deleteApiAdminLibrariesByIdRoots,
  patchApiAdminLibrariesById,
  patchApiAdminLibrariesByIdMaintenance,
  patchApiAdminLibrariesByIdProviders,
  postApiAdminLibrariesByIdRoots,
  postApiAdminMaintenanceByActionRun,
} from "@/client/sdk.gen";
import type { LibraryType } from "@/api/media";
import { libraryRoots } from "@/api/media";
import { ConfirmActionButton } from "@/components/confirm-action-button";
import { FolderTreePicker } from "@/components/folder-tree-picker";
import { LibraryTypeBadge } from "@/components/library-type-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { extractApiErrorMessage } from "@/lib/api-error";
import { LIBRARY_TYPES } from "@/lib/library-types";
import {
  LIBRARY_CARD_TITLE_CLASS,
  MaintenanceActionCard,
  ProviderTaskCard,
} from "@/pages/admin/libraries/maintenance-cards";

type ScheduleInput = SudoStreamInternalMaintenanceScheduleInput;
type ProviderAvailability = InternalHttpapiProviderAvailability;

const METADATA_ACTION = "metadata.scan";
const THUMBS_ACTION = "thumbnails.warm";
const PROVIDERS_METADATA_ACTION = "providers.metadata";
const PROVIDERS_POSTERS_ACTION = "providers.posters";
const PROVIDERS_SUBTITLES_ACTION = "providers.subtitles";
const NONE_PROVIDER = "none";
const PROVIDER_LIBRARY_TYPES: LibraryType[] = ["film", "series"];

function validateSchedule(enabled: boolean, cron: string): string | null {
  if (enabled && cron.trim() === "") {
    return "admin.maintenanceCronRequired";
  }
  return null;
}

function validateProviderSettings(
  subtitleProvider: string,
  subtitleLanguages: string[],
): string | null {
  if (subtitleProvider !== NONE_PROVIDER && subtitleLanguages.length === 0) {
    return "admin.providersSubtitleLanguagesRequired";
  }
  return null;
}

export { MaintenanceActionCard } from "@/pages/admin/libraries/maintenance-cards";

export default function AdminLibraryDetailPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { id = "" } = useParams();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerSelected, setPickerSelected] = useState<string[]>([]);

  const [metadataValidationError, setMetadataValidationError] = useState<string | null>(null);
  const [thumbsValidationError, setThumbsValidationError] = useState<string | null>(null);
  const [providersMetadataValidationError, setProvidersMetadataValidationError] = useState<
    string | null
  >(null);
  const [providersPostersValidationError, setProvidersPostersValidationError] = useState<
    string | null
  >(null);
  const [providersSubtitlesValidationError, setProvidersSubtitlesValidationError] = useState<
    string | null
  >(null);

  const librariesQuery = useQuery({
    ...getApiAdminLibrariesOptions(),
  });
  const maintenanceQuery = useQuery({
    ...getApiAdminLibrariesByIdMaintenanceOptions({ path: { id } }),
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const actions = query.state.data?.actions ?? [];
      return actions.some((action) => action.running) ? 10_000 : false;
    },
  });

  const library = (librariesQuery.data?.libraries ?? []).find((item) => item.id === id);
  const supportsProviders = PROVIDER_LIBRARY_TYPES.includes(
    (library?.type ?? "other") as LibraryType,
  );

  const providersQuery = useQuery({
    ...getApiAdminLibrariesByIdProvidersOptions({ path: { id } }),
    enabled: Boolean(id) && supportsProviders,
  });

  const [name, setName] = useState("");
  const [type, setType] = useState<LibraryType>("other");
  const [metadataCron, setMetadataCron] = useState("");
  const [metadataEnabled, setMetadataEnabled] = useState(false);
  const [thumbsCron, setThumbsCron] = useState("");
  const [thumbsEnabled, setThumbsEnabled] = useState(false);
  const [providersMetadataCron, setProvidersMetadataCron] = useState("");
  const [providersMetadataEnabled, setProvidersMetadataEnabled] = useState(false);
  const [providersPostersCron, setProvidersPostersCron] = useState("");
  const [providersPostersEnabled, setProvidersPostersEnabled] = useState(false);
  const [providersSubtitlesCron, setProvidersSubtitlesCron] = useState("");
  const [providersSubtitlesEnabled, setProvidersSubtitlesEnabled] = useState(false);

  const [metadataProvider, setMetadataProvider] = useState(NONE_PROVIDER);
  const [posterProvider, setPosterProvider] = useState(NONE_PROVIDER);
  const [subtitleProvider, setSubtitleProvider] = useState(NONE_PROVIDER);
  const [subtitleLanguages, setSubtitleLanguages] = useState<string[]>([]);
  const [allowOverrideUserMetadata, setAllowOverrideUserMetadata] = useState(false);
  const [metadataApplyMode, setMetadataApplyMode] = useState("fill_missing");
  const [metadataWriteTarget, setMetadataWriteTarget] = useState("db");
  const [hydratedLibraryId, setHydratedLibraryId] = useState<string | null>(null);
  const [hydratedMaintenanceId, setHydratedMaintenanceId] = useState<string | null>(null);
  const [hydratedProvidersId, setHydratedProvidersId] = useState<string | null>(null);

  // Hydrate once per library id. Object-identity sync skips when the list query is
  // already cached on first paint (common when navigating from /admin/libraries).
  if (library?.id === id && hydratedLibraryId !== id) {
    setHydratedLibraryId(id);
    setName(library.name?.trim() || library.slug || "");
    setType((library.type ?? "other") as LibraryType);
  }

  if (maintenanceQuery.data && hydratedMaintenanceId !== id) {
    setHydratedMaintenanceId(id);
    const actions = maintenanceQuery.data.actions ?? [];
    const metadata = actions.find((action) => action.action === METADATA_ACTION);
    const thumbs = actions.find((action) => action.action === THUMBS_ACTION);
    const providersMetadata = actions.find((action) => action.action === PROVIDERS_METADATA_ACTION);
    const providersPosters = actions.find((action) => action.action === PROVIDERS_POSTERS_ACTION);
    const providersSubtitles = actions.find(
      (action) => action.action === PROVIDERS_SUBTITLES_ACTION,
    );
    setMetadataCron(metadata?.schedule?.cron ?? "");
    setMetadataEnabled(Boolean(metadata?.schedule?.enabled));
    setThumbsCron(thumbs?.schedule?.cron ?? "");
    setThumbsEnabled(Boolean(thumbs?.schedule?.enabled));
    setProvidersMetadataCron(providersMetadata?.schedule?.cron ?? "");
    setProvidersMetadataEnabled(Boolean(providersMetadata?.schedule?.enabled));
    setProvidersPostersCron(providersPosters?.schedule?.cron ?? "");
    setProvidersPostersEnabled(Boolean(providersPosters?.schedule?.enabled));
    setProvidersSubtitlesCron(providersSubtitles?.schedule?.cron ?? "");
    setProvidersSubtitlesEnabled(Boolean(providersSubtitles?.schedule?.enabled));
  }

  if (providersQuery.data && hydratedProvidersId !== id) {
    setHydratedProvidersId(id);
    const settings = providersQuery.data.settings;
    if (settings) {
      setMetadataProvider(settings.metadataProvider ?? NONE_PROVIDER);
      setPosterProvider(settings.posterProvider ?? NONE_PROVIDER);
      setSubtitleProvider(settings.subtitleProvider ?? NONE_PROVIDER);
      setSubtitleLanguages(settings.subtitleLanguages ?? []);
      setAllowOverrideUserMetadata(Boolean(settings.allowOverrideUserMetadata));
      setMetadataApplyMode(settings.metadataApplyMode ?? "fill_missing");
      setMetadataWriteTarget(settings.metadataWriteTarget ?? "db");
    }
  }

  const saveLibraryMutation = useMutation({
    mutationFn: async () => {
      const response = await patchApiAdminLibrariesById({
        path: { id },
        body: { name, type },
      });
      if (response.error || !response.data) {
        throw new Error(t("admin.updateLibraryFailed"));
      }
      return response.data.library;
    },
    onSuccess: () => {
      void librariesQuery.refetch();
    },
  });

  const saveScheduleMutation = useMutation({
    mutationFn: async (schedules: ScheduleInput[]) => {
      const response = await patchApiAdminLibrariesByIdMaintenance({
        path: { id },
        body: { schedules },
      });
      if (response.error || !response.data) {
        throw new Error(t("admin.maintenanceSaveFailed"));
      }
      return response.data;
    },
    onSuccess: () => {
      void maintenanceQuery.refetch();
    },
  });

  const saveProvidersMutation = useMutation({
    mutationFn: async () => {
      const response = await patchApiAdminLibrariesByIdProviders({
        path: { id },
        body: {
          metadataProvider: metadataProvider === NONE_PROVIDER ? undefined : metadataProvider,
          posterProvider: posterProvider === NONE_PROVIDER ? undefined : posterProvider,
          subtitleProvider: subtitleProvider === NONE_PROVIDER ? undefined : subtitleProvider,
          subtitleLanguages,
          allowOverrideUserMetadata,
          metadataApplyMode,
          metadataWriteTarget,
        },
      });
      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.providersSaveFailed"));
      }
      return response.data;
    },
    onSuccess: () => {
      void providersQuery.refetch();
    },
  });

  const runMutation = useMutation({
    mutationFn: async (action: string) => {
      const response = await postApiAdminMaintenanceByActionRun({
        path: { action },
        query: { libraryId: id },
      });
      if (response.error || !response.data) {
        throw new Error(t("admin.maintenanceRunFailed"));
      }
      return response.data;
    },
    onSuccess: () => {
      void maintenanceQuery.refetch();
    },
  });

  const addRootMutation = useMutation({
    mutationFn: async (relPaths: string[]) => {
      for (const relPath of relPaths) {
        const response = await postApiAdminLibrariesByIdRoots({
          path: { id },
          body: { relPath },
        });
        if (response.error || !response.data) {
          throw new Error(extractApiErrorMessage(response.error) ?? t("admin.addFolderFailed"));
        }
      }
    },
    onSuccess: () => {
      setPickerOpen(false);
      setPickerSelected([]);
      void librariesQuery.refetch();
      void queryClient.invalidateQueries({ queryKey: getApiAdminFolderTreeQueryKey() });
    },
  });

  const removeRootMutation = useMutation({
    mutationFn: async (relPath: string) => {
      const response = await deleteApiAdminLibrariesByIdRoots({
        path: { id },
        query: { path: relPath },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.removeFolderFailed"));
      }
    },
    onSuccess: () => {
      void librariesQuery.refetch();
    },
  });

  const deleteLibraryMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteApiAdminLibrariesById({
        path: { id },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.deleteLibraryFailed"));
      }
    },
    onSuccess: () => {
      void navigate("/admin/libraries");
    },
  });

  const saving =
    saveScheduleMutation.isPending ||
    saveProvidersMutation.isPending ||
    saveLibraryMutation.isPending;
  const running = runMutation.isPending;

  const saveMaintenance = async (
    schedule: ScheduleInput,
    setValidationError: (value: string | null) => void,
    alsoSaveProviders: boolean,
  ) => {
    const scheduleKey = validateSchedule(Boolean(schedule.enabled), schedule.cron ?? "");
    if (scheduleKey) {
      setValidationError(t(scheduleKey));
      return;
    }

    if (alsoSaveProviders) {
      const providerKey = validateProviderSettings(subtitleProvider, subtitleLanguages);
      if (providerKey) {
        setValidationError(t(providerKey));
        return;
      }
    }

    setValidationError(null);

    try {
      if (alsoSaveProviders) {
        await saveProvidersMutation.mutateAsync();
      }
      await saveScheduleMutation.mutateAsync([schedule]);
    } catch (error) {
      setValidationError(error instanceof Error ? error.message : t("common.error"));
    }
  };

  if (librariesQuery.isLoading || maintenanceQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  if (!library) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("admin.libraryNotFound")}</AlertTitle>
      </Alert>
    );
  }

  const actions = maintenanceQuery.data?.actions ?? [];
  const metadataStatus = actions.find((action) => action.action === METADATA_ACTION);
  const thumbsStatus = actions.find((action) => action.action === THUMBS_ACTION);
  const providersMetadataStatus = actions.find(
    (action) => action.action === PROVIDERS_METADATA_ACTION,
  );
  const providersPostersStatus = actions.find(
    (action) => action.action === PROVIDERS_POSTERS_ACTION,
  );
  const providersSubtitlesStatus = actions.find(
    (action) => action.action === PROVIDERS_SUBTITLES_ACTION,
  );
  const available: ProviderAvailability = providersQuery.data?.available ?? {
    metadata: [],
    poster: [],
    subtitle: [],
  };
  const heading = library.name ?? library.slug;
  const roots = libraryRoots(library);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <Button variant="ghost" className="w-fit px-0" asChild>
          <Link to="/admin/libraries">
            <ArrowLeftIcon data-icon="inline-start" />
            {t("admin.backToLibraries")}
          </Link>
        </Button>
        <h1 className="text-2xl font-semibold tracking-tight">{heading}</h1>
        <p className="text-muted-foreground">{roots.join(", ") || t("admin.noFolders")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>
            {t("admin.librarySettingsTitle")}
          </CardTitle>
          <CardDescription>{t("admin.librarySettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <FieldGroup>
            <Field>
              <FieldLabel>{t("admin.name")}</FieldLabel>
              <Input
                value={name}
                onChange={(event) => {
                  setName(event.target.value);
                }}
              />
              <FieldDescription>{t("admin.libraryNameHelp")}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel>{t("admin.type")}</FieldLabel>
              <Select
                value={type}
                onValueChange={(value) => {
                  setType(value as LibraryType);
                }}
              >
                <SelectTrigger className="w-48">
                  <SelectValue>
                    <LibraryTypeBadge type={type} />
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {LIBRARY_TYPES.map((libraryType) => (
                      <SelectItem key={libraryType} value={libraryType}>
                        <LibraryTypeBadge type={libraryType} />
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          </FieldGroup>
          <Button
            className="w-fit"
            disabled={saveLibraryMutation.isPending || !name.trim()}
            onClick={() => {
              saveLibraryMutation.mutate();
            }}
          >
            {saveLibraryMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
            {t("admin.saveLibrary")}
          </Button>
          {saveLibraryMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("admin.updateLibraryFailed")}</AlertTitle>
              <AlertDescription>
                {saveLibraryMutation.error instanceof Error
                  ? saveLibraryMutation.error.message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={LIBRARY_CARD_TITLE_CLASS}>
            {t("admin.libraryFoldersTitle")}
          </CardTitle>
          <CardDescription>{t("admin.libraryFoldersDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {roots.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("admin.noFolders")}</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {roots.map((root) => (
                <li key={root} className="flex items-center justify-between gap-3">
                  <Badge variant="outline">{root}</Badge>
                  <Button
                    variant="outline"
                    disabled={removeRootMutation.isPending}
                    onClick={() => {
                      removeRootMutation.mutate(root);
                    }}
                  >
                    {t("admin.removeFolder")}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <Button
            className="w-fit"
            variant="outline"
            onClick={() => {
              setPickerSelected([]);
              setPickerOpen(true);
            }}
          >
            <FolderPlusIcon data-icon="inline-start" />
            {t("admin.addFolder")}
          </Button>
          {addRootMutation.isError || removeRootMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("common.error")}</AlertTitle>
              <AlertDescription>
                {(addRootMutation.error ?? removeRootMutation.error) instanceof Error
                  ? ((addRootMutation.error ?? removeRootMutation.error) as Error).message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
      </Card>

      <Dialog
        open={pickerOpen}
        onOpenChange={(open) => {
          setPickerOpen(open);
          if (!open) {
            setPickerSelected([]);
            addRootMutation.reset();
          }
        }}
      >
        <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("admin.folderPickerTitle")}</DialogTitle>
            <DialogDescription>{t("admin.folderPickerDescription")}</DialogDescription>
          </DialogHeader>
          <div className="flex min-h-0 flex-col gap-3">
            {pickerOpen ? (
              <FolderTreePicker
                currentLibraryId={id}
                selected={pickerSelected}
                onSelectedChange={setPickerSelected}
              />
            ) : null}
            {pickerSelected.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {pickerSelected.map((root) => (
                  <Badge key={root} variant="secondary">
                    {root}
                  </Badge>
                ))}
              </div>
            ) : null}
            {addRootMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.addFolderFailed")}</AlertTitle>
                <AlertDescription>
                  {addRootMutation.error instanceof Error
                    ? addRootMutation.error.message
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              disabled={pickerSelected.length === 0 || addRootMutation.isPending}
              onClick={() => {
                addRootMutation.mutate(pickerSelected);
              }}
            >
              {addRootMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {t("admin.addFolder")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <MaintenanceActionCard
        title={t("admin.metadataScanTitle")}
        description={t("admin.metadataScanDescription")}
        cron={metadataCron}
        enabled={metadataEnabled}
        status={metadataStatus}
        saving={saving}
        running={Boolean(metadataStatus?.running) || running}
        validationError={metadataValidationError}
        onCronChange={setMetadataCron}
        onEnabledChange={setMetadataEnabled}
        onSave={() => {
          void saveMaintenance(
            {
              action: METADATA_ACTION,
              cron: metadataCron,
              enabled: metadataEnabled,
            },
            setMetadataValidationError,
            false,
          );
        }}
        onRun={() => {
          runMutation.mutate(METADATA_ACTION);
        }}
      />

      <MaintenanceActionCard
        title={t("admin.thumbnailsWarmTitle")}
        description={t("admin.thumbnailsWarmDescription")}
        cron={thumbsCron}
        enabled={thumbsEnabled}
        status={thumbsStatus}
        saving={saving}
        running={Boolean(thumbsStatus?.running) || running}
        validationError={thumbsValidationError}
        onCronChange={setThumbsCron}
        onEnabledChange={setThumbsEnabled}
        onSave={() => {
          void saveMaintenance(
            {
              action: THUMBS_ACTION,
              cron: thumbsCron,
              enabled: thumbsEnabled,
            },
            setThumbsValidationError,
            false,
          );
        }}
        onRun={() => {
          runMutation.mutate(THUMBS_ACTION);
        }}
      />

      {supportsProviders ? (
        <ProviderTaskCard
          title={t("admin.providersMetadataCardTitle")}
          description={t("admin.providersMetadataCardDescription")}
          providerLabel={t("admin.providersMetadataSlot")}
          providerValue={metadataProvider}
          providerOptions={available.metadata ?? []}
          noneLabel={t("admin.providersNone")}
          cron={providersMetadataCron}
          enabled={providersMetadataEnabled}
          status={providersMetadataStatus}
          saving={saving}
          running={Boolean(providersMetadataStatus?.running) || running}
          validationError={providersMetadataValidationError}
          onProviderChange={setMetadataProvider}
          onCronChange={setProvidersMetadataCron}
          onEnabledChange={setProvidersMetadataEnabled}
          metadataWriteTarget={metadataWriteTarget}
          metadataApplyMode={metadataApplyMode}
          allowOverrideUserMetadata={allowOverrideUserMetadata}
          onMetadataWriteTargetChange={setMetadataWriteTarget}
          onMetadataApplyModeChange={setMetadataApplyMode}
          onAllowOverrideUserMetadataChange={setAllowOverrideUserMetadata}
          onSave={() => {
            void saveMaintenance(
              {
                action: PROVIDERS_METADATA_ACTION,
                cron: providersMetadataCron,
                enabled: providersMetadataEnabled,
              },
              setProvidersMetadataValidationError,
              true,
            );
          }}
          onRun={() => {
            runMutation.mutate(PROVIDERS_METADATA_ACTION);
          }}
        />
      ) : null}

      {supportsProviders ? (
        <ProviderTaskCard
          title={t("admin.providersPosterCardTitle")}
          description={t("admin.providersPosterCardDescription")}
          providerLabel={t("admin.providersPosterSlot")}
          providerValue={posterProvider}
          providerOptions={available.poster ?? []}
          noneLabel={t("admin.providersNone")}
          cron={providersPostersCron}
          enabled={providersPostersEnabled}
          status={providersPostersStatus}
          saving={saving}
          running={Boolean(providersPostersStatus?.running) || running}
          validationError={providersPostersValidationError}
          onProviderChange={setPosterProvider}
          onCronChange={setProvidersPostersCron}
          onEnabledChange={setProvidersPostersEnabled}
          onSave={() => {
            void saveMaintenance(
              {
                action: PROVIDERS_POSTERS_ACTION,
                cron: providersPostersCron,
                enabled: providersPostersEnabled,
              },
              setProvidersPostersValidationError,
              true,
            );
          }}
          onRun={() => {
            runMutation.mutate(PROVIDERS_POSTERS_ACTION);
          }}
        />
      ) : null}

      {supportsProviders ? (
        <ProviderTaskCard
          title={t("admin.providersSubtitlesCardTitle")}
          description={t("admin.providersSubtitlesCardDescription")}
          providerLabel={t("admin.providersSubtitleSlot")}
          providerValue={subtitleProvider}
          providerOptions={available.subtitle ?? []}
          noneLabel={t("admin.providersNone")}
          cron={providersSubtitlesCron}
          enabled={providersSubtitlesEnabled}
          status={providersSubtitlesStatus}
          saving={saving}
          running={Boolean(providersSubtitlesStatus?.running) || running}
          validationError={providersSubtitlesValidationError}
          onProviderChange={(value) => {
            setSubtitleProvider(value);
            if (value === NONE_PROVIDER) {
              setSubtitleLanguages([]);
            }
          }}
          onCronChange={setProvidersSubtitlesCron}
          onEnabledChange={setProvidersSubtitlesEnabled}
          subtitleLanguages={subtitleLanguages}
          onSubtitleLanguagesChange={setSubtitleLanguages}
          onSave={() => {
            void saveMaintenance(
              {
                action: PROVIDERS_SUBTITLES_ACTION,
                cron: providersSubtitlesCron,
                enabled: providersSubtitlesEnabled,
              },
              setProvidersSubtitlesValidationError,
              true,
            );
          }}
          onRun={() => {
            runMutation.mutate(PROVIDERS_SUBTITLES_ACTION);
          }}
        />
      ) : null}

      {saveScheduleMutation.isError || runMutation.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>
            {(saveScheduleMutation.error ?? runMutation.error) instanceof Error
              ? ((saveScheduleMutation.error ?? runMutation.error) as Error).message
              : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}

      <ConfirmActionButton
        label={t("admin.deleteLibrary")}
        title={t("admin.deleteLibraryTitle")}
        description={t("admin.deleteLibraryDescription")}
        variant="destructive"
        pending={deleteLibraryMutation.isPending}
        onConfirm={() => {
          deleteLibraryMutation.mutate();
        }}
      >
        <Trash2Icon data-icon="inline-start" />
      </ConfirmActionButton>
    </div>
  );
}
