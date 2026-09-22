import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Trash2Icon } from "lucide-react";

import { getApiAuthSessionsOptions } from "@/client/@tanstack/react-query.gen";
import { deleteApiAuthSessionsById, postApiAuthSessionsRevokeAll } from "@/client/sdk.gen";
import { ConfirmActionButton } from "@/components/confirm-action-button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { redirectToLogin } from "@/lib/auth-session";
import { extractApiErrorMessage } from "@/lib/api-error";

function formatDate(value?: string): string {
  if (!value) {
    return "—";
  }

  return new Date(value).toLocaleString();
}

export function SessionsSection() {
  const { t } = useTranslation();
  const sessionsQuery = useQuery({
    ...getApiAuthSessionsOptions(),
  });

  const revokeMutation = useMutation({
    mutationFn: async (id: string) => {
      const response = await deleteApiAuthSessionsById({
        path: { id },
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
      const response = await postApiAuthSessionsRevokeAll();
      if (response.error) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("account.revokeAllSessionsFailed"),
        );
      }
    },
    onSuccess: () => {
      redirectToLogin();
    },
  });

  const sessions = sessionsQuery.data?.sessions ?? [];
  const sectionError = revokeMutation.error ?? revokeAllMutation.error;

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-1.5">
          <CardTitle>{t("account.sessions")}</CardTitle>
          <CardDescription>{t("account.sessionsDescription")}</CardDescription>
        </div>
        <ConfirmActionButton
          label={t("account.revokeAllSessions")}
          title={t("account.confirmRevokeAllTitle")}
          description={t("account.confirmRevokeAllDescription")}
          disabled={sessions.length === 0}
          pending={revokeAllMutation.isPending}
          onConfirm={() => {
            revokeAllMutation.mutate();
          }}
        />
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {sectionError instanceof Error ? (
          <Alert variant="destructive">
            <AlertTitle>{t("common.error")}</AlertTitle>
            <AlertDescription>{sectionError.message}</AlertDescription>
          </Alert>
        ) : null}

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
                  <TableCell>
                    <div className="flex flex-col gap-1">
                      <span className="text-sm">
                        {session.userAgent ?? t("account.unknownDevice")}
                      </span>
                      {session.current ? (
                        <Badge variant="secondary">{t("account.currentSession")}</Badge>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{session.ip ?? "—"}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(session.createdAt)}
                  </TableCell>
                  <TableCell>
                    <ConfirmActionButton
                      label={t("account.revokeSession")}
                      title={t("account.confirmRevokeSessionTitle")}
                      description={t("account.confirmRevokeSessionDescription")}
                      variant="ghost"
                      size="icon"
                      disabled={session.current}
                      pending={revokeMutation.isPending && revokeMutation.variables === session.id}
                      onConfirm={() => {
                        if (!session.id) {
                          return;
                        }
                        revokeMutation.mutate(session.id);
                      }}
                    >
                      {revokeMutation.isPending && revokeMutation.variables === session.id ? (
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
      </CardContent>
    </Card>
  );
}
