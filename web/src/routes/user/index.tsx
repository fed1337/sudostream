import { useTranslation } from "react-i18next";

import { ChangeEmailForm } from "@/components/account/change-email-form";
import { PlaybackPreferencesSection } from "@/components/account/playback-preferences-section";
import { ChangePasswordForm } from "@/components/account/change-password-form";
import { SessionsSection } from "@/components/account/sessions-section";
import { TwoFactorSection } from "@/components/account/two-factor-section";
import { useCurrentUser } from "@/hooks/use-auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export default function UserIndexPage() {
  const { t } = useTranslation();
  const { data, isLoading } = useCurrentUser();

  if (isLoading) {
    return <Skeleton className="h-40 w-full max-w-lg rounded-xl" />;
  }

  const user = data?.user;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("account.title")}</h1>
        <p className="text-muted-foreground">
          {t("auth.signedInAs", { email: user?.email ?? "" })}
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("account.profile")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">{t("account.email")}</span>
            <span>{user?.email}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">{t("account.role")}</span>
            <span className="capitalize">{user?.role}</span>
          </div>
        </CardContent>
      </Card>

      <PlaybackPreferencesSection />

      <div className="grid gap-6 md:grid-cols-2 md:items-stretch">
        <ChangePasswordForm />
        <ChangeEmailForm />
      </div>

      <TwoFactorSection />

      <SessionsSection />
    </div>
  );
}
