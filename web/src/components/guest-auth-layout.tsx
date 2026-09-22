import type { ReactNode } from "react";

import { AppPreferences } from "@/components/app-preferences";
import { GuestShell } from "@/components/guest-shell";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

type GuestAuthLayoutProps = {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
};

export function GuestAuthLayout({ title, description, children, className }: GuestAuthLayoutProps) {
  return (
    <GuestShell>
      <div className={cn("flex w-full max-w-md flex-col gap-6 md:gap-8", className)}>
        <Card>
          <div className="flex justify-end px-4 md:px-6">
            <AppPreferences />
          </div>
          <CardHeader className="text-center">
            <CardTitle className="text-xl">{title}</CardTitle>
            {description ? <CardDescription>{description}</CardDescription> : null}
          </CardHeader>
          <CardContent>{children}</CardContent>
        </Card>
      </div>
    </GuestShell>
  );
}
