export type SeriesEpisodeOption = {
  path: string;
  season: number;
  episode: number;
  title: string;
};

export type SeriesNavResult = {
  next: SeriesEpisodeOption | null;
  seasonComplete: boolean;
  seriesComplete: boolean;
};

/** Normalize media paths for equality (route params vs catalog JSON). */
export function normalizeMediaPath(path: string): string {
  const trimmed = path.replace(/^\/+/, "").replaceAll("\\", "/");
  try {
    return decodeURIComponent(trimmed);
  } catch {
    return trimmed;
  }
}

/** Flat list of episodes sorted by season then episode number. */
export function sortEpisodes(episodes: SeriesEpisodeOption[]): SeriesEpisodeOption[] {
  return [...episodes].sort((a, b) => {
    if (a.season !== b.season) {
      return a.season - b.season;
    }
    return a.episode - b.episode;
  });
}

export function findNextEpisode(
  episodes: SeriesEpisodeOption[],
  currentPath: string,
): SeriesNavResult {
  const sorted = sortEpisodes(episodes);
  const needle = normalizeMediaPath(currentPath);
  const index = sorted.findIndex((ep) => normalizeMediaPath(ep.path) === needle);
  if (index < 0) {
    return { next: null, seasonComplete: false, seriesComplete: false };
  }

  const current = sorted[index]!;
  const next = sorted[index + 1] ?? null;
  const seasonComplete = !next || next.season !== current.season;
  const seriesComplete = next === null;

  return { next, seasonComplete, seriesComplete };
}

export function episodesForSeason(
  episodes: SeriesEpisodeOption[],
  season: number,
): SeriesEpisodeOption[] {
  return sortEpisodes(episodes.filter((ep) => ep.season === season));
}

export function uniqueSeasons(episodes: SeriesEpisodeOption[]): number[] {
  return [...new Set(episodes.map((ep) => ep.season))].sort((a, b) => a - b);
}

/** Prefer catalog season headers; fall back to seasons inferred from loaded episodes. */
export function seasonNumbersForMenu(
  catalogSeasons: number[] | undefined,
  episodes: SeriesEpisodeOption[],
): number[] {
  if (catalogSeasons && catalogSeasons.length > 0) {
    return [...new Set(catalogSeasons)].sort((a, b) => a - b);
  }

  return uniqueSeasons(episodes);
}
