import "@videojs/react/video/skin.css";

import { Container, createPlayer } from "@videojs/react";
import { I18nProvider } from "@videojs/react/i18n";
import { GoogleCast } from "@videojs/react/extensions/google-cast";
import { HlsJsVideo } from "@videojs/react/media/hlsjs-video";
import { Video, VideoSkin, videoFeatures } from "@videojs/react/video";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { SeriesPlayerChrome } from "@/components/player/series-player-chrome";
import { SkipIntroAction } from "@/components/player/skip-intro-action";
import { usePlayerControlsVisible } from "@/components/player/use-player-controls-visible";
import { PlaybackResumeLoader } from "@/components/player/playback-resume-loader";
import { PlaybackProgressSaver } from "@/components/player/playback-progress-saver";
import { createHlsAuthXhrSetup } from "@/lib/hls-auth-config";
import { localeCode } from "@/lib/i18n";
import { chaptersToWebVTT } from "@/lib/chapters-vtt";
import { withAccessToken } from "@/lib/media-urls";
import type { SeriesEpisodeOption } from "@/lib/series-nav";
import type { SkipIntroSegment } from "@/lib/skip-intro";
import { cn } from "@/lib/utils";

import "./videojs-player.css";

const { Player } = createPlayer({ features: videoFeatures });

type ChapterCue = {
  startSeconds: number;
  endSeconds: number;
  title: string;
};

type SeriesChrome = {
  mediaPath: string;
  season: number;
  episode: number;
  showName: string;
  episodes: SeriesEpisodeOption[];
  seasonsLoading?: boolean;
  onSelectEpisode: (path: string) => void;
};

type VideoJSPlayerProps = {
  src: string;
  /** Progressive file (Direct Play) vs HLS master. */
  playbackMode?: "hls" | "direct";
  resumeAtSeconds?: number;
  className?: string;
  onError?: () => void;
  onEnded?: () => void;
  onProgress?: (positionSeconds: number, durationSeconds: number) => void;
  onHlsAuthFailure?: () => void | Promise<void>;
  providerSubtitleTracks?: Array<{ lang: string; label: string; url: string }>;
  userSubtitleTrack?: { lang: string; label: string; url: string } | null;
  chapters?: ChapterCue[];
  skipIntro?: SkipIntroSegment | null;
  seriesChrome?: SeriesChrome | null;
  onUploadSubtitle?: (file: File) => Promise<void>;
  uploadBusy?: boolean;
};

/**
 * HLS or Direct Play + stock VideoSkin (including native quality on HLS).
 * Series Menus / Next / Dialog and subtitle upload sit in Container beside the skin.
 */
