import { Link, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { SearchIcon } from "lucide-react";

import { useQuery } from "@tanstack/react-query";

import { getApiSearchOptions } from "@/client/@tanstack/react-query.gen";
import { ListPager } from "@/components/list-pager";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { playerPath } from "@/lib/media-urls";
import { MEDIA_PAGE_SIZE, pageToOffset, parsePageParam } from "@/lib/pagination";

function resultHref(item: {
  path?: string;
  librarySlug?: string;
  libraryType?: string;
  showKey?: string;
}): string {
  const path = item.path ?? "";
  if (item.libraryType === "series" && item.librarySlug && item.showKey) {
    return `/libraries/${item.librarySlug}/series/${item.showKey}`;
  }
  if (item.libraryType === "film" && path) {
    return playerPath(path, "/search");
  }
  if (item.librarySlug && path) {
    return `/libraries/${item.librarySlug}/browse/${path}`;
  }
  return path ? playerPath(path, "/search") : "/libraries";
}

export default function SearchPage() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get("q") ?? "").trim();
  const page = parsePageParam(searchParams.get("page"));

  const searchQuery = useQuery({
    ...getApiSearchOptions({
      query: { q: query, limit: MEDIA_PAGE_SIZE, offset: pageToOffset(page) },
    }),
    enabled: Boolean(query),
  });

  const setPage = (next: number) => {
    const params = new URLSearchParams(searchParams);
    if (next <= 1) {
      params.delete("page");
    } else {
      params.set("page", String(next));
    }
    setSearchParams(params, { replace: true });
  };

  if (!query) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <SearchIcon />
          </EmptyMedia>
          <EmptyTitle>{t("search.title")}</EmptyTitle>
          <EmptyDescription>{t("search.emptyQuery")}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  if (searchQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48 w-full rounded-xl" />
      </div>
    );
  }

  if (searchQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("search.failed")}</AlertTitle>
        <AlertDescription>{t("common.error")}</AlertDescription>
      </Alert>
    );
  }

  const items = searchQuery.data?.items ?? [];
  const total = searchQuery.data?.total ?? items.length;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("search.title")}</h1>
        <p className="text-muted-foreground">{t("search.resultsFor", { query, count: total })}</p>
      </div>
      {items.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <SearchIcon />
            </EmptyMedia>
            <EmptyTitle>{t("search.noResults")}</EmptyTitle>
            <EmptyDescription>{t("search.noResultsDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {items.map((item) => (
              <li key={`${item.librarySlug}-${item.path}`}>
                <Link
                  to={resultHref(item)}
                  className="flex flex-col rounded-lg border px-4 py-3 hover:bg-muted/50"
                >
                  <span className="font-medium">{item.title ?? item.path}</span>
                  <span className="text-sm text-muted-foreground">
                    {item.librarySlug}
                    {item.path ? ` · ${item.path}` : ""}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
          <ListPager page={page} total={total} pageSize={MEDIA_PAGE_SIZE} onPageChange={setPage} />
        </>
      )}
    </div>
  );
}
