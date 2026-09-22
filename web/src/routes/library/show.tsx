import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  ChevronDownIcon,
  DownloadIcon,
  EyeIcon,
  EyeOffIcon,
  HardDriveIcon,
  HeartIcon,
  PlayIcon,
} from "lucide-react";

import {
  getApiCatalogByLibrarySlugShowsByShowKey,
  getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes,
} from "@/client/sdk.gen";
import type { SudoStreamInternalCatalogEpisode } from "@/client/types.gen";
import { patchFavoriteState } from "@/api/favorite";
import { patchWatchState } from "@/api/watch";
import { AuthenticatedImage } from "@/components/authenticated-image";
import { CatalogUpButton } from "@/components/catalog-up-button";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { authenticatedMediaUrl, playerPath } from "@/lib/media-urls";

/** Primary episode row label: "Episode N" or "Episode N: Title" (never the show name alone). */
function episodeNumberFromPath(path: string): number | undefined {
  const base = path.split("/").pop() ?? path;
  const leading = base.match(/^(\d{1,3})[._\s-]+.+\.[^.]+$/i);
  const mid = base.match(
    /^(.+?)[\s._-]+(\d{1,3})(?:\s*v\d+)?[\s._-]+([^[{].*?)(?:\s*\[[^\]]*])*(?:\s*\{[^}]*})*\.[^.]+$/i,
  );
  const end = base.match(
    /[\s._-](\d{1,3})(?:\s*v\d+)?(?:\s*(?:end|ova|ona|special))?(?:\s*\[[^\]]*])*(?:\s*\{[^}]*})*\.[^.]+$/i,
  );
  const raw = leading?.[1] ?? mid?.[2] ?? end?.[1];
  if (!raw) {
    return undefined;
  }
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value <= 0 || value > 999) {
    return undefined;
  }

  return value;
}

function episodeListLabel(
  t: TFunction,
  episode: Pick<SudoStreamInternalCatalogEpisode, "path" | "episode" | "episodeTitle" | "title">,
): string {
  const number =
    typeof episode.episode === "number" && Number.isFinite(episode.episode)
      ? episode.episode
      : episodeNumberFromPath(episode.path ?? "");
  const episodeTitle = episode.episodeTitle?.trim();
  const fallbackTitle = episode.title?.trim();
  // Prefer a real episode title; ignore when it duplicates the catalog title (often the show name).
  const titleForLabel =
    episodeTitle && (!fallbackTitle || episodeTitle.toLowerCase() !== fallbackTitle.toLowerCase())
      ? episodeTitle
      : undefined;

  if (typeof number === "number") {
    if (titleForLabel) {
      return t("catalog.episodeWithTitle", { number, title: titleForLabel });
    }

    return t("catalog.episodeNumber", { number });
  }
  if (titleForLabel) {
    return titleForLabel;
  }

  return fallbackTitle || t("catalog.episodeUnknown");
}

function EpisodeThumb({
  src,
  fallbackSrc,
  title,
}: {
  src?: string;
  fallbackSrc?: string;
  title: string;
}) {
  const sources = [src, fallbackSrc].filter(
    (value, index, list): value is string => Boolean(value) && list.indexOf(value) === index,
  );
  const [sourceIndex, setSourceIndex] = useState(0);
  const activeSrc = sources[sourceIndex];

  return (
    <div className="size-14 shrink-0 overflow-hidden rounded-md bg-muted">
      {activeSrc ? (
        <AuthenticatedImage
          key={activeSrc}
          src={activeSrc}
          alt={title}
          className="size-full object-cover"
          onError={() => setSourceIndex((index) => index + 1)}
        />
      ) : (
        <div className="flex size-full items-center justify-center text-xs text-muted-foreground">
          {title.slice(0, 1).toUpperCase()}
        </div>
      )}
    </div>
  );
}

