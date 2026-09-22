import { useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useForm, useWatch } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { MailIcon, SendIcon, Trash2Icon } from "lucide-react";
import { z } from "zod";

import { getApiAdminInvitesOptions } from "@/client/@tanstack/react-query.gen";
import {
  deleteApiAdminInvitesById,
  postApiAdminInvites,
  postApiAdminInvitesByIdResend,
} from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
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

export default function AdminInvitesPage() {
  const { t } = useTranslation();
  const invitesQuery = useQuery({
    ...getApiAdminInvitesOptions(),
  });

  const schema = useMemo(
    () =>
      z.object({
        email: z.email({ error: t("validation.email") }),
        role: z.string().min(1),
        expiresInHours: z.string().optional(),
      }),
    [t],
  );

  type FormValues = z.infer<typeof schema>;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", role: "user", expiresInHours: "" },
  });
  const inviteRole = useWatch({ control: form.control, name: "role" });

  const createMutation = useMutation({
    mutationFn: async (values: FormValues) => {
      const expiresInHours = values.expiresInHours ? Number(values.expiresInHours) : undefined;

      const response = await postApiAdminInvites({
        body: {
          email: values.email,
          role: values.role,
          expiresInHours: Number.isFinite(expiresInHours) ? expiresInHours : undefined,
        },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.createInviteFailed"));
      }
    },
    onSuccess: () => {
      form.reset({ email: "", role: "user", expiresInHours: "" });
      void invitesQuery.refetch();
    },
  });

  const resendMutation = useMutation({
    mutationFn: async (inviteId: string) => {
      const response = await postApiAdminInvitesByIdResend({
        path: { id: inviteId },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.resendInviteFailed"));
      }
    },
    onSuccess: () => {
      void invitesQuery.refetch();
    },
  });

  const revokeMutation = useMutation({
    mutationFn: async (inviteId: string) => {
      const response = await deleteApiAdminInvitesById({
        path: { id: inviteId },
      });

      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.revokeInviteFailed"));
      }
    },
    onSuccess: () => {
      void invitesQuery.refetch();
    },
  });

  if (invitesQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const invites = invitesQuery.data?.invites ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("admin.invitesTitle")}</h1>
        <p className="text-muted-foreground">{t("admin.invitesDescription")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("admin.createInvite")}</CardTitle>
          <CardDescription>{t("admin.createInviteDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={form.handleSubmit((values) => {
              createMutation.reset();
              createMutation.mutate(values);
            })}
          >
            {createMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.createInviteFailed")}</AlertTitle>
                <AlertDescription>
                  {createMutation.error instanceof Error
                    ? createMutation.error.message
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}

            {createMutation.isSuccess ? (
              <Alert variant="success">
                <AlertTitle>{t("admin.createInviteSuccess")}</AlertTitle>
              </Alert>
            ) : null}

            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="invite-email">{t("admin.email")}</FieldLabel>
                <Input id="invite-email" type="email" {...form.register("email")} />
              </Field>
              <Field>
                <FieldLabel htmlFor="invite-role">{t("admin.role")}</FieldLabel>
                <Select
                  value={inviteRole}
                  onValueChange={(value) => {
                    form.setValue("role", value);
                  }}
                >
                  <SelectTrigger id="invite-role">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">{t("admin.roleUser")}</SelectItem>
                    <SelectItem value="admin">{t("admin.roleAdmin")}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="invite-expires">{t("admin.inviteExpiresHours")}</FieldLabel>
                <Input
                  id="invite-expires"
                  type="number"
                  min={1}
                  placeholder={t("admin.inviteExpiresPlaceholder")}
                  {...form.register("expiresInHours")}
                />
              </Field>
              <Field>
                <Button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <SendIcon data-icon="inline-start" />
                  )}
                  {createMutation.isPending ? t("admin.sendingInvite") : t("admin.sendInvite")}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      {resendMutation.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t("admin.resendInviteFailed")}</AlertTitle>
          <AlertDescription>
            {resendMutation.error instanceof Error
              ? resendMutation.error.message
              : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}

      {revokeMutation.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t("admin.revokeInviteFailed")}</AlertTitle>
          <AlertDescription>
            {revokeMutation.error instanceof Error
              ? revokeMutation.error.message
              : t("common.error")}
          </AlertDescription>
        </Alert>
      ) : null}

      {invites.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <MailIcon />
            </EmptyMedia>
            <EmptyTitle>{t("admin.invitesEmptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("admin.invitesEmptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.email")}</TableHead>
              <TableHead>{t("admin.role")}</TableHead>
              <TableHead>{t("admin.status")}</TableHead>
              <TableHead>{t("admin.expiresAt")}</TableHead>
              <TableHead className="w-48" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {invites.map((invite) => (
              <TableRow key={invite.id}>
                <TableCell className="font-medium">{invite.email}</TableCell>
                <TableCell className="capitalize">{invite.role}</TableCell>
                <TableCell>
                  <Badge variant={invite.expired ? "destructive" : "secondary"}>
                    {invite.expired ? t("admin.expired") : t("admin.pending")}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(invite.expiresAt)}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      disabled={invite.expired || resendMutation.isPending}
                      onClick={() => {
                        if (!invite.id) {
                          return;
                        }
                        resendMutation.mutate(invite.id);
                      }}
                    >
                      {resendMutation.isPending && resendMutation.variables === invite.id ? (
                        <Spinner data-icon="inline-start" />
                      ) : (
                        <SendIcon data-icon="inline-start" />
                      )}
                      {t("admin.resendInvite")}
                    </Button>
                    <Button
                      variant="outline"
                      disabled={invite.expired || revokeMutation.isPending}
                      onClick={() => {
                        if (!invite.id) {
                          return;
                        }
                        revokeMutation.mutate(invite.id);
                      }}
                    >
                      {revokeMutation.isPending && revokeMutation.variables === invite.id ? (
                        <Spinner data-icon="inline-start" />
                      ) : (
                        <Trash2Icon data-icon="inline-start" />
                      )}
                      {t("admin.revokeInvite")}
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