export function VideoJSPlayer({
  src,
  playbackMode = "hls",
  resumeAtSeconds,
  className,
  onError,
  onEnded,
  onProgress,
  onHlsAuthFailure,
  providerSubtitleTracks,
  userSubtitleTrack,
  chapters,
  skipIntro,
  seriesChrome,
  onUploadSubtitle,
  uploadBusy,
}: VideoJSPlayerProps) {
  const { i18n } = useTranslation();
  const [videoEl, setVideoEl] = useState<HTMLVideoElement | null>(null);
  const [startPosition] = useState(resumeAtSeconds);
  const isDirect = playbackMode === "direct";

  const handleError = useCallback(() => onError?.(), [onError]);

  const hlsSource = useMemo(() => {
    return {
      src,
      engine: {
        hlsJs: {
          preferManagedMediaSource: false,
          renderTextTracksNatively: true,
          ...(startPosition !== undefined && startPosition > 0 ? { startPosition } : {}),
          xhrSetup: createHlsAuthXhrSetup(onHlsAuthFailure),
        },
      },
    };
  }, [src, onHlsAuthFailure, startPosition]);

  useEffect(() => {
    if (!videoEl || !onEnded) {
      return;
    }

    videoEl.addEventListener("ended", onEnded);

    return () => {
      videoEl.removeEventListener("ended", onEnded);
    };
  }, [onEnded, videoEl]);

  useEffect(() => {
    if (!videoEl) {
      return;
    }

    const created: HTMLTrackElement[] = [];
    const attach = (track: { lang: string; label: string; url: string }, isDefault: boolean) => {
      const el = document.createElement("track");
      el.kind = "subtitles";
      el.label = track.label || track.lang;
      el.srclang = track.lang;
      el.src = withAccessToken(track.url);
      el.default = isDefault;
      videoEl.appendChild(el);
      created.push(el);
    };

    let defaultSet = false;
    if (userSubtitleTrack) {
      attach(userSubtitleTrack, true);
      defaultSet = true;
    }
    for (const track of providerSubtitleTracks ?? []) {
      attach(track, !defaultSet);
      defaultSet = true;
    }

    return () => {
      for (const el of created) {
        el.remove();
      }
    };
  }, [videoEl, providerSubtitleTracks, userSubtitleTrack]);

  useEffect(() => {
    if (!videoEl || !chapters?.length) {
      return;
    }

    const blob = new Blob([chaptersToWebVTT(chapters)], { type: "text/vtt" });
    const url = URL.createObjectURL(blob);
    const el = document.createElement("track");
    el.kind = "chapters";
    el.label = "Chapters";
    el.src = url;
    el.default = true;
    videoEl.appendChild(el);

    return () => {
      el.remove();
      URL.revokeObjectURL(url);
    };
  }, [videoEl, chapters]);

  const showStandaloneUpload = Boolean(onUploadSubtitle) && !seriesChrome;
  const mediaSrc = isDirect ? withAccessToken(src) : src;

  return (
    <div className={className ? `sudostream-video-player ${className}` : "sudostream-video-player"}>
      <Player key={`${playbackMode}:${src}`}>
        <I18nProvider locale={localeCode(i18n.resolvedLanguage ?? i18n.language)}>
          <Container className="sudostream-player-container">
            <VideoSkin className="h-full w-full">
              {isDirect ? (
                <Video ref={setVideoEl} src={mediaSrc} playsInline onError={handleError} />
              ) : (
                <HlsJsVideo ref={setVideoEl} source={hlsSource} playsInline onError={handleError} />
              )}
            </VideoSkin>
            {seriesChrome ? (
              <SeriesPlayerChrome
                mediaPath={seriesChrome.mediaPath}
                currentSeason={seriesChrome.season}
                currentEpisode={seriesChrome.episode}
                showName={seriesChrome.showName}
                episodes={seriesChrome.episodes}
                seasonsLoading={seriesChrome.seasonsLoading}
                onSelectEpisode={seriesChrome.onSelectEpisode}
                onUploadSubtitle={onUploadSubtitle}
                uploadBusy={uploadBusy}
                skipIntro={skipIntro}
              />
            ) : null}
            {skipIntro && !seriesChrome ? (
              <div className="sudostream-series-chrome__actions">
                <SkipIntroAction segment={skipIntro} />
              </div>
            ) : null}
            {showStandaloneUpload ? (
              <StandaloneSubtitleUpload
                onUploadSubtitle={onUploadSubtitle!}
                uploadBusy={uploadBusy}
              />
            ) : null}
            <GoogleCast src={withAccessToken(mediaSrc)} />
            <PlaybackResumeLoader resumeAtSeconds={resumeAtSeconds} resumeKey={src} />
            {onProgress ? <PlaybackProgressSaver video={videoEl} onProgress={onProgress} /> : null}
          </Container>
        </I18nProvider>
      </Player>
    </div>
  );
}

function StandaloneSubtitleUpload({
  onUploadSubtitle,
  uploadBusy,
}: {
  onUploadSubtitle: (file: File) => Promise<void>;
  uploadBusy?: boolean;
}) {
  const { t } = useTranslation();
  const fileRef = useRef<HTMLInputElement>(null);
  const controlsVisible = usePlayerControlsVisible();

  return (
    <div
      className={cn(
        "sudostream-series-chrome sudostream-series-chrome--upload-only",
        !controlsVisible && "sudostream-series-chrome--idle",
      )}
      aria-hidden={!controlsVisible}
    >
      <input
        ref={fileRef}
        type="file"
        accept=".vtt,.srt,text/vtt,application/x-subrip"
        className="sr-only"
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (file) {
            void onUploadSubtitle(file);
          }
        }}
      />
      <button
        type="button"
        className="sudostream-series-chrome__trigger"
        disabled={uploadBusy}
        onClick={() => fileRef.current?.click()}
      >
        {t("player.uploadSubtitle")}
      </button>
    </div>
  );
}