function SeasonEpisodesList({
  slug,
  showKey,
  season,
  returnTo,
  open,
}: {
  slug: string;
  showKey: string;
  season: number;
  returnTo: string;
  open: boolean;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const episodesQuery = useQuery({
    queryKey: ["catalog-show-episodes", slug, showKey, season],
    enabled: open && Boolean(slug && showKey),
    queryFn: async () => {
      const response = await getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes({
        path: { librarySlug: slug, showKey, season },
      });
      if (response.error || !response.data) {
        throw new Error("episodes failed");
      }

      return response.data;
    },
  });

  const toggleWatched = useMutation({
    mutationFn: ({ path, watched }: { path: string; watched: boolean }) =>
      patchWatchState(path, watched),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({
        queryKey: ["catalog-show-episodes", slug, showKey, season],
      });
      void queryClient.invalidateQueries({ queryKey: ["watch", variables.path] });
    },
  });

  const toggleFavorite = useMutation({
    mutationFn: ({ path, favorited }: { path: string; favorited: boolean }) =>
      patchFavoriteState(path, favorited),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({
        queryKey: ["catalog-show-episodes", slug, showKey, season],
      });
    },
  });

  if (!open) {
    return null;
  }

  if (episodesQuery.isLoading) {
    return (
      <div className="space-y-2 p-4">
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-14 w-full" />
      </div>
    );
  }

  if (episodesQuery.isError) {
    return (
      <Alert variant="destructive" className="m-4">
        <AlertTitle>{t("catalog.episodesLoadFailed")}</AlertTitle>
        <AlertDescription>{(episodesQuery.error as Error).message}</AlertDescription>
      </Alert>
    );
  }

  const episodes = episodesQuery.data?.episodes ?? [];
  if (episodes.length === 0) {
    return <p className="px-4 py-3 text-sm text-muted-foreground">{t("catalog.episodesEmpty")}</p>;
  }

  return (
    <ul className="divide-y">
      {episodes.map((episode) => {
        const watched = episode.watched === true;
        const favorited = episode.favorited === true;
        const label = episodeListLabel(t, episode);

        return (
          <li key={episode.path} className="flex items-center gap-3 px-4 py-3">
            <EpisodeThumb
              src={episode.posterUrl}
              fallbackSrc={episode.actions?.thumbnail}
              title={label}
            />
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium">{label}</p>
            </div>
            <div className="flex shrink-0 gap-0.5">
              {episode.path ? (
                <>
                  <IconTooltip
                    label={watched ? t("browse.markUnwatched") : t("browse.markWatched")}
                  >
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-pressed={watched}
                      disabled={toggleWatched.isPending}
                      onClick={() =>
                        toggleWatched.mutate({
                          path: episode.path!,
                          watched: !watched,
                        })
                      }
                    >
                      {watched ? <EyeOffIcon /> : <EyeIcon />}
                    </Button>
                  </IconTooltip>
                  <IconTooltip label={favorited ? t("browse.unfavorite") : t("browse.favorite")}>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-pressed={favorited}
                      disabled={toggleFavorite.isPending}
                      onClick={() =>
                        toggleFavorite.mutate({
                          path: episode.path!,
                          favorited: !favorited,
                        })
                      }
                    >
                      <HeartIcon className={favorited ? "fill-current text-red-500" : undefined} />
                    </Button>
                  </IconTooltip>
                  <IconTooltip label={t("browse.play")}>
                    <Button variant="ghost" size="icon" asChild>
                      <Link
                        to={playerPath(episode.path, returnTo, {
                          name: episode.path.split("/").pop() ?? episode.title,
                        })}
                        aria-label={t("catalog.playEpisode")}
                      >
                        <PlayIcon />
                      </Link>
                    </Button>
                  </IconTooltip>
                </>
              ) : null}
              {episode.actions?.download ? (
                <IconTooltip label={t("browse.download")}>
                  <Button variant="ghost" size="icon" asChild>
                    <a
                      href={authenticatedMediaUrl(episode.actions.download)}
                      download
                      aria-label={t("catalog.downloadEpisode")}
                    >
                      <DownloadIcon />
                    </a>
                  </Button>
                </IconTooltip>
              ) : null}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function SeasonCollapsible({
  slug,
  showKey,
  season,
  episodeCount,
  returnTo,
  label,
}: {
  slug: string;
  showKey: string;
  season: number;
  episodeCount: number;
  returnTo: string;
  label: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-lg border">
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left font-medium [&[data-state=open]>svg]:rotate-180">
        <span className="flex flex-col gap-0.5 sm:flex-row sm:items-baseline sm:gap-2">
          <span>{label}</span>
          <span className="text-sm font-normal text-muted-foreground">
            {t("catalog.seasonEpisodes", { count: episodeCount })}
          </span>
        </span>
        <ChevronDownIcon className="size-4 shrink-0 transition-transform" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t">
        <SeasonEpisodesList
          slug={slug}
          showKey={showKey}
          season={season}
          returnTo={returnTo}
          open={open}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function ShowHeroPoster({ src, title }: { src?: string; title: string }) {
  const [failed, setFailed] = useState(false);
  const showImage = Boolean(src) && !failed;

  return (
    <div className="aspect-video w-40 shrink-0 overflow-hidden rounded-md bg-muted sm:w-48">
      {showImage ? (
        <AuthenticatedImage
          src={src!}
          alt={title}
          className="size-full object-cover object-center"
          onError={() => setFailed(true)}
        />
      ) : (
        <div className="flex size-full items-center justify-center text-2xl text-muted-foreground">
          {title.slice(0, 1).toUpperCase()}
        </div>
      )}
    </div>
  );
}

export default function LibraryShowPage() {
  const { t } = useTranslation();
  const { slug = "", showKey = "" } = useParams();
  const decodedKey = decodeURIComponent(showKey);
  const returnTo = `/libraries/${slug}/series/${showKey}`;

  const showQuery = useQuery({
    queryKey: ["catalog-show", slug, decodedKey],
    enabled: Boolean(slug && decodedKey),
    queryFn: async () => {
      const response = await getApiCatalogByLibrarySlugShowsByShowKey({
        path: { librarySlug: slug, showKey: decodedKey },
      });
      if (response.error || !response.data) {
        throw new Error("show catalog failed");
      }

      return response.data;
    },
  });

  if (showQuery.isLoading) {
    return <Skeleton className="h-64 w-full" />;
  }

  if (showQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("catalog.showLoadFailed")}</AlertTitle>
        <AlertDescription>{(showQuery.error as Error).message}</AlertDescription>
      </Alert>
    );
  }

  const seasons = showQuery.data?.seasons ?? [];
  const title = showQuery.data?.name || decodedKey;
  const seasonCount = showQuery.data?.seasonCount ?? seasons.length;
  const episodeCount =
    showQuery.data?.episodeCount ??
    seasons.reduce((sum, season) => sum + (season.episodeCount ?? 0), 0);
  const heroSrc = showQuery.data?.posterUrl ?? showQuery.data?.actions?.thumbnail;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <CatalogUpButton to={`/libraries/${slug}`} label={t("catalog.back")} />
      </div>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <ShowHeroPoster src={heroSrc} title={title} />
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          <p className="text-muted-foreground">
            {t("catalog.showMeta", { seasons: seasonCount, episodes: episodeCount })}
          </p>
        </div>
      </div>

      {seasons.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <HardDriveIcon />
            </EmptyMedia>
            <EmptyTitle>{t("catalog.showEmptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("catalog.showEmptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="flex flex-col gap-3">
          {seasons.map((season) => {
            const seasonNum = season.season ?? 0;

            return (
              <SeasonCollapsible
                key={seasonNum}
                slug={slug}
                showKey={decodedKey}
                season={seasonNum}
                episodeCount={season.episodeCount ?? 0}
                returnTo={returnTo}
                label={
                  seasonNum === 0
                    ? t("catalog.seasonUnknown")
                    : t("catalog.seasonLabel", { number: seasonNum })
                }
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function IconTooltip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent sideOffset={6}>{label}</TooltipContent>
    </Tooltip>
  );
}
