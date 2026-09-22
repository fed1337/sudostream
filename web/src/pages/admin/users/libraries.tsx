import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeftIcon } from "lucide-react";

import type {
  SudoStreamInternalAccessGrantInput,
  SudoStreamInternalAccessLibraryPermissions,
} from "@/client/types.gen";
import {
  getApiAdminUsersByIdLibrariesOptions,
  getApiAdminUsersOptions,
} from "@/client/@tanstack/react-query.gen";
import { putApiAdminUsersByIdLibraries } from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
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

type GrantInput = SudoStreamInternalAccessGrantInput;
type LibraryPermissions = SudoStreamInternalAccessLibraryPermissions;

type GrantRow = {
  libraryId: string;
  libraryName: string;
  permissions: LibraryPermissions;
};

export default function AdminUserLibrariesPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();

  const usersQuery = useQuery({
    ...getApiAdminUsersOptions(),
  });
  const grantsQuery = useQuery({
    ...getApiAdminUsersByIdLibrariesOptions({ path: { id } }),
    enabled: Boolean(id),
  });

  const user = usersQuery.data?.users?.find((entry) => entry.id === id);
  const remoteGrants = grantsQuery.data?.grants;
  const [grants, setGrants] = useState<GrantRow[]>([]);
  const [syncedGrants, setSyncedGrants] = useState(remoteGrants);

  if (remoteGrants !== syncedGrants) {
    setSyncedGrants(remoteGrants);
    setGrants(
      remoteGrants?.map((grant) => ({
        libraryId: grant.library?.id ?? "",
        libraryName: grant.library?.name ?? grant.library?.relPath ?? "",
        permissions: {
          create: grant.permissions?.create ?? false,
          read: grant.permissions?.read ?? false,
          update: grant.permissions?.update ?? false,
          delete: grant.permissions?.delete ?? false,
        },
      })) ?? [],
    );
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: GrantInput[]) => {
      const response = await putApiAdminUsersByIdLibraries({
        path: { id },
        body: { grants: payload },
      });

      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.saveGrantsFailed"));
      }

      return response.data;
    },
    onSuccess: () => {
      void grantsQuery.refetch();
    },
  });

  const togglePermission = (
    libraryId: string,
    permission: keyof LibraryPermissions,
    checked: boolean,
  ) => {
    setGrants((current) =>
      current.map((grant) =>
        grant.libraryId === libraryId
          ? {
              ...grant,
              permissions: { ...grant.permissions, [permission]: checked },
            }
          : grant,
      ),
    );
  };

  if (usersQuery.isLoading || grantsQuery.isLoading) {
    return <Skeleton className="h-64 w-full rounded-xl" />;
  }

  if (!user) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("admin.userNotFound")}</AlertTitle>
        <AlertDescription>{t("admin.userNotFoundDescription")}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <Button variant="ghost" className="w-fit" asChild>
          <Link to={`/admin/users/${id}`}>
            <ArrowLeftIcon data-icon="inline-start" />
            {t("admin.backToUser")}
          </Link>
        </Button>
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t("admin.userLibrariesTitle")}</h1>
          <p className="text-muted-foreground">
            {t("admin.userLibrariesDescription", { email: user.email })}
          </p>
        </div>
      </div>

      {saveMutation.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t("admin.saveGrantsFailed")}</AlertTitle>
          <AlertDescription>
            {saveMutation.error instanceof Error ? saveMutation.error.message : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}

      {saveMutation.isSuccess ? (
        <Alert variant="success">
          <AlertTitle>{t("admin.saveGrantsSuccess")}</AlertTitle>
        </Alert>
      ) : null}

      {grants.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("admin.noLibrariesForGrants")}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.name")}</TableHead>
              <TableHead>{t("admin.permissionRead")}</TableHead>
              <TableHead>{t("admin.permissionCreate")}</TableHead>
              <TableHead>{t("admin.permissionUpdate")}</TableHead>
              <TableHead>{t("admin.permissionDelete")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {grants.map((grant) => (
              <TableRow key={grant.libraryId}>
                <TableCell className="font-medium">{grant.libraryName}</TableCell>
                {(["read", "create", "update", "delete"] as const).map((permission) => (
                  <TableCell key={permission}>
                    <div className="flex items-center gap-2">
                      <input
                        id={`${grant.libraryId}-${permission}`}
                        type="checkbox"
                        className="size-4 rounded border"
                        checked={grant.permissions[permission] ?? false}
                        onChange={(event) => {
                          togglePermission(grant.libraryId, permission, event.target.checked);
                        }}
                      />
                      <Label htmlFor={`${grant.libraryId}-${permission}`} className="sr-only">
                        {t(
                          `admin.permission${permission.charAt(0).toUpperCase()}${permission.slice(1)}`,
                        )}
                      </Label>
                    </div>
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Button
        className="w-fit"
        disabled={saveMutation.isPending || grants.length === 0}
        onClick={() => {
          saveMutation.mutate(
            grants.map((grant) => ({
              libraryId: grant.libraryId,
              permissions: grant.permissions,
            })),
          );
        }}
      >
        {saveMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
        {saveMutation.isPending ? t("admin.saving") : t("admin.saveGrants")}
      </Button>
    </div>
  );
}
