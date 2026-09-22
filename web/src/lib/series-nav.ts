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
  const index = sorted.findIndex((ep) => ep.path === currentPath);
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
