import { useTranslation } from "react-i18next";

import { appDocsUrl, appName, appRepoUrl, appVersion } from "@/lib/app-meta";

export function AppFooter() {
  const { t } = useTranslation();

  return (
    <footer className="mt-auto border-t px-4 py-3 text-center text-sm text-muted-foreground md:px-6">
      <span>{appName}</span>
      <span aria-hidden="true"> · </span>
      <a href="/api/health" className="underline-offset-4 hover:underline">
        {t("footer.version", { version: appVersion })}
      </a>
      <span aria-hidden="true"> · </span>
      <a
        href={appDocsUrl}
        target="_blank"
        rel="noreferrer"
        className="underline-offset-4 hover:underline"
      >
        {t("footer.docs")}
      </a>
      <span aria-hidden="true"> · </span>
      <a
        href={appRepoUrl}
        target="_blank"
        rel="noreferrer"
        className="underline-offset-4 hover:underline"
      >
        {t("footer.repository")}
      </a>
    </footer>
  );
}
