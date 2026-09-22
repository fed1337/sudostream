import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ChevronsUpDownIcon, LogOutIcon, UserIcon } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { useCurrentUser, useLogout } from "@/hooks/use-auth";

function initials(email: string | undefined): string {
  if (!email) {
    return "?";
  }

  const local = email.split("@")[0] ?? email;
  return local.slice(0, 2).toUpperCase();
}

export function NavUser() {
  const { t } = useTranslation();
  const { isMobile } = useSidebar();
  const { data } = useCurrentUser();
  const logout = useLogout();
  const email = data?.user?.email;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground">
              <Avatar className="size-8 shrink-0 rounded-lg md:size-10">
                <AvatarFallback className="rounded-lg">{initials(email)}</AvatarFallback>
              </Avatar>
              <div className="grid min-w-0 flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden md:text-base">
                <span className="truncate font-medium">{email ?? t("common.loading")}</span>
                <span className="truncate text-xs capitalize text-muted-foreground md:text-sm">
                  {data?.user?.role ?? ""}
                </span>
              </div>
              <ChevronsUpDownIcon className="ml-auto group-data-[collapsible=icon]:hidden" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-56 text-sm md:w-64 md:text-base"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm md:text-base">
                <Avatar className="size-8 rounded-lg md:size-10">
                  <AvatarFallback className="rounded-lg">{initials(email)}</AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight md:text-base">
                  <span className="truncate font-medium">{email}</span>
                  <span className="truncate text-xs capitalize text-muted-foreground md:text-sm">
                    {data?.user?.role}
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild>
                <Link to="/user">
                  <UserIcon />
                  {t("nav.user")}
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onClick={() => {
                void logout();
              }}
            >
              <LogOutIcon />
              {t("nav.logout")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
