import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowLeftIcon, DownloadIcon } from "lucide-react";

import { fetchPlayback, isHlsPlaybackReady, uploadUserSubtitle } from "@/api/playback";
import { fetchWatchState, patchWatchState } from "@/api/watch";
import {
  getApiCatalogByLibrarySlugShowsByShowKey,
  getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes,
} from "@/client";
import {
  getApiMeContinueQueryKey,
  getApiMeStatsQueryKey,
  getApiMeUnwatchedQueryKey,
  getApiMeWatchedQueryKey,
} from "@/client/@tanstack/react-query.gen";
import { AppPreferences } from "@/components/app-preferences";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { MetadataInspector } from "@/components/metadata/metadata-inspector";
import { SubtitleStyleMenu } from "@/components/player/subtitle-style-menu";
import { VideoJSPlayer } from "@/components/player/videojs-player";
import { useIsMobile } from "@/hooks/use-mobile";
import { metadataDisplayName, useMetadata } from "@/hooks/use-metadata";
import { buildDeviceProfile } from "@/lib/device-profile";
import {
  absoluteApiUrl,
  authenticatedMediaUrl,
  mediaActionsForPath,
  playerPath,
} from "@/lib/media-urls";
import type { SeriesEpisodeOption } from "@/lib/series-nav";
import { isPlaybackComplete, shouldSaveProgress } from "@/lib/watch-progress";

