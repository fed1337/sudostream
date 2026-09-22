import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useForm, useWatch } from "react-hook-form";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { MailIcon, UserPlusIcon, UsersIcon } from "lucide-react";
import { z } from "zod";

import { getApiAdminUsersOptions } from "@/client/@tanstack/react-query.gen";
import { postApiAdminInvites, postApiAdminUsers } from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/password-input";
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

function statusBadgeVariant(status?: string): "secondary" | "destructive" | "outline" {
  if (status === "active") {
    return "secondary";
  }
  if (status === "invited" || status === "unconfirmed") {
    return "outline";
  }

  return "destructive";
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

export default function AdminUsersPage() {
  const { t } = useTranslation();
  const usersQuery = useQuery({
    ...getApiAdminUsersOptions(),
  });
  const [inviteOpen, setInviteOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);

  const inviteSchema = useMemo(
    () =>
      z.object({
        email: z.email({ error: t("validation.email") }),
        role: z.string().min(1),
      }),
    [t],
  );
  type InviteValues = z.infer<typeof inviteSchema>;

  const createSchema = useMemo(
    () =>
      z.object({
        email: z.email({ error: t("validation.email") }),
        password: z.string().min(8, { error: t("validation.passwordMin") }),
        role: z.string().min(1),
      }),
    [t],
  );
  type CreateValues = z.infer<typeof createSchema>;

  const inviteForm = useForm<InviteValues>({
    resolver: zodResolver(inviteSchema),
    defaultValues: { email: "", role: "user" },
  });
  const inviteRole = useWatch({ control: inviteForm.control, name: "role" });

  const createForm = useForm<CreateValues>({
    resolver: zodResolver(createSchema),
    defaultValues: { email: "", password: "", role: "user" },
  });
  const createRole = useWatch({ control: createForm.control, name: "role" });

  const inviteMutation = useMutation({
    mutationFn: async (values: InviteValues) => {
      const response = await postApiAdminInvites({
        body: { email: values.email, role: values.role },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.createInviteFailed"));
      }
    },
    onSuccess: () => {
      inviteForm.reset({ email: "", role: "user" });
      setInviteOpen(false);
      void usersQuery.refetch();
    },
  });

  const createMutation = useMutation({
    mutationFn: async (values: CreateValues) => {
      const response = await postApiAdminUsers({
        body: {
          email: values.email,
          password: values.password,
          role: values.role,
        },
      });
      if (response.error) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.createUserFailed"));
      }
    },
    onSuccess: () => {
      createForm.reset({ email: "", password: "", role: "user" });
      setCreateOpen(false);
      void usersQuery.refetch();
    },
  });

  if (usersQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const users = usersQuery.data?.users ?? [];
  const formError = inviteMutation.error ?? createMutation.error;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t("admin.usersTitle")}</h1>
          <p className="text-muted-foreground">{t("admin.usersDescription")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            onClick={() => {
              setInviteOpen(true);
            }}
          >
            <MailIcon data-icon="inline-start" />
            {t("admin.inviteUser")}
          </Button>
          <Button
            onClick={() => {
              setCreateOpen(true);
            }}
          >
            <UserPlusIcon data-icon="inline-start" />
            {t("admin.createUser")}
          </Button>
        </div>
      </div>

      {formError instanceof Error ? (
        <Alert variant="destructive">
          <AlertTitle>{t("common.error")}</AlertTitle>
          <AlertDescription>{formError.message}</AlertDescription>
        </Alert>
      ) : null}

      {users.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <UsersIcon />
            </EmptyMedia>
            <EmptyTitle>{t("admin.usersEmptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("admin.usersEmptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.email")}</TableHead>
              <TableHead>{t("admin.role")}</TableHead>
              <TableHead>{t("admin.status")}</TableHead>
              <TableHead>{t("admin.twoFactor")}</TableHead>
              <TableHead>{t("admin.lastLogin")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((user) => (
              <TableRow key={user.id}>
                <TableCell>
                  <Link
                    to={`/admin/users/${user.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {user.email}
                  </Link>
                </TableCell>
                <TableCell className="capitalize">{user.role}</TableCell>
                <TableCell>
                  <Badge variant={statusBadgeVariant(user.status)}>
                    {statusLabel(user.status, user.enabled, t)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge variant={user.has2FA ? "success" : "secondary"}>
                    {user.has2FA ? t("admin.yes") : t("admin.no")}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(user.lastLoginAt)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog open={inviteOpen} onOpenChange={setInviteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("admin.inviteUser")}</DialogTitle>
            <DialogDescription>{t("admin.inviteUserDescription")}</DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={inviteForm.handleSubmit((values) => {
              inviteMutation.mutate(values);
            })}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="invite-email">{t("admin.email")}</FieldLabel>
                <Input id="invite-email" type="email" {...inviteForm.register("email")} />
              </Field>
              <Field>
                <FieldLabel>{t("admin.role")}</FieldLabel>
                <Select
                  value={inviteRole}
                  onValueChange={(role) => {
                    inviteForm.setValue("role", role);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">{t("admin.roleUser")}</SelectItem>
                    <SelectItem value="admin">{t("admin.roleAdmin")}</SelectItem>
                    <SelectItem value="tv">{t("admin.roleTV")}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </FieldGroup>
            <DialogFooter>
              <Button type="submit" disabled={inviteMutation.isPending}>
                {inviteMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {t("admin.sendInvite")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("admin.createUser")}</DialogTitle>
            <DialogDescription>{t("admin.createUserDescription")}</DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={createForm.handleSubmit((values) => {
              createMutation.mutate(values);
            })}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="create-email">{t("admin.email")}</FieldLabel>
                <Input id="create-email" type="email" {...createForm.register("email")} />
              </Field>
              <Field>
                <FieldLabel htmlFor="create-password">{t("auth.password")}</FieldLabel>
                <PasswordInput
                  id="create-password"
                  autoComplete="new-password"
                  {...createForm.register("password")}
                />
              </Field>
              <Field>
                <FieldLabel>{t("admin.role")}</FieldLabel>
                <Select
                  value={createRole}
                  onValueChange={(role) => {
                    createForm.setValue("role", role);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">{t("admin.roleUser")}</SelectItem>
                    <SelectItem value="admin">{t("admin.roleAdmin")}</SelectItem>
                    <SelectItem value="tv">{t("admin.roleTV")}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </FieldGroup>
            <DialogFooter>
              <Button type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {t("admin.createUser")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
