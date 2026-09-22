import { useTranslation } from "react-i18next";
import { HomeIcon, LibraryIcon } from "lucide-react";

import { SidebarNavItem, type SidebarNavItemConfig } from "@/components/sidebar-nav-item";
import { SidebarGroup, SidebarGroupLabel, SidebarMenu } from "@/components/ui/sidebar";

export function NavMain() {
  const { t } = useTranslation();

  const items: SidebarNavItemConfig[] = [
    { title: t("nav.home"), url: "/", icon: HomeIcon, end: true },
    { title: t("nav.libraries"), url: "/libraries", icon: LibraryIcon, end: false },
  ];

  return (
    <SidebarGroup>
      <SidebarGroupLabel>{t("nav.mainMenu")}</SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => (
          <SidebarNavItem key={item.url} item={item} />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}
