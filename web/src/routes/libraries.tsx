import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { FolderOpenIcon } from "lucide-react";

import { fetchLibraries } from "@/api/media";
import { LibraryCard } from "@/components/library-card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";

export default function LibrariesPage() {
  const { t } = useTranslation();
  const librariesQuery = useQuery({
    queryKey: ["libraries"],
    queryFn: fetchLibraries,
  });

  if (librariesQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4 md:gap-6">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-80 max-w-full" />
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 md:gap-5">
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="min-h-32 rounded-xl md:min-h-40" />
          ))}
        </div>
      </div>
    );
  }

  if (librariesQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("common.error")}</AlertTitle>
        <AlertDescription>
          {librariesQuery.error instanceof Error ? librariesQuery.error.message : t("common.error")}
        </AlertDescription>
      </Alert>
    );
  }

  const libraries = librariesQuery.data ?? [];

  return (
    <div className="flex flex-col gap-4 md:gap-6">
      <div className="flex flex-col gap-1.5 md:gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">{t("libraries.title")}</h1>
        <p className="text-base text-muted-foreground">{t("libraries.description")}</p>
      </div>

      {libraries.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderOpenIcon />
            </EmptyMedia>
            <EmptyTitle>{t("libraries.emptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("libraries.emptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 md:gap-5">
          {libraries.map((library) => (
            <LibraryCard key={library.id ?? library.slug} library={library} />
          ))}
        </div>
      )}
    </div>
  );
}
