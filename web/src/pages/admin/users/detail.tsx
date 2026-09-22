import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeftIcon, KeyRoundIcon, MailIcon, ShieldOffIcon, Trash2Icon } from "lucide-react";

import {
  getApiAdminUsersByIdSessionsOptions,
  getApiAdminUsersOptions,
} from "@/client/@tanstack/react-query.gen";
import {
  deleteApiAdminSessionsById,
  deleteApiAdminUsersById,
  patchApiAdminUsersById,
  postApiAdminUsersById2FaReset,
  postApiAdminUsersByIdResendConfirm,
  postApiAdminUsersByIdSessionsRevokeAll,
} from "@/client/sdk.gen";
import { ConfirmActionButton } from "@/components/confirm-action-button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

function statusLabel(
  status: string | undefined,
  enabled: boolean | undefined,
  t: (key: string) => string,
): string {
  if (status === "invited") {
    return t("admin.invited");
  }
  if (status === "unconfirmed") {
    return t("admin.unconfirmed");
  }
  if (status === "active" || (!status && enabled)) {
    return t("admin.active");
  }

  return t("admin.disabled");
}

export default function AdminUserDetailPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id = "" } = useParams();
  const [emailDraft, setEmailDraft] = useState("");

  const usersQuery = useQuery({
    ...getApiAdminUsersOptions(),
  });
  const sessionsQuery = useQuery({
    ...getApiAdminUsersByIdSessionsOptions({ path: { id } }),
    enabled: Boolean(id),
  });

  const user = useMemo(
    () => usersQuery.data?.users?.find((entry) => entry.id === id),
    [id, usersQuery.data?.users],
  );

  const patchMutation = useMutation({
    mutationFn: async (body: { enabled?: boolean; role?: string; email?: string }) => {
      const response = await patchApiAdminUsersById({
        path: { id },
        body,
      });

      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.updateUserFailed"));
      }

      return response.data.user;
    },
    onSuccess: () => {
      void usersQuery.refetch();
      setEmailDraft("");
    },
  });

  const resendConfirmMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAdminUsersByIdResendConfirm({
        path: { id },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.resendConfirmFailed"));
      }
    },
  });

  const reset2FAMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAdminUsersById2FaReset({
        path: { id },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.reset2FAFailed"));
      }
    },
    onSuccess: () => {
      void usersQuery.refetch();
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteApiAdminUsersById({
        path: { id },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.deleteUserFailed"));
      }
    },
    onSuccess: () => {
      void navigate("/admin/users", { replace: true });
    },
  });

  const revokeSessionMutation = useMutation({
    mutationFn: async (sessionId: string) => {
      const response = await deleteApiAdminSessionsById({
        path: { id: sessionId },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("account.revokeSessionFailed"));
      }
    },
    onSuccess: () => {
      void sessionsQuery.refetch();
    },
  });

  const revokeAllMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAdminUsersByIdSessionsRevokeAll({
        path: { id },
      });

      if (response.error) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.revokeAllSessionsFailed"),
        );
      }
    },
    onSuccess: () => {
      void sessionsQuery.refetch();
    },
  });

  if (usersQuery.isLoading) {
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

  const sessions = sessionsQuery.data?.sessions ?? [];
  const isInvited = user.status === "invited";
  const isUnconfirmed = user.status === "unconfirmed";
  const canResendConfirm = isInvited || isUnconfirmed;
  const canDelete = user.enabled === false;
  const actionError =
    patchMutation.error ??
    resendConfirmMutation.error ??
    reset2FAMutation.error ??
    deleteUserMutation.error ??
    revokeSessionMutation.error ??
    revokeAllMutation.error;

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-4">
        <Button variant="ghost" className="w-fit" asChild>
          <Link to="/admin/users">
            <ArrowLeftIcon data-icon="inline-start" />
            {t("admin.backToUsers")}
          </Link>
        </Button>
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{user.email}</h1>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="capitalize">
              {user.role}
            </Badge>
            <Badge
              variant={
                isInvited || isUnconfirmed ? "outline" : user.enabled ? "secondary" : "destructive"
              }
            >
              {statusLabel(user.status, user.enabled, t)}
            </Badge>
            <Badge variant={user.has2FA ? "success" : "secondary"}>
              {user.has2FA ? t("admin.twoFactorEnabled") : t("admin.twoFactorDisabled")}
            </Badge>
          </div>
        </div>
      </div>

      {actionError instanceof Error ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>{actionError.message}</AlertDescription>
        </Alert>
      ) : null}

      {(resendConfirmMutation.isSuccess ||
        reset2FAMutation.isSuccess ||
        revokeAllMutation.isSuccess ||
        patchMutation.isSuccess) && (
        <Alert variant="success">
          <AlertTitle>{t("admin.actionSuccess")}</AlertTitle>
        </Alert>
      )}

      <section className="flex flex-col gap-4">
        <h2 className="text-lg font-medium">{t("admin.userControls")}</h2>
        <div className="flex flex-wrap gap-2">
          <ConfirmActionButton
            label={user.enabled ? t("admin.disableUser") : t("admin.enableUser")}
            title={user.enabled ? t("admin.confirmDisableTitle") : t("admin.confirmEnableTitle")}
            description={
              user.enabled
                ? t("admin.confirmDisableDescription")
                : t("admin.confirmEnableDescription")
            }
            variant={user.enabled ? "outline" : "default"}
            pending={patchMutation.isPending}
            onConfirm={() => {
              patchMutation.mutate({ enabled: !user.enabled });
            }}
          />
          <Button variant="outline" asChild>
            <Link to={`/admin/users/${id}/libraries`}>
              <KeyRoundIcon data-icon="inline-start" />
              {t("admin.manageLibraries")}
            </Link>
          </Button>
          {canResendConfirm ? (
            <ConfirmActionButton
              label={t("admin.resendConfirm")}
              title={t("admin.confirmResendTitle")}
              description={t("admin.confirmResendDescription")}
              pending={resendConfirmMutation.isPending}
              onConfirm={() => {
                resendConfirmMutation.mutate();
              }}
            >
              {resendConfirmMutation.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <MailIcon data-icon="inline-start" />
              )}
            </ConfirmActionButton>
          ) : null}
          <ConfirmActionButton
            label={t("admin.reset2FA")}
            title={t("admin.confirmReset2FATitle")}
            description={t("admin.confirmReset2FADescription")}
            disabled={!user.has2FA}
            pending={reset2FAMutation.isPending}
            onConfirm={() => {
              reset2FAMutation.mutate();
            }}
          >
            {reset2FAMutation.isPending ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <ShieldOffIcon data-icon="inline-start" />
            )}
          </ConfirmActionButton>
          {canDelete ? (
            <ConfirmActionButton
              label={t("admin.deleteUser")}
              title={t("admin.confirmDeleteTitle")}
              description={t("admin.confirmDeleteDescription")}
              variant="destructive"
              pending={deleteUserMutation.isPending}
              onConfirm={() => {
                deleteUserMutation.mutate();
              }}
            >
              {deleteUserMutation.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Trash2Icon data-icon="inline-start" />
              )}
            </ConfirmActionButton>
          ) : null}
        </div>
      </section>

      <section className="flex flex-col gap-4">
        <h2 className="text-lg font-medium">{t("admin.role")}</h2>
        <Select
          value={user.role ?? "user"}
          disabled={patchMutation.isPending}
          onValueChange={(role) => {
            patchMutation.mutate({ role });
          }}
        >
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="user">{t("admin.roleUser")}</SelectItem>
            <SelectItem value="admin">{t("admin.roleAdmin")}</SelectItem>
            <SelectItem value="tv">{t("admin.roleTV")}</SelectItem>
          </SelectContent>
        </Select>
      </section>

      <section className="flex flex-col gap-4">
        <h2 className="text-lg font-medium">{t("admin.changeEmail")}</h2>
        <FieldGroup className="max-w-md">
          <Field>
            <FieldLabel htmlFor="admin-change-email">{t("admin.email")}</FieldLabel>
            <div className="flex gap-2">
              <Input
                id="admin-change-email"
                type="email"
                placeholder={user.email}
                value={emailDraft}
                onChange={(event) => {
                  setEmailDraft(event.target.value);
                }}
              />
              <ConfirmActionButton
                label={t("admin.saveEmail")}
                title={t("admin.confirmChangeEmailTitle")}
                description={t("admin.confirmChangeEmailDescription")}
                disabled={!emailDraft.trim() || emailDraft.trim() === user.email}
                pending={patchMutation.isPending}
                onConfirm={() => {
                  patchMutation.mutate({ email: emailDraft.trim() });
                }}
              />
            </div>
          </Field>
        </FieldGroup>
      </section>

      <section className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-4">
          <h2 className="text-lg font-medium">{t("account.sessions")}</h2>
          <ConfirmActionButton
            label={t("admin.revokeAllSessions")}
            title={t("admin.confirmRevokeAllTitle")}
            description={t("admin.confirmRevokeAllDescription")}
            disabled={sessions.length === 0}
            pending={revokeAllMutation.isPending}
            onConfirm={() => {
              revokeAllMutation.mutate();
            }}
          />
        </div>
        {sessionsQuery.isLoading ? (
          <Skeleton className="h-32 w-full rounded-xl" />
        ) : sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("account.noSessions")}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("account.sessionDevice")}</TableHead>
                <TableHead>{t("account.sessionIp")}</TableHead>
                <TableHead>{t("account.sessionCreated")}</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.id}>
                  <TableCell className="text-sm">
                    {session.userAgent ?? t("account.unknownDevice")}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{session.ip ?? "—"}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(session.createdAt)}
                  </TableCell>
                  <TableCell>
                    <ConfirmActionButton
                      label={t("account.revokeSession")}
                      title={t("admin.confirmRevokeSessionTitle")}
                      description={t("admin.confirmRevokeSessionDescription")}
                      variant="ghost"
                      size="icon"
                      pending={
                        revokeSessionMutation.isPending &&
                        revokeSessionMutation.variables === session.id
                      }
                      onConfirm={() => {
                        if (!session.id) {
                          return;
                        }
                        revokeSessionMutation.mutate(session.id);
                      }}
                    >
                      {revokeSessionMutation.isPending &&
                      revokeSessionMutation.variables === session.id ? (
                        <Spinner />
                      ) : (
                        <Trash2Icon />
                      )}
                    </ConfirmActionButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </section>
    </div>
  );
}
