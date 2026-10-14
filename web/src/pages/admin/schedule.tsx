import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { HistoryIcon } from "lucide-react";

import type { SudoStreamInternalMaintenanceScheduleInput } from "@/client/types.gen";
import {
  getApiAdminMaintenanceOptions,
  getApiAdminMaintenanceRunsOptions,
  getApiAdminMaintenanceRunsQueryKey,
} from "@/client/@tanstack/react-query.gen";
import {
  patchApiAdminMaintenance,
  postApiAdminMaintenanceByActionRun,
  postApiAdminMaintenanceRunsByIdCancel,
} from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MaintenanceActionCard } from "@/pages/admin/libraries/maintenance-cards";
import { formatFileSize } from "@/lib/media-item";

type ScheduleInput = SudoStreamInternalMaintenanceScheduleInput;

const SCAN_ACTION = "libraries.scan";
const PURGE_ACTION = "playback.cache.purge";
const TRASH_PURGE_ACTION = "trash.purge";
const RUNS_LIMIT = 30;

export default function AdminSchedulePage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const maintenanceQuery = useQuery({
    ...getApiAdminMaintenanceOptions(),
    refetchInterval: (query) => {
      const actions = query.state.data?.actions ?? [];
      return actions.some((action) => action.running) ? 10_000 : false;
    },
  });
  const runsQuery = useQuery({
    ...getApiAdminMaintenanceRunsOptions({ query: { limit: RUNS_LIMIT } }),
    refetchInterval: (query) => {
      const runs = query.state.data?.runs ?? [];
      return runs.some((run) => run.status === "running") ? 10_000 : false;
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

  const invalidateRuns = () => {
    void queryClient.invalidateQueries({ queryKey: getApiAdminMaintenanceRunsQueryKey() });
    void maintenanceQuery.refetch();
    void runsQuery.refetch();
  };

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
      invalidateRuns();
    },
  });

  const cancelMutation = useMutation({
    mutationFn: async (runId: string) => {
      const response = await postApiAdminMaintenanceRunsByIdCancel({
        path: { id: runId },
      });
      if (response.error) {
        throw new Error(t("admin.maintenanceCancelFailed"));
      }
    },
    onSuccess: () => {
      invalidateRuns();
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
  const runs = runsQuery.data?.runs ?? [];
  const cancelingId = cancelMutation.isPending ? cancelMutation.variables : undefined;

  const cancelForStatus = (status?: (typeof actions)[number]) => {
    const runId = status?.running ? status.latestRun?.id : undefined;
    if (!runId) {
      return undefined;
    }

    return () => {
      cancelMutation.mutate(runId);
    };
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("admin.scheduleTitle")}</h1>
        <p className="text-muted-foreground">{t("admin.scheduleDescription")}</p>
      </div>

      <Tabs defaultValue="global">
        <TabsList variant="line">
          <TabsTrigger value="global">{t("admin.scheduleTabGlobal")}</TabsTrigger>
          <TabsTrigger value="history">{t("admin.scheduleTabHistory")}</TabsTrigger>
        </TabsList>

        <TabsContent value="global" className="flex flex-col gap-6 pt-4">
          <MaintenanceActionCard
            title={t("admin.librariesScanTitle")}
            description={t("admin.librariesScanDescription")}
            cron={scanCron}
            enabled={scanEnabled}
            status={scanStatus}
            saving={saveMutation.isPending}
            running={Boolean(scanStatus?.running) || runMutation.isPending}
            canceling={cancelMutation.isPending}
            onCronChange={setScanCron}
            onEnabledChange={setScanEnabled}
            onSave={() => {
              saveMutation.mutate([{ action: SCAN_ACTION, cron: scanCron, enabled: scanEnabled }]);
            }}
            onRun={() => {
              runMutation.mutate(SCAN_ACTION);
            }}
            onCancel={cancelForStatus(scanStatus)}
          />

          <MaintenanceActionCard
            title={t("admin.cachePurgeTitle")}
            description={purgeDescription}
            cron={purgeCron}
            enabled={purgeEnabled}
            status={purgeStatus}
            saving={saveMutation.isPending}
            running={Boolean(purgeStatus?.running) || runMutation.isPending}
            canceling={cancelMutation.isPending}
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
            onCancel={cancelForStatus(purgeStatus)}
          />

          <MaintenanceActionCard
            title={t("admin.trashPurgeTitle")}
            description={t("admin.trashPurgeDescription")}
            cron={trashCron}
            enabled={trashEnabled}
            status={trashStatus}
            saving={saveMutation.isPending}
            running={Boolean(trashStatus?.running) || runMutation.isPending}
            canceling={cancelMutation.isPending}
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
            onCancel={cancelForStatus(trashStatus)}
          />

          {saveMutation.isError || runMutation.isError || cancelMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("common.error")}</AlertTitle>
              <AlertDescription>
                {(saveMutation.error ?? runMutation.error ?? cancelMutation.error) instanceof Error
                  ? ((saveMutation.error ?? runMutation.error ?? cancelMutation.error) as Error)
                      .message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
        </TabsContent>

        <TabsContent value="history" className="flex flex-col gap-4 pt-4">
          <p className="text-sm text-muted-foreground">{t("admin.maintenanceRunsDescription")}</p>
          {runsQuery.isLoading ? (
            <Skeleton className="h-40 w-full rounded-xl" />
          ) : runs.length === 0 ? (
            <Empty className="border">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <HistoryIcon />
                </EmptyMedia>
                <EmptyTitle>{t("admin.maintenanceRunsEmptyTitle")}</EmptyTitle>
                <EmptyDescription>{t("admin.maintenanceRunsEmpty")}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="overflow-hidden rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("admin.maintenanceRunsAction")}</TableHead>
                    <TableHead>{t("admin.maintenanceRunsLibrary")}</TableHead>
                    <TableHead>{t("admin.maintenanceRunsStatus")}</TableHead>
                    <TableHead>{t("admin.maintenanceRunsStarted")}</TableHead>
                    <TableHead>{t("admin.maintenanceRunsFinished")}</TableHead>
                    <TableHead className="w-[1%]">{t("admin.maintenanceRunsActions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {runs.map((run) => (
                    <TableRow key={run.id}>
                      <TableCell className="font-mono text-sm">{run.action}</TableCell>
                      <TableCell>
                        {run.libraryName ||
                          (run.libraryId ? run.libraryId : t("admin.maintenanceRunsSystem"))}
                      </TableCell>
                      <TableCell>{run.status}</TableCell>
                      <TableCell className="whitespace-nowrap text-sm">
                        {run.startedAt ?? "—"}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-sm">
                        {run.finishedAt ?? "—"}
                      </TableCell>
                      <TableCell>
                        {run.status === "running" && run.id ? (
                          <Button
                            variant="destructive"
                            disabled={cancelMutation.isPending}
                            onClick={() => {
                              cancelMutation.mutate(run.id!);
                            }}
                          >
                            {cancelingId === run.id ? <Spinner data-icon="inline-start" /> : null}
                            {t("admin.maintenanceCancel")}
                          </Button>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}

          {cancelMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("common.error")}</AlertTitle>
              <AlertDescription>
                {cancelMutation.error instanceof Error
                  ? cancelMutation.error.message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
        </TabsContent>
      </Tabs>
    </div>
  );
}
