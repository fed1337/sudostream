import { useQuery } from "@tanstack/react-query";

import { getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes } from "@/client";
import type {
  SudoStreamInternalCatalogEpisode,
  SudoStreamInternalCatalogSeasonEpisodes,
} from "@/client/types.gen";
import type { SeriesEpisodeOption } from "@/lib/series-nav";

/** Same key as the series show-page accordion — shared React Query cache. */
export function catalogShowEpisodesQueryKey(
  librarySlug: string,
  showKey: string,
  season: number,
): readonly ["catalog-show-episodes", string, string, number] {
  return ["catalog-show-episodes", librarySlug, showKey, season];
}

/** One season page (same payload the show-page accordion stores). */
export async function fetchShowSeasonEpisodes(
  librarySlug: string,
  showKey: string,
  season: number,
): Promise<SudoStreamInternalCatalogSeasonEpisodes> {
  const response = await getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes({
    path: { librarySlug, showKey, season },
  });
  if (response.error || !response.data) {
    throw new Error("episodes failed");
  }

  return response.data;
}

export function mapCatalogEpisodesToOptions(
  episodes: SudoStreamInternalCatalogEpisode[],
  seasonFallback: number,
  episodeLabel: (n: number) => string,
): SeriesEpisodeOption[] {
  const options: SeriesEpisodeOption[] = [];
  for (const ep of episodes) {
    if (!ep.path) {
      continue;
    }
    options.push({
      path: ep.path,
      season: ep.season ?? seasonFallback,
      episode: ep.episode ?? 0,
      title: ep.episodeTitle || ep.title || episodeLabel(ep.episode ?? 0),
    });
  }

  return options;
}

/**
 * Lazy per-season episode load for the player.
 * Shares cache with show-page accordions via {@link catalogShowEpisodesQueryKey}.
 */
export function useShowSeasonEpisodeOptions(
  librarySlug: string | undefined,
  showKey: string | undefined,
  season: number | undefined,
  enabled: boolean,
  episodeLabel: (n: number) => string,
) {
  const slug = librarySlug ?? "";
  const key = showKey ?? "";
  const seasonNum = season ?? 0;

  return useQuery({
    queryKey: catalogShowEpisodesQueryKey(slug, key, seasonNum),
    enabled: enabled && Boolean(librarySlug && showKey && season !== undefined),
    queryFn: () => fetchShowSeasonEpisodes(slug, key, seasonNum),
    select: (page) => mapCatalogEpisodesToOptions(page.episodes ?? [], seasonNum, episodeLabel),
  });
}

/** Next season number after `current` in a catalog season list. */
export function nextCatalogSeason(seasons: number[], current: number): number | undefined {
  const sorted = [...new Set(seasons)].sort((a, b) => a - b);
  const index = sorted.indexOf(current);
  if (index < 0 || index >= sorted.length - 1) {
    return undefined;
  }

  return sorted[index + 1];
}
