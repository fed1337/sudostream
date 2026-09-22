import { useTranslation } from "react-i18next";
import { CheckIcon, LanguagesIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SUPPORTED_LOCALES, localeCode, type SupportedLocale } from "@/lib/i18n";

export function LocaleSwitcher() {
  const { i18n, t } = useTranslation();
  const activeCode = localeCode(i18n.resolvedLanguage ?? i18n.language);
  const active =
    SUPPORTED_LOCALES.find((locale) => locale.code === activeCode) ?? SUPPORTED_LOCALES[0];

  const selectLocale = (code: SupportedLocale) => {
    void i18n.changeLanguage(code);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="default"
          className="gap-2 px-2"
          aria-label={t("locale.choose")}
        >
          <span className="text-base leading-none" aria-hidden>
            {active.flag}
          </span>
          <span className="hidden md:inline">{active.label}</span>
          <LanguagesIcon className="md:hidden" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-40">
        {SUPPORTED_LOCALES.map((locale) => (
          <DropdownMenuItem key={locale.code} onClick={() => selectLocale(locale.code)}>
            <span className="text-base leading-none" aria-hidden>
              {locale.flag}
            </span>
            <span className="flex-1">{locale.label}</span>
            {activeCode === locale.code ? <CheckIcon className="size-4 opacity-70" /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
