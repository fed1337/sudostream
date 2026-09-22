import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import type { SudoStreamInternalMaintenanceScheduleInput } from "@/client/types.gen";
import { getApiAdminMaintenanceOptions } from "@/client/@tanstack/react-query.gen";
import { patchApiAdminMaintenance, postApiAdminMaintenanceByActionRun } from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { MaintenanceActionCard } from "@/pages/admin/libraries/maintenance-cards";
import { formatFileSize } from "@/lib/media-item";

type ScheduleInput = SudoStreamInternalMaintenanceScheduleInput;

const SCAN_ACTION = "libraries.scan";
const PURGE_ACTION = "playback.cache.purge";
const TRASH_PURGE_ACTION = "trash.purge";

export default function AdminSchedulePage() {
  const { t } = useTranslation();
  const maintenanceQuery = useQuery({
    ...getApiAdminMaintenanceOptions(),
    refetchInterval: (query) => {
      const actions = query.state.data?.actions ?? [];
      return actions.some((action) => action.running) ? 10_000 : false;
    },
  });

  const [scanCron, setScanCron] = useState("");
  const [scanEnabled, setScanEnabled] = useState(false);
  const [purgeCron, setPurgeCron] = useState("");
  const [purgeEnabled, setPurgeEnabled] = useState(false);
  const [trashCron, setTrashCron] = useState("");
  const [trashEnabled, setTrashEnabled] = useState(false);
  const [prevMaintenance, setPrevMaintenance] = useState(maintenanceQuery.data);

  if (maintenanceQuery.data !== prevMaintenance) {
    setPrevMaintenance(maintenanceQuery.data);
    const actions = maintenanceQuery.data?.actions ?? [];
    const scan = actions.find((action) => action.action === SCAN_ACTION);
    const purge = actions.find((action) => action.action === PURGE_ACTION);
    const trashPurge = actions.find((action) => action.action === TRASH_PURGE_ACTION);
    setScanCron(scan?.schedule?.cron ?? "");
    setScanEnabled(Boolean(scan?.schedule?.enabled));
    setPurgeCron(purge?.schedule?.cron ?? "");
    setPurgeEnabled(Boolean(purge?.schedule?.enabled));
    setTrashCron(trashPurge?.schedule?.cron ?? "");
    setTrashEnabled(Boolean(trashPurge?.schedule?.enabled));
  }

  const saveMutation = useMutation({
    mutationFn: async (schedules: ScheduleInput[]) => {
      const response = await patchApiAdminMaintenance({
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

  const runMutation = useMutation({
    mutationFn: async (action: string) => {
      const response = await postApiAdminMaintenanceByActionRun({
        path: { action },
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

  if (maintenanceQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const actions = maintenanceQuery.data?.actions ?? [];
  const scanStatus = actions.find((action) => action.action === SCAN_ACTION);
  const purgeStatus = actions.find((action) => action.action === PURGE_ACTION);
  const trashStatus = actions.find((action) => action.action === TRASH_PURGE_ACTION);
  const cacheBytes = maintenanceQuery.data?.cacheBytes;
  const purgeDescription =
    typeof cacheBytes === "number"
      ? `${t("admin.cachePurgeDescription")} ${t("admin.cachePurgeSize", {
          size: formatFileSize(cacheBytes),
        })}`
      : t("admin.cachePurgeDescription");

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("admin.scheduleTitle")}</h1>
        <p className="text-muted-foreground">{t("admin.scheduleDescription")}</p>
      </div>

      <MaintenanceActionCard
        title={t("admin.librariesScanTitle")}
        description={t("admin.librariesScanDescription")}
        cron={scanCron}
        enabled={scanEnabled}
        status={scanStatus}
        saving={saveMutation.isPending}
        running={Boolean(scanStatus?.running) || runMutation.isPending}
        onCronChange={setScanCron}
        onEnabledChange={setScanEnabled}
        onSave={() => {
          saveMutation.mutate([{ action: SCAN_ACTION, cron: scanCron, enabled: scanEnabled }]);
        }}
        onRun={() => {
          runMutation.mutate(SCAN_ACTION);
        }}
      />

      <MaintenanceActionCard
        title={t("admin.cachePurgeTitle")}
        description={purgeDescription}
        cron={purgeCron}
        enabled={purgeEnabled}
        status={purgeStatus}
        saving={saveMutation.isPending}
        running={Boolean(purgeStatus?.running) || runMutation.isPending}
        onCronChange={setPurgeCron}
        onEnabledChange={setPurgeEnabled}
        onSave={() => {
          saveMutation.mutate([
            {
              action: PURGE_ACTION,
              cron: purgeCron,
              enabled: purgeEnabled,
            },
          ]);
        }}
        onRun={() => {
          runMutation.mutate(PURGE_ACTION);
        }}
      />

      <MaintenanceActionCard
        title={t("admin.trashPurgeTitle")}
        description={t("admin.trashPurgeDescription")}
        cron={trashCron}
        enabled={trashEnabled}
        status={trashStatus}
        saving={saveMutation.isPending}
        running={Boolean(trashStatus?.running) || runMutation.isPending}
        onCronChange={setTrashCron}
        onEnabledChange={setTrashEnabled}
        onSave={() => {
          saveMutation.mutate([
            {
              action: TRASH_PURGE_ACTION,
              cron: trashCron,
              enabled: trashEnabled,
            },
          ]);
        }}
        onRun={() => {
          runMutation.mutate(TRASH_PURGE_ACTION);
        }}
      />

      {saveMutation.isError || runMutation.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>
            {(saveMutation.error ?? runMutation.error) instanceof Error
              ? ((saveMutation.error ?? runMutation.error) as Error).message
              : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}
