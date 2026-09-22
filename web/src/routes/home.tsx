import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import {
  ChartNoAxesColumnIcon,
  CirclePlayIcon,
  EyeIcon,
  EyeOffIcon,
  HeartIcon,
} from "lucide-react";

import { useQuery } from "@tanstack/react-query";

import {
  getApiMeContinueOptions,
  getApiMeFavoritesOptions,
  getApiMeStatsOptions,
  getApiMeUnwatchedOptions,
  getApiMeWatchedOptions,
} from "@/client/@tanstack/react-query.gen";
import { HomeShelfCarousel } from "@/components/home-shelf-carousel";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";

const homeShelfLimit = 10;

export default function HomePage() {
  const { t } = useTranslation();
  const continueQuery = useQuery({
    ...getApiMeContinueOptions({ query: { limit: homeShelfLimit } }),
  });
  const favoritesQuery = useQuery({
    ...getApiMeFavoritesOptions({ query: { limit: homeShelfLimit } }),
  });
  const watchedQuery = useQuery({
    ...getApiMeWatchedOptions({ query: { limit: homeShelfLimit } }),
  });
  const unwatchedQuery = useQuery({
    ...getApiMeUnwatchedOptions({ query: { limit: homeShelfLimit } }),
  });
  const statsQuery = useQuery({
    ...getApiMeStatsOptions(),
  });

  const loading =
    continueQuery.isLoading ||
    favoritesQuery.isLoading ||
    watchedQuery.isLoading ||
    unwatchedQuery.isLoading ||
    statsQuery.isLoading;

  if (loading) {
    return (
      <div className="flex flex-col gap-8">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-56" />
          <Skeleton className="h-4 w-80 max-w-full" />
        </div>
        {Array.from({ length: 4 }).map((_, index) => (
          <div key={index} className="flex flex-col gap-3">
            <Skeleton className="h-6 w-40" />
            <div className="flex gap-3 overflow-hidden">
              {Array.from({ length: 4 }).map((__, card) => (
                <Skeleton key={card} className="h-36 w-64 shrink-0 rounded-md" />
              ))}
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (
    continueQuery.isError ||
    favoritesQuery.isError ||
    watchedQuery.isError ||
    unwatchedQuery.isError ||
    statsQuery.isError
  ) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("common.error")}</AlertTitle>
        <AlertDescription>{t("common.error")}</AlertDescription>
      </Alert>
    );
  }

  const continueItems = continueQuery.data?.items ?? [];
  const favorites = favoritesQuery.data?.items ?? [];
  const watched = watchedQuery.data?.items ?? [];
  const unwatched = unwatchedQuery.data?.items ?? [];
  const libraries = statsQuery.data?.libraries ?? [];

  return (
    <div className="flex flex-col gap-8 md:gap-10">
      <div className="flex flex-col gap-1.5 md:gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">{t("home.title")}</h1>
        <p className="text-base text-muted-foreground">{t("home.description")}</p>
      </div>

      <HomeShelfCarousel
        title={t("home.continueTitle")}
        href="/continue"
        emptyTitle={t("home.continueEmptyTitle")}
        emptyDescription={t("home.continueEmptyDescription")}
        icon={<CirclePlayIcon />}
        items={continueItems}
        showProgress
      />
      <HomeShelfCarousel
        title={t("home.favoritesTitle")}
        href="/favorites"
        emptyTitle={t("home.favoritesEmptyTitle")}
        emptyDescription={t("home.favoritesEmptyDescription")}
        icon={<HeartIcon />}
        items={favorites}
        favorited
      />
      <HomeShelfCarousel
        title={t("home.watchedTitle")}
        href="/watched"
        emptyTitle={t("home.watchedEmptyTitle")}
        emptyDescription={t("home.watchedEmptyDescription")}
        icon={<EyeIcon />}
        items={watched}
        watched
      />
      <HomeShelfCarousel
        title={t("home.unwatchedTitle")}
        href="/unwatched"
        emptyTitle={t("home.unwatchedEmptyTitle")}
        emptyDescription={t("home.unwatchedEmptyDescription")}
        icon={<EyeOffIcon />}
        items={unwatched}
      />

      <section className="flex flex-col gap-3">
        <h2 className="text-xl font-semibold tracking-tight">{t("home.statsTitle")}</h2>
        {libraries.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ChartNoAxesColumnIcon />
              </EmptyMedia>
              <EmptyTitle>{t("home.statsEmptyTitle")}</EmptyTitle>
              <EmptyDescription>{t("home.statsEmptyDescription")}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {libraries.map((library) => (
              <li key={library.libraryId ?? library.slug} className="rounded-xl border p-4">
                <div className="flex flex-col gap-2">
                  <Link
                    to={`/libraries/${library.slug ?? ""}`}
                    className="font-medium hover:underline"
                  >
                    {library.name || library.slug}
                  </Link>
                  <p className="text-sm text-muted-foreground">
                    {t("home.statsProgress", {
                      watched: library.watchedCount ?? 0,
                      total: library.totalCount ?? 0,
                      percent: Math.round(library.watchedPercent ?? 0),
                    })}
                  </p>
                  <div className="h-2 overflow-hidden rounded-full bg-muted">
                    <div
                      className="h-full bg-primary"
                      style={{
                        width: `${Math.min(100, Math.max(0, library.watchedPercent ?? 0))}%`,
                      }}
                    />
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
