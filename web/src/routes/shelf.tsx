import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";

import { useQuery } from "@tanstack/react-query";

import {
  getApiMeContinueOptions,
  getApiMeFavoritesOptions,
  getApiMeUnwatchedOptions,
  getApiMeWatchedOptions,
} from "@/client/@tanstack/react-query.gen";
import { BrowseLayoutToggle, type BrowseLayout } from "@/components/browse-items";
import { CatalogUpButton } from "@/components/catalog-up-button";
import type { HomeShelfItem } from "@/components/home-shelf-carousel";
import { ListPager } from "@/components/list-pager";
import { MediaPosterCard } from "@/components/media-poster-card";
import { ShelfItemList } from "@/components/shelf-item-list";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { MEDIA_PAGE_SIZE, pageToOffset, parsePageParam } from "@/lib/pagination";
import { getShelfViewMode, setShelfViewMode, type ShelfViewMode } from "@/lib/shelf-view";
import { mediaActionsForPath } from "@/lib/media-urls";
import { progressRatio } from "@/lib/watch-progress";

const shelfGridClass = "grid grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] gap-3";

type ShelfKind = "continue" | "favorites" | "watched" | "unwatched";

type ShelfPageProps = {
  kind: ShelfKind;
};

function layoutFromView(mode: ShelfViewMode): BrowseLayout {
  return mode === "list" ? "list" : "grid";
}

export function ShelfPage({ kind }: ShelfPageProps) {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parsePageParam(searchParams.get("page"));
  const [layout, setLayout] = useState<BrowseLayout>(() => layoutFromView(getShelfViewMode()));
  const offset = pageToOffset(page);
  const returnTo = `/${kind}`;

  useEffect(() => {
    setShelfViewMode(layout === "list" ? "list" : "tiles");
  }, [layout]);

  const continueQuery = useQuery({
    ...getApiMeContinueOptions({ query: { limit: MEDIA_PAGE_SIZE, offset } }),
    enabled: kind === "continue",
  });
  const favoritesQuery = useQuery({
    ...getApiMeFavoritesOptions({ query: { limit: MEDIA_PAGE_SIZE, offset } }),
    enabled: kind === "favorites",
  });
  const watchedQuery = useQuery({
    ...getApiMeWatchedOptions({ query: { limit: MEDIA_PAGE_SIZE, offset } }),
    enabled: kind === "watched",
  });
  const unwatchedQuery = useQuery({
    ...getApiMeUnwatchedOptions({ query: { limit: MEDIA_PAGE_SIZE, offset } }),
    enabled: kind === "unwatched",
  });

  const query =
    kind === "continue"
      ? continueQuery
      : kind === "favorites"
        ? favoritesQuery
        : kind === "watched"
          ? watchedQuery
          : unwatchedQuery;

  const copy = {
    continue: {
      title: t("home.continueTitle"),
      emptyTitle: t("home.continueEmptyTitle"),
      emptyDescription: t("home.continueEmptyDescription"),
    },
    favorites: {
      title: t("home.favoritesTitle"),
      emptyTitle: t("home.favoritesEmptyTitle"),
      emptyDescription: t("home.favoritesEmptyDescription"),
    },
    watched: {
      title: t("home.watchedTitle"),
      emptyTitle: t("home.watchedEmptyTitle"),
      emptyDescription: t("home.watchedEmptyDescription"),
    },
    unwatched: {
      title: t("home.unwatchedTitle"),
      emptyTitle: t("home.unwatchedEmptyTitle"),
      emptyDescription: t("home.unwatchedEmptyDescription"),
    },
  }[kind];

  const setPage = (next: number) => {
    const params = new URLSearchParams(searchParams);
    if (next <= 1) {
      params.delete("page");
    } else {
      params.set("page", String(next));
    }
    setSearchParams(params, { replace: true });
  };

  if (query.isLoading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-8 w-56" />
        <div className={shelfGridClass}>
          {Array.from({ length: 8 }).map((_, index) => (
            <Skeleton key={index} className="aspect-video rounded-md" />
          ))}
        </div>
      </div>
    );
  }

  if (query.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("common.error")}</AlertTitle>
        <AlertDescription>{t("common.error")}</AlertDescription>
      </Alert>
    );
  }

  const items = (query.data?.items ?? []) as HomeShelfItem[];
  const total = query.data?.total ?? 0;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-col gap-2">
          <CatalogUpButton to="/" label={t("catalog.back")} />
          <h1 className="text-3xl font-semibold tracking-tight">{copy.title}</h1>
        </div>
        <BrowseLayoutToggle layout={layout} onLayoutChange={setLayout} gridEnabled />
      </div>

      {items.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyTitle>{copy.emptyTitle}</EmptyTitle>
            <EmptyDescription>{copy.emptyDescription}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : layout === "list" ? (
        <ShelfItemList
          items={items}
          watched={kind === "watched"}
          favorited={kind === "favorites"}
          showProgress={kind === "continue"}
          returnTo={returnTo}
        />
      ) : (
        <div className={shelfGridClass}>
          {items.map((item) => {
            const path = item.path ?? "";
            const actions = mediaActionsForPath(path, { name: item.title });
            return (
              <MediaPosterCard
                key={path}
                title={item.title || path}
                subtitle={item.librarySlug}
                thumbnailUrl={item.posterUrl}
                fallbackThumbnailUrl={actions.thumbnail}
                mediaPath={path}
                downloadUrl={actions.download}
                watched={kind === "watched"}
                favorited={kind === "favorites"}
                progressRatio={
                  kind === "continue"
                    ? progressRatio(item.positionSeconds, item.durationSeconds)
                    : undefined
                }
                returnTo={returnTo}
              />
            );
          })}
        </div>
      )}

      <ListPager page={page} total={total} pageSize={MEDIA_PAGE_SIZE} onPageChange={setPage} />
    </div>
  );
}

export function ContinuePage() {
  return <ShelfPage kind="continue" />;
}

export function FavoritesPage() {
  return <ShelfPage kind="favorites" />;
}

export function WatchedPage() {
  return <ShelfPage kind="watched" />;
}

export function UnwatchedPage() {
  return <ShelfPage kind="unwatched" />;
}
