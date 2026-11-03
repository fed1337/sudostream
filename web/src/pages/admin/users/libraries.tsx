import { useEffect, useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Controller, useFieldArray, useForm } from "react-hook-form";
import { Link, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeftIcon } from "lucide-react";
import { z } from "zod";

import type { SudoStreamInternalAccessGrantInput } from "@/client/types.gen";
import {
  getApiAdminUsersByIdLibrariesOptions,
  getApiAdminUsersOptions,
} from "@/client/@tanstack/react-query.gen";
import { putApiAdminUsersByIdLibraries } from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { extractApiErrorMessage, translateApiError } from "@/lib/api-error";

type GrantInput = SudoStreamInternalAccessGrantInput;

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

  const schema = useMemo(
    () =>
      z.object({
        grants: z.array(
          z.object({
            libraryId: z.string(),
            libraryName: z.string(),
            permissions: z.object({
              create: z.boolean(),
              read: z.boolean(),
              update: z.boolean(),
              delete: z.boolean(),
            }),
          }),
        ),
      }),
    [],
  );
  type Values = z.infer<typeof schema>;

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { grants: [] },
  });
  const { fields } = useFieldArray({ control: form.control, name: "grants" });

  useEffect(() => {
    const remote = grantsQuery.data?.grants;
    if (!remote) {
      return;
    }
    form.reset({
      grants:
        remote.map((grant) => ({
          libraryId: grant.library?.id ?? "",
          libraryName: grant.library?.name ?? grant.library?.relPath ?? "",
          permissions: {
            create: grant.permissions?.create ?? false,
            read: grant.permissions?.read ?? false,
            update: grant.permissions?.update ?? false,
            delete: grant.permissions?.delete ?? false,
          },
        })) ?? [],
    });
  }, [grantsQuery.data, form]);

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
            {translateApiError(saveMutation.error, t, { fallbackKey: "admin.saveGrantsFailed" })}
          </AlertDescription>
        </Alert>
      ) : null}

      {saveMutation.isSuccess ? (
        <Alert variant="success">
          <AlertTitle>{t("admin.saveGrantsSuccess")}</AlertTitle>
        </Alert>
      ) : null}

      <form
        className="flex flex-col gap-4"
        onSubmit={form.handleSubmit((values) => {
          saveMutation.mutate(
            values.grants.map((grant) => ({
              libraryId: grant.libraryId,
              permissions: grant.permissions,
            })),
          );
        })}
      >
        {fields.length === 0 ? (
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
              {fields.map((grant, index) => (
                <TableRow key={grant.id}>
                  <TableCell className="font-medium">{grant.libraryName}</TableCell>
                  {(["read", "create", "update", "delete"] as const).map((permission) => (
                    <TableCell key={permission}>
                      <div className="flex items-center gap-2">
                        <Controller
                          control={form.control}
                          name={`grants.${index}.permissions.${permission}`}
                          render={({ field }) => (
                            <Checkbox
                              id={`${grant.libraryId}-${permission}`}
                              checked={field.value}
                              onCheckedChange={(checked) => {
                                field.onChange(checked === true);
                              }}
                            />
                          )}
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
          type="submit"
          className="w-fit"
          disabled={saveMutation.isPending || fields.length === 0}
        >
          {saveMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
          {saveMutation.isPending ? t("admin.saving") : t("admin.saveGrants")}
        </Button>
      </form>
    </div>
  );
}
