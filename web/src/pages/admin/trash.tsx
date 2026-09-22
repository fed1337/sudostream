import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { RotateCcwIcon, Trash2Icon } from "lucide-react";

import {
  getApiAdminTrashOptions,
  getApiAdminTrashSettingsOptions,
} from "@/client/@tanstack/react-query.gen";
import {
  deleteApiAdminTrash,
  deleteApiAdminTrashById,
  patchApiAdminTrashSettings,
  postApiAdminTrashByIdRestore,
} from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
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
import { extractApiErrorMessage } from "@/lib/api-error";

function formatDate(value?: string): string {
  if (!value) {
    return "—";
  }

  return new Date(value).toLocaleString();
}

export default function AdminTrashPage() {
  const { t } = useTranslation();
  const trashQuery = useQuery({
    ...getApiAdminTrashOptions(),
  });
  const settingsQuery = useQuery({
    ...getApiAdminTrashSettingsOptions(),
  });
  const [retentionDays, setRetentionDays] = useState("0");
  const [prevSettings, setPrevSettings] = useState(settingsQuery.data);

  if (settingsQuery.data !== prevSettings) {
    setPrevSettings(settingsQuery.data);
    if (typeof settingsQuery.data?.retentionDays === "number") {
      setRetentionDays(String(settingsQuery.data.retentionDays));
    }
  }

  const restoreMutation = useMutation({
    mutationFn: async (id: string) => {
      const response = await postApiAdminTrashByIdRestore({
        path: { id },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashRestoreFailed"));
      }
    },
    onSuccess: () => {
      void trashQuery.refetch();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const response = await deleteApiAdminTrashById({
        path: { id },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashDeleteFailed"));
      }
    },
    onSuccess: () => {
      void trashQuery.refetch();
    },
  });

  const emptyMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteApiAdminTrash();
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashEmptyFailed"));
      }
    },
    onSuccess: () => {
      void trashQuery.refetch();
    },
  });

  const saveSettingsMutation = useMutation({
    mutationFn: async (days: number) => {
      const response = await patchApiAdminTrashSettings({
        body: { retentionDays: days },
      });
      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashSettingsFailed"));
      }
      return response.data;
    },
    onSuccess: () => {
      void settingsQuery.refetch();
    },
  });

  if (trashQuery.isLoading || settingsQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const items = trashQuery.data?.items ?? [];
  const busy =
    restoreMutation.isPending ||
    deleteMutation.isPending ||
    emptyMutation.isPending ||
    saveSettingsMutation.isPending;
  const error =
    restoreMutation.error ??
    deleteMutation.error ??
    emptyMutation.error ??
    saveSettingsMutation.error ??
    trashQuery.error;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t("admin.trashTitle")}</h1>
          <p className="text-muted-foreground">{t("admin.trashDescription")}</p>
        </div>
        <Button
          variant="destructive"
          disabled={busy || items.length === 0}
          onClick={() => {
            emptyMutation.mutate();
          }}
        >
          {emptyMutation.isPending ? <Spinner /> : null}
          {t("admin.trashEmpty")}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("admin.trashRetentionTitle")}</CardTitle>
          <CardDescription>{t("admin.trashRetentionDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="retention-days">{t("admin.trashRetentionDays")}</FieldLabel>
              <div className="flex max-w-sm items-center gap-2">
                <Input
                  id="retention-days"
                  type="number"
                  min={0}
                  value={retentionDays}
                  onChange={(event) => {
                    setRetentionDays(event.target.value);
                  }}
                />
                <Button
                  disabled={busy}
                  onClick={() => {
                    const parsed = Number.parseInt(retentionDays, 10);
                    saveSettingsMutation.mutate(Number.isFinite(parsed) ? parsed : 0);
                  }}
                >
                  {saveSettingsMutation.isPending ? <Spinner /> : null}
                  {t("common.save")}
                </Button>
              </div>
              <FieldDescription>{t("admin.trashRetentionHint")}</FieldDescription>
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}

      {items.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Trash2Icon />
            </EmptyMedia>
            <EmptyTitle>{t("admin.trashEmptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("admin.trashEmptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.trashPath")}</TableHead>
              <TableHead>{t("admin.trashDeletedAt")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => (
              <TableRow key={item.id}>
                <TableCell className="font-medium">{item.originalRelPath}</TableCell>
                <TableCell>{formatDate(item.deletedAt)}</TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() => {
                        if (item.id) {
                          restoreMutation.mutate(item.id);
                        }
                      }}
                    >
                      <RotateCcwIcon />
                      {t("admin.trashRestore")}
                    </Button>
                    <Button
                      variant="destructive"
                      disabled={busy}
                      onClick={() => {
                        if (item.id) {
                          deleteMutation.mutate(item.id);
                        }
                      }}
                    >
                      {t("admin.trashDeleteForever")}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
