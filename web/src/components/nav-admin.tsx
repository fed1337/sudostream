import { useTranslation } from "react-i18next";
import { CalendarClockIcon, LibraryIcon, SettingsIcon, Trash2Icon, UsersIcon } from "lucide-react";

import { SidebarNavItem, type SidebarNavItemConfig } from "@/components/sidebar-nav-item";
import { SidebarGroup, SidebarGroupLabel, SidebarMenu } from "@/components/ui/sidebar";
import { useIsAdmin } from "@/hooks/use-auth";

export function NavAdmin() {
  const { t } = useTranslation();
  const isAdmin = useIsAdmin();

  if (!isAdmin) {
    return null;
  }

  const items: SidebarNavItemConfig[] = [
    {
      title: t("nav.adminLibraries"),
      url: "/admin/libraries",
      icon: LibraryIcon,
      end: false,
    },
    {
      title: t("nav.adminSchedule"),
      url: "/admin/schedule",
      icon: CalendarClockIcon,
      end: false,
    },
    {
      title: t("nav.adminUsers"),
      url: "/admin/users",
      icon: UsersIcon,
      end: false,
    },
    {
      title: t("nav.adminTrash"),
      url: "/admin/trash",
      icon: Trash2Icon,
      end: false,
    },
    {
      title: t("nav.adminSettings"),
      url: "/admin/settings",
      icon: SettingsIcon,
      end: false,
    },
  ];

  return (
    <SidebarGroup>
      <SidebarGroupLabel>{t("nav.adminMenu")}</SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => (
          <SidebarNavItem key={item.url} item={item} />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}
