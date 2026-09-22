import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { HardDriveIcon } from "lucide-react";

import { getApiCatalogByLibrarySlug } from "@/client/sdk.gen";
import { fetchLibraries } from "@/api/media";
import { BrowseLayoutToggle, type BrowseLayout } from "@/components/browse-items";
import { CatalogUpButton } from "@/components/catalog-up-button";
import { ListPager } from "@/components/list-pager";
import { MediaPosterCard } from "@/components/media-poster-card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { getLibraryViewMode, setLibraryViewMode } from "@/lib/library-view";
import { MEDIA_PAGE_SIZE, pageToOffset, parsePageParam } from "@/lib/pagination";

/** ~6 cols on wide desktops; app breakpoints are collapsed so avoid sm/md/lg col counts. */
const catalogGridClass = "grid grid-cols-[repeat(auto-fill,minmax(20rem,1fr))] gap-3";

export default function LibraryCatalogPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { slug = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parsePageParam(searchParams.get("page"));
  const [layout, setLayout] = useState<BrowseLayout>(() =>
    getLibraryViewMode(slug) === "files" ? "list" : "grid",
  );
  const [prevSlug, setPrevSlug] = useState(slug);

  if (slug !== prevSlug) {
    setPrevSlug(slug);
    setLayout(getLibraryViewMode(slug) === "files" ? "list" : "grid");
  }

  useEffect(() => {
    if (layout === "list" && slug) {
      void navigate(`/libraries/${slug}/browse`, { replace: true });
    }
  }, [layout, slug, navigate]);

  const librariesQuery = useQuery({
    queryKey: ["libraries"],
    queryFn: fetchLibraries,
  });

  const library = librariesQuery.data?.find((item) => item.slug === slug || item.relPath === slug);

  const catalogQuery = useQuery({
    queryKey: ["catalog", slug, page],
    enabled: Boolean(slug) && (library?.type === "film" || library?.type === "series"),
    queryFn: async () => {
      const response = await getApiCatalogByLibrarySlug({
        path: { librarySlug: slug },
        query: { limit: MEDIA_PAGE_SIZE, offset: pageToOffset(page) },
      });
      if (response.error || !response.data) {
        throw new Error("catalog failed");
      }

      return response.data;
    },
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

  if (librariesQuery.isLoading || catalogQuery.isLoading) {
    return (
      <div className={catalogGridClass}>
        {Array.from({ length: MEDIA_PAGE_SIZE }).map((_, index) => (
          <Skeleton key={index} className="aspect-video w-full rounded-md" />
        ))}
      </div>
    );
  }

  if (!library || (library.type !== "film" && library.type !== "series")) {
    return (
      <Alert>
        <AlertTitle>{t("catalog.unsupportedTitle")}</AlertTitle>
        <AlertDescription>
          <Link to={slug ? `/libraries/${slug}/browse` : "/"} className="underline">
            {t("catalog.openFiles")}
          </Link>
        </AlertDescription>
      </Alert>
    );
  }

  if (catalogQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("catalog.loadFailed")}</AlertTitle>
        <AlertDescription>{(catalogQuery.error as Error).message}</AlertDescription>
      </Alert>
    );
  }

  const movies = catalogQuery.data?.movies ?? [];
  const shows = catalogQuery.data?.shows ?? [];
  const total = catalogQuery.data?.total ?? 0;
  const empty = total === 0;
  const returnTo = `/libraries/${slug}`;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-col gap-1">
          <CatalogUpButton to="/" label={t("catalog.back")} />
          <h1 className="text-2xl font-semibold tracking-tight">{library.name}</h1>
          <p className="text-muted-foreground">{t("catalog.subtitle")}</p>
        </div>
        <BrowseLayoutToggle
          layout="grid"
          gridEnabled
          onLayoutChange={(next) => {
            if (next === "list") {
              setLibraryViewMode(slug, "files");
              setLayout("list");
            }
          }}
        />
      </div>

      {empty ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <HardDriveIcon />
            </EmptyMedia>
            <EmptyTitle>{t("catalog.emptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("catalog.emptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : library.type === "film" ? (
        <>
          <div className={catalogGridClass}>
            {movies.map((movie) => (
              <MediaPosterCard
                key={movie.path}
                title={movie.title || movie.path || ""}
                subtitle={movie.year ? String(movie.year) : undefined}
                thumbnailUrl={movie.posterUrl}
                fallbackThumbnailUrl={movie.actions?.thumbnail}
                mediaPath={movie.path}
                downloadUrl={movie.actions?.download}
                watched={movie.watched === true}
                favorited={movie.favorited === true}
                returnTo={returnTo}
              />
            ))}
          </div>
          <ListPager page={page} total={total} pageSize={MEDIA_PAGE_SIZE} onPageChange={setPage} />
        </>
      ) : (
        <>
          <div className={catalogGridClass}>
            {shows.map((show) => (
              <MediaPosterCard
                key={show.showKey}
                title={show.name || show.showKey || ""}
                subtitle={t("catalog.showMeta", {
                  seasons: show.seasonCount ?? 0,
                  episodes: show.episodeCount ?? 0,
                })}
                thumbnailUrl={show.posterUrl}
                fallbackThumbnailUrl={show.actions?.thumbnail}
                href={`/libraries/${slug}/series/${encodeURIComponent(show.showKey || "")}`}
              />
            ))}
          </div>
          <ListPager page={page} total={total} pageSize={MEDIA_PAGE_SIZE} onPageChange={setPage} />
        </>
      )}
    </div>
  );
}
