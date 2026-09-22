import type { FormEvent } from "react";
import { Outlet, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { SearchIcon } from "lucide-react";

import { AppFooter } from "@/components/app-footer";
import { AppPreferences } from "@/components/app-preferences";
import { AppSidebar } from "@/components/app-sidebar";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";

export function DashboardLayout() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const raw = data.get("q");
    const query = typeof raw === "string" ? raw.trim() : "";
    if (!query) {
      return;
    }
    void navigate(`/search?q=${encodeURIComponent(query)}`);
  };

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset className="min-h-svh text-base">
        <header className="flex h-14 shrink-0 items-center gap-2 border-b md:h-16 md:gap-3">
          <div className="flex flex-1 items-center gap-2 px-4 md:gap-3 md:px-5">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 data-[orientation=vertical]" />
            <form className="hidden w-full max-w-md md:block" onSubmit={submitSearch}>
              <Field>
                <FieldLabel htmlFor="header-search" className="sr-only">
                  {t("search.title")}
                </FieldLabel>
                <ButtonGroup className="w-full">
                  <Input
                    id="header-search"
                    name="q"
                    placeholder={t("search.placeholder")}
                    autoComplete="off"
                  />
                  <Button
                    variant="outline"
                    size="icon"
                    type="submit"
                    aria-label={t("search.title")}
                  >
                    <SearchIcon />
                  </Button>
                </ButtonGroup>
              </Field>
            </form>
            <div className="ml-auto flex items-center gap-1 md:gap-2">
              <AppPreferences />
            </div>
          </div>
        </header>
        <div className="flex flex-1 flex-col gap-4 p-4 pt-2 md:gap-6 md:p-6">
          <Outlet />
        </div>
        <AppFooter />
      </SidebarInset>
    </SidebarProvider>
  );
}
