import type { ReactNode } from "react";

import { AppFooter } from "@/components/app-footer";
import { cn } from "@/lib/utils";

type GuestShellProps = {
  children: ReactNode;
  className?: string;
};

export function GuestShell({ children, className }: GuestShellProps) {
  return (
    <div className={cn("flex min-h-svh flex-col bg-muted", className)}>
      <div className="flex flex-1 flex-col items-center justify-center gap-6 p-6 md:gap-8 md:p-12">
        {children}
      </div>
      <AppFooter />
    </div>
  );
}