export default function PlayerPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const { "*": mediaPath } = useParams<{ "*": string | undefined }>();
  const returnTo = searchParams.get("return") ?? "/";
  const isMobile = useIsMobile();
  const [playerError, setPlayerError] = useState(false);
  const handlePlayerError = useCallback(() => setPlayerError(true), []);
  const markWatched = useMutation({
    mutationFn: () => patchWatchState(mediaPath!, true),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({ queryKey: ["watch", mediaPath] });
      void queryClient.invalidateQueries({ queryKey: getApiMeContinueQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeWatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeUnwatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeStatsQueryKey() });
    },
  });
  const saveProgress = useMutation({
    mutationFn: ({
      positionSeconds,
      durationSeconds,
      watched,
    }: {
      positionSeconds: number;
      durationSeconds: number;
      watched?: boolean;
    }) =>
      patchWatchState(mediaPath!, {
        positionSeconds,
        durationSeconds,
        watched,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["watch", mediaPath] });
      void queryClient.invalidateQueries({ queryKey: getApiMeContinueQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeWatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeUnwatchedQueryKey() });
    },
  });
  const handlePlaybackEnded = useCallback(() => {
    if (!mediaPath || markWatched.isPending) {
      return;
    }

    markWatched.mutate();
  }, [markWatched, mediaPath]);
  const handleProgress = useCallback(
    (positionSeconds: number, durationSeconds: number) => {
      if (!mediaPath || saveProgress.isPending) {
        return;
      }
      if (isPlaybackComplete(positionSeconds, durationSeconds)) {
        saveProgress.mutate({ positionSeconds, durationSeconds, watched: true });
        return;
      }
      if (!shouldSaveProgress(positionSeconds)) {
        return;
      }
      saveProgress.mutate({ positionSeconds, durationSeconds });
    },
    [mediaPath, saveProgress],
  );

  const metadataQuery = useMetadata(mediaPath, Boolean(mediaPath));
  const watchQuery = useQuery({
    queryKey: ["watch", mediaPath],
    queryFn: () => fetchWatchState(mediaPath!),
    enabled: Boolean(mediaPath),
  });
  const playbackNegotiatedRef = useRef(false);
  useEffect(() => {
    playbackNegotiatedRef.current = false;
  }, [mediaPath]);

  const playbackQuery = useQuery({
    queryKey: ["playback", mediaPath],
    queryFn: async () => {
      if (!playbackNegotiatedRef.current) {
        playbackNegotiatedRef.current = true;
        return fetchPlayback(mediaPath!, {
          transcode: true,
          deviceProfile: buildDeviceProfile(),
        });
      }
      return fetchPlayback(mediaPath!, { transcode: true });
    },
    enabled: Boolean(mediaPath),
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.playMethod === "directPlay") {
        return false;
      }
      const status = data?.status;

      return status === "ready" || status === "error" ? false : 1000;
    },
  });

  const playback = playbackQuery.data;
  const series = playback?.series;

  const showQuery = useQuery({
    queryKey: ["catalog-show", series?.librarySlug, series?.showKey],
    enabled: Boolean(series?.librarySlug && series?.showKey),
    queryFn: async () => {
      const response = await getApiCatalogByLibrarySlugShowsByShowKey({
        path: { librarySlug: series!.librarySlug, showKey: series!.showKey },
      });
      if (response.error || !response.data) {
        throw new Error("show catalog failed");
      }
      return response.data;
    },
  });

  const seasons = showQuery.data?.seasons ?? [];
  const episodesQuery = useQuery({
    queryKey: [
      "player-series-episodes",
      series?.librarySlug,
      series?.showKey,
      seasons.map((s) => s.season).join(","),
    ],
    enabled: Boolean(series?.librarySlug && series?.showKey && seasons.length > 0),
    queryFn: async () => {
      const pages = await Promise.all(
        seasons.map(async (season) => {
          const response = await getApiCatalogByLibrarySlugShowsByShowKeySeasonsBySeasonEpisodes({
            path: {
              librarySlug: series!.librarySlug,
              showKey: series!.showKey,
              season: season.season ?? 0,
            },
          });
          if (response.error || !response.data) {
            throw new Error("season episodes failed");
          }
          return response.data.episodes ?? [];
        }),
      );
      const options: SeriesEpisodeOption[] = [];
      for (const page of pages) {
        for (const ep of page) {
          if (!ep.path) {
            continue;
          }
          options.push({
            path: ep.path,
            season: ep.season ?? 0,
            episode: ep.episode ?? 0,
            title:
              ep.episodeTitle || ep.title || t("player.episodeValue", { number: ep.episode ?? 0 }),
          });
        }
      }
      return options;
    },
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadUserSubtitle(mediaPath!, file),
    onSuccess: () => {
      playbackNegotiatedRef.current = false;
      void queryClient.invalidateQueries({ queryKey: ["playback", mediaPath] });
    },
  });

  const selectEpisode = useCallback(
    (path: string) => {
      void navigate(playerPath(path, returnTo, { name: path.split("/").pop() }));
    },
    [navigate, returnTo],
  );

  const seriesChrome = useMemo(() => {
    if (!mediaPath || !series) {
      return null;
    }
    return {
      mediaPath,
      season: series.season,
      episode: series.episode,
      showName: series.showName,
      episodes: episodesQuery.data ?? [],
      seasonsLoading: showQuery.isLoading || episodesQuery.isLoading,
      onSelectEpisode: selectEpisode,
    };
  }, [
    episodesQuery.data,
    episodesQuery.isLoading,
    mediaPath,
    selectEpisode,
    series,
    showQuery.isLoading,
  ]);

  if (!mediaPath) {
    return (
      <div className="flex min-h-svh items-center justify-center p-4">
        <Alert variant="destructive" className="max-w-lg">
          <AlertTitle>{t("player.loadError")}</AlertTitle>
          <AlertDescription>{t("player.notFound")}</AlertDescription>
        </Alert>
      </div>
    );
  }

  const fileName = mediaPath.split("/").pop() ?? mediaPath;
  const fallbackName = searchParams.get("title") ?? fileName;
  const mimeType = searchParams.get("mime");
  const displayName = metadataDisplayName(metadataQuery.data, fallbackName);
  const media = mediaActionsForPath(mediaPath, { mimeType, name: fileName });
  const downloadUrl = authenticatedMediaUrl(media.download);

  const hlsReady = isHlsPlaybackReady(playback?.status);
  const isDirectPlay = playback?.playMethod === "directPlay" && Boolean(playback.streamUrl);
  const directStreamUrl =
    isDirectPlay && playback?.streamUrl ? absoluteApiUrl(playback.streamUrl) : null;
  const hlsMasterUrl =
    !isDirectPlay && hlsReady && media.play
      ? absoluteApiUrl(playback?.masterUrl ?? media.play)
      : null;
  const playerSrc = directStreamUrl ?? hlsMasterUrl;
  const playerReady = Boolean(playerSrc) && (isDirectPlay || hlsReady);
  const processing =
    !playerReady && playback?.status !== "error" && !playbackQuery.isError && !isDirectPlay;
  const playbackError = playback?.status === "error" || playbackQuery.isError || playerError;

  const goBack = () => {
    void navigate(returnTo);
  };

  const header = (sidebarTrigger: boolean) => (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b md:h-16 md:gap-3">
      <div className="flex min-w-0 flex-1 items-center gap-2 px-4 md:gap-3 md:px-5">
        {sidebarTrigger ? (
          <>
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-1" />
          </>
        ) : null}
        <Button variant="ghost" onClick={goBack}>
          <ArrowLeftIcon data-icon="inline-start" />
          {t("player.back")}
        </Button>
        <h1 className="min-w-0 truncate text-base font-semibold md:text-lg">
          {metadataQuery.isLoading ? <Skeleton className="h-6 w-48" /> : displayName}
        </h1>
        <div className="ml-auto flex shrink-0 items-center gap-1 md:gap-2">
          <SubtitleStyleMenu />
          <Button asChild variant="outline">
            <a href={downloadUrl} download={media.name}>
              <DownloadIcon data-icon="inline-start" />
              {t("player.download")}
            </a>
          </Button>
          <AppPreferences />
        </div>
      </div>
    </header>
  );

  const stage = (
    <div className="relative min-h-0 flex-1 overflow-hidden bg-black">
      {playerReady && playerSrc && !playerError ? (
        <VideoJSPlayer
          className="h-full w-full"
          src={playerSrc}
          playbackMode={isDirectPlay ? "direct" : "hls"}
          resumeAtSeconds={
            watchQuery.data &&
            !watchQuery.data.watched &&
            shouldSaveProgress(watchQuery.data.positionSeconds ?? 0)
              ? watchQuery.data.positionSeconds
              : undefined
          }
          providerSubtitleTracks={playback?.providerSubtitleTracks?.map((track) => ({
            ...track,
            url: absoluteApiUrl(track.url),
          }))}
          userSubtitleTrack={
            playback?.userSubtitle
              ? {
                  ...playback.userSubtitle,
                  url: absoluteApiUrl(playback.userSubtitle.url),
                }
              : null
          }
          chapters={playback?.chapters}
          seriesChrome={seriesChrome}
          onUploadSubtitle={async (file) => {
            await uploadMutation.mutateAsync(file);
          }}
          uploadBusy={uploadMutation.isPending}
          onError={handlePlayerError}
          onEnded={handlePlaybackEnded}
          onProgress={handleProgress}
        />
      ) : (
        <div className="flex h-full items-center justify-center bg-background p-4">
          {playbackError ? (
            <Alert variant="destructive" className="max-w-lg">
              <AlertTitle>{t("player.loadError")}</AlertTitle>
              <AlertDescription>{playback?.error ?? t("player.transcodeError")}</AlertDescription>
            </Alert>
          ) : null}

          {processing ? (
            <div className="flex w-full max-w-lg flex-col gap-4">
              <Skeleton className="aspect-video w-full rounded-xl" />
              <Alert>
                <AlertTitle>{t("player.preparingTitle")}</AlertTitle>
                <AlertDescription>{t("player.preparingDescription")}</AlertDescription>
              </Alert>
            </div>
          ) : null}

          {!playerReady && !processing && !playbackError ? (
            <Alert className="max-w-lg">
              <AlertTitle>{t("player.downloadOnlyTitle")}</AlertTitle>
              <AlertDescription>{t("player.downloadOnly")}</AlertDescription>
              <Button asChild className="mt-4" variant="outline">
                <a href={downloadUrl} download={media.name}>
                  <DownloadIcon data-icon="inline-start" />
                  {t("player.download")}
                </a>
              </Button>
            </Alert>
          ) : null}
        </div>
      )}
    </div>
  );

  const shell = (chrome: ReactNode) => (
    <div className="flex h-svh min-h-0 flex-col overflow-hidden">{chrome}</div>
  );

  if (isMobile) {
    return shell(
      <>
        {header(false)}
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          {stage}
          <div className="max-h-[40vh] shrink-0 overflow-y-auto border-t bg-muted/20 p-4">
            <MetadataInspector path={mediaPath} editable />
          </div>
        </div>
      </>,
    );
  }

  return (
    <SidebarProvider
      className="h-svh! min-h-0!"
      style={{ "--sidebar-width": "32rem" } as CSSProperties}
    >
      <Sidebar collapsible="offcanvas" className="border-r">
        <SidebarHeader className="h-16 shrink-0 justify-center border-b px-5 py-0">
          <h2 className="truncate text-xl font-semibold">{t("metadata.inspectorTitle")}</h2>
        </SidebarHeader>
        <SidebarContent className="min-w-0 gap-4 overflow-x-hidden p-5">
          <MetadataInspector path={mediaPath} editable variant="sidebar" />
        </SidebarContent>
      </Sidebar>

      <SidebarInset className="min-h-0 overflow-hidden">
        {header(true)}
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">{stage}</div>
      </SidebarInset>
    </SidebarProvider>
  );
}
