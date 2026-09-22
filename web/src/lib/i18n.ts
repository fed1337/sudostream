import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

import en from "@/locales/en/translation.json";
import ru from "@/locales/ru/translation.json";

export const SUPPORTED_LOCALES = [
  { code: "en" as const, flag: "🇬🇧", label: "English" },
  { code: "ru" as const, flag: "🇷🇺", label: "Русский" },
];

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]["code"];

export function localeCode(language: string | undefined): SupportedLocale {
  const base = language?.split("-")[0]?.toLowerCase();
  if (base === "ru") {
    return "ru";
  }

  return "en";
}

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      ru: { translation: ru },
    },
    supportedLngs: ["en", "ru"],
    fallbackLng: "en",
    defaultNS: "translation",
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
    },
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
