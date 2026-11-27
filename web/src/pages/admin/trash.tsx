import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Columns3Icon, FileIcon, FolderIcon, RotateCcwIcon, Trash2Icon } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";

import { getApiAdminTrashOptions } from "@/client/@tanstack/react-query.gen";
import {
  deleteApiAdminTrash,
  deleteApiAdminTrashById,
  postApiAdminTrashByIdRestore,
  postApiAdminTrashPurge,
  postApiAdminTrashRestore,
  postApiAdminTrashRestoreAll,
} from "@/client/sdk.gen";
import type { InternalHttpapiTrashItemView } from "@/client/types.gen";
import { ConfirmActionButton } from "@/components/confirm-action-button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DataTable,
  type RowSelectionState,
  type VisibilityState,
} from "@/components/ui/data-table";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { extractApiErrorMessage, translateApiError } from "@/lib/api-error";
import { formatFileSize } from "@/lib/media-item";

type TrashRow = InternalHttpapiTrashItemView & { id: string };

function formatDate(value?: string): string {
  if (!value) {
    return "—";
  }

  return new Date(value).toLocaleString();
}

function selectedIds(selection: RowSelectionState): string[] {
  return Object.keys(selection).filter((id) => selection[id]);
}

export default function AdminTrashPage() {
  const { t } = useTranslation();
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({});

  const trashQuery = useQuery({
    ...getApiAdminTrashOptions(),
  });

  const items = useMemo<TrashRow[]>(
    () =>
      (trashQuery.data?.items ?? []).flatMap((item) => (item.id ? [{ ...item, id: item.id }] : [])),
    [trashQuery.data?.items],
  );

  const clearSelection = () => {
    setRowSelection({});
  };

  const restoreOneMutation = useMutation({
    mutationFn: async (id: string) => {
      const response = await postApiAdminTrashByIdRestore({
        path: { id },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashRestoreFailed"));
      }
    },
    onSuccess: () => {
      clearSelection();
      void trashQuery.refetch();
    },
  });

  const deleteOneMutation = useMutation({
    mutationFn: async (id: string) => {
      const response = await deleteApiAdminTrashById({
        path: { id },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashDeleteFailed"));
      }
    },
    onSuccess: () => {
      clearSelection();
      void trashQuery.refetch();
    },
  });

  const restoreManyMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      const response = await postApiAdminTrashRestore({
        body: { ids },
      });
      if (response.error) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.trashRestoreManyFailed"),
        );
      }
      const failed = response.data?.failed ?? [];
      if (failed.length > 0) {
        const first = failed[0]?.error;
        throw new Error(first ?? t("admin.trashRestoreManyFailed"));
      }
    },
    onSuccess: () => {
      clearSelection();
      void trashQuery.refetch();
    },
  });

  const purgeManyMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      const response = await postApiAdminTrashPurge({
        body: { ids },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashDeleteManyFailed"));
      }
      const failed = response.data?.failed ?? [];
      if (failed.length > 0) {
        const first = failed[0]?.error;
        throw new Error(first ?? t("admin.trashDeleteManyFailed"));
      }
    },
    onSuccess: () => {
      clearSelection();
      void trashQuery.refetch();
    },
  });

  const restoreAllMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAdminTrashRestoreAll();
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashRestoreAllFailed"));
      }
      const failed = response.data?.failed ?? [];
      if (failed.length > 0) {
        const first = failed[0]?.error;
        throw new Error(first ?? t("admin.trashRestoreAllFailed"));
      }
    },
    onSuccess: () => {
      clearSelection();
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
      clearSelection();
      void trashQuery.refetch();
    },
  });

  const busy =
    restoreOneMutation.isPending ||
    deleteOneMutation.isPending ||
    restoreManyMutation.isPending ||
    purgeManyMutation.isPending ||
    restoreAllMutation.isPending ||
    emptyMutation.isPending;

  const columns = useMemo<ColumnDef<TrashRow>[]>(
    () => [
      {
        id: "select",
        enableHiding: false,
        header: ({ table }) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected()
                ? true
                : table.getIsSomePageRowsSelected()
                  ? "indeterminate"
                  : false
            }
            onCheckedChange={(value) => {
              table.toggleAllPageRowsSelected(!!value);
            }}
            aria-label={t("admin.trashSelectAll")}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => {
              row.toggleSelected(!!value);
            }}
            aria-label={t("admin.trashSelectRow")}
          />
        ),
      },
      {
        accessorKey: "originalRelPath",
        id: "originalRelPath",
        header: t("admin.trashPath"),
        cell: ({ row }) => (
          <div className="flex items-center gap-2 font-medium">
            {row.original.isDir ? (
              <FolderIcon data-icon="inline-start" />
            ) : (
              <FileIcon data-icon="inline-start" />
            )}
            <span className="truncate">{row.original.originalRelPath}</span>
          </div>
        ),
      },
      {
        accessorKey: "size",
        id: "size",
        header: t("admin.trashSize"),
        cell: ({ row }) => (row.original.isDir ? "—" : formatFileSize(row.original.size)),
      },
      {
        accessorKey: "deletedByEmail",
        id: "deletedByEmail",
        header: t("admin.trashDeletedBy"),
        cell: ({ row }) => row.original.deletedByEmail || "—",
      },
      {
        accessorKey: "deletedAt",
        id: "deletedAt",
        header: t("admin.trashDeletedAt"),
        cell: ({ row }) => formatDate(row.original.deletedAt),
      },
      {
        id: "actions",
        enableHiding: false,
        header: () => <div className="text-right">{t("common.actions")}</div>,
        cell: ({ row }) => {
          const id = row.original.id;
          return (
            <div className="flex justify-end gap-2">
              <ConfirmActionButton
                label={t("admin.trashRestore")}
                title={t("admin.trashRestoreConfirmTitle")}
                description={t("admin.trashRestoreConfirmDescription", {
                  path: row.original.originalRelPath,
                })}
                confirmLabel={t("admin.trashRestore")}
                variant="outline"
                disabled={busy}
                pending={restoreOneMutation.isPending}
                onConfirm={() => {
                  restoreOneMutation.mutate(id);
                }}
              >
                <RotateCcwIcon data-icon="inline-start" />
              </ConfirmActionButton>
              <ConfirmActionButton
                label={t("admin.trashDeleteForever")}
                title={t("admin.trashDeleteConfirmTitle")}
                description={t("admin.trashDeleteConfirmDescription", {
                  path: row.original.originalRelPath,
                })}
                confirmLabel={t("admin.trashDeleteForever")}
                variant="destructive"
                disabled={busy}
                pending={deleteOneMutation.isPending}
                onConfirm={() => {
                  deleteOneMutation.mutate(id);
                }}
              />
            </div>
          );
        },
      },
    ],
    [t, busy, restoreOneMutation, deleteOneMutation],
  );

  const toggleableColumns = [
    { id: "originalRelPath", label: t("admin.trashPath") },
    { id: "size", label: t("admin.trashSize") },
    { id: "deletedByEmail", label: t("admin.trashDeletedBy") },
    { id: "deletedAt", label: t("admin.trashDeletedAt") },
  ] as const;

  if (trashQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const ids = selectedIds(rowSelection);
  const error =
    restoreOneMutation.error ??
    deleteOneMutation.error ??
    restoreManyMutation.error ??
    purgeManyMutation.error ??
    restoreAllMutation.error ??
    emptyMutation.error ??
    trashQuery.error;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t("admin.trashTitle")}</h1>
          <p className="text-muted-foreground">{t("admin.trashDescription")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" disabled={items.length === 0}>
                <Columns3Icon data-icon="inline-start" />
                {t("admin.trashColumns")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>{t("admin.trashColumns")}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              {toggleableColumns.map((column) => (
                <DropdownMenuCheckboxItem
                  key={column.id}
                  checked={columnVisibility[column.id] !== false}
                  onSelect={(event) => {
                    event.preventDefault();
                  }}
                  onCheckedChange={(checked) => {
                    setColumnVisibility((prev) => ({
                      ...prev,
                      [column.id]: !!checked,
                    }));
                  }}
                >
                  {column.label}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <ConfirmActionButton
            label={t("admin.trashRestoreSelected")}
            title={t("admin.trashRestoreManyConfirmTitle")}
            description={t("admin.trashRestoreManyConfirmDescription", {
              count: ids.length,
            })}
            confirmLabel={t("admin.trashRestoreSelected")}
            variant="outline"
            disabled={busy || ids.length === 0}
            pending={restoreManyMutation.isPending}
            onConfirm={() => {
              restoreManyMutation.mutate(ids);
            }}
          >
            {restoreManyMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          </ConfirmActionButton>
          <ConfirmActionButton
            label={t("admin.trashDeleteSelected")}
            title={t("admin.trashDeleteManyConfirmTitle")}
            description={t("admin.trashDeleteManyConfirmDescription", {
              count: ids.length,
            })}
            confirmLabel={t("admin.trashDeleteSelected")}
            variant="destructive"
            disabled={busy || ids.length === 0}
            pending={purgeManyMutation.isPending}
            onConfirm={() => {
              purgeManyMutation.mutate(ids);
            }}
          >
            {purgeManyMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          </ConfirmActionButton>
          <ConfirmActionButton
            label={t("admin.trashRestoreAll")}
            title={t("admin.trashRestoreAllConfirmTitle")}
            description={t("admin.trashRestoreAllConfirmDescription")}
            confirmLabel={t("admin.trashRestoreAll")}
            variant="outline"
            disabled={busy || items.length === 0}
            pending={restoreAllMutation.isPending}
            onConfirm={() => {
              restoreAllMutation.mutate();
            }}
          >
            {restoreAllMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          </ConfirmActionButton>
          <ConfirmActionButton
            label={t("admin.trashEmpty")}
            title={t("admin.trashEmptyConfirmTitle")}
            description={t("admin.trashEmptyConfirmDescription")}
            confirmLabel={t("admin.trashEmpty")}
            variant="destructive"
            disabled={busy || items.length === 0}
            pending={emptyMutation.isPending}
            onConfirm={() => {
              emptyMutation.mutate();
            }}
          >
            {emptyMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          </ConfirmActionButton>
        </div>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>
            {translateApiError(error, t, { fallbackKey: "common.error" })}
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
        <div className="flex flex-col gap-3">
          {ids.length > 0 ? (
            <p className="text-sm text-muted-foreground">
              {t("admin.trashSelectedCount", { count: ids.length })}
            </p>
          ) : null}
          <DataTable
            columns={columns}
            data={items}
            rowSelection={rowSelection}
            onRowSelectionChange={setRowSelection}
            columnVisibility={columnVisibility}
            onColumnVisibilityChange={setColumnVisibility}
            getRowId={(row) => row.id}
          />
        </div>
      )}
    </div>
  );
}
