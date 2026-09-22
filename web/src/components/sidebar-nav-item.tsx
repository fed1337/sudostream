import type { LucideIcon } from "lucide-react";
import { NavLink, useLocation } from "react-router";

import { SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar";

export type SidebarNavItemConfig = {
  title: string;
  url: string;
  icon: LucideIcon;
  end: boolean;
};

export function SidebarNavItem({ item }: { item: SidebarNavItemConfig }) {
  const location = useLocation();
  const isActive = item.end
    ? location.pathname === item.url
    : location.pathname === item.url || location.pathname.startsWith(`${item.url}/`);

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild tooltip={item.title} isActive={isActive}>
        <NavLink to={item.url} end={item.end}>
          <item.icon />
          <span>{item.title}</span>
        </NavLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}
