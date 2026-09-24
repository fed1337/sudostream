import { Dialog, Menu, selectPlayback, selectTime, usePlayer } from "@videojs/react";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { usePlayerControlsVisible } from "@/components/player/use-player-controls-visible";
import {
  episodesForSeason,
  findNextEpisode,
  uniqueSeasons,
  type SeriesEpisodeOption,
} from "@/lib/series-nav";
import { SkipIntroAction } from "@/components/player/skip-intro-action";
import { isInSkipIntroSegment, type SkipIntroSegment } from "@/lib/skip-intro";
import { cn } from "@/lib/utils";

type SeriesPlayerChromeProps = {
  mediaPath: string;
  currentSeason: number;
  currentEpisode: number;
  showName: string;
  episodes: SeriesEpisodeOption[];
  seasonsLoading?: boolean;
  onSelectEpisode: (path: string) => void;
  onUploadSubtitle?: (file: File) => Promise<void>;
  uploadBusy?: boolean;
  skipIntro?: SkipIntroSegment | null;
};

type PlaybackEndedState = {
  ended: boolean;
};

type TimeState = {
  currentTime: number;
  duration: number;
};

/** Show Next only in the final tenth of the timeline (or when ended). */
const NEXT_EPISODE_RATIO = 0.95;

/**
 * Native Video.js Menu / Dialog chrome for series playback (Part B).
 * Lives inside `<Container>` so it participates in fullscreen and activity.
 */
export function SeriesPlayerChrome({
  mediaPath,
  currentSeason,
  currentEpisode,
  showName,
  episodes,
  seasonsLoading,
  onSelectEpisode,
  onUploadSubtitle,
  uploadBusy,
  skipIntro,
}: SeriesPlayerChromeProps): ReactNode {
  const { t } = useTranslation();
  const playback = usePlayer(selectPlayback) as PlaybackEndedState | undefined;
  const time = usePlayer(selectTime) as TimeState | undefined;
  const controlsVisible = usePlayerControlsVisible();
  const ended = playback?.ended ?? false;
  const fileRef = useRef<HTMLInputElement>(null);
  const [dismissedKey, setDismissedKey] = useState<string | null>(null);

  const seasons = useMemo(() => uniqueSeasons(episodes), [episodes]);
  const seasonEpisodes = useMemo(
    () => episodesForSeason(episodes, currentSeason),
    [episodes, currentSeason],
  );
  const nav = useMemo(() => findNextEpisode(episodes, mediaPath), [episodes, mediaPath]);

  const nearEnd = useMemo(() => {
    if (ended) {
      return true;
    }
    const duration = time?.duration ?? 0;
    const currentTime = time?.currentTime ?? 0;
    if (!Number.isFinite(duration) || duration <= 0 || !Number.isFinite(currentTime)) {
      return false;
    }
    return currentTime / duration >= NEXT_EPISODE_RATIO;
  }, [ended, time?.currentTime, time?.duration]);

  const showSkipIntro = useMemo(
    () => isInSkipIntroSegment(skipIntro ?? undefined, time?.currentTime ?? 0),
    [skipIntro, time?.currentTime],
  );
  const showNext = Boolean(nav.next && nearEnd && !showSkipIntro);
  const showActions = showSkipIntro || showNext;

  const congratsKind =
    ended && nav.seriesComplete
      ? "series"
      : ended && nav.seasonComplete && nav.next
        ? "season"
        : null;
  const endKey = `${mediaPath}:${ended ? "1" : "0"}:${congratsKind ?? "none"}`;
  const congratsOpen = congratsKind !== null && dismissedKey !== endKey;

  const seasonLabel = t("player.seasonValue", { number: currentSeason });
  const episodeLabel = t("player.episodeValue", { number: currentEpisode });

  return (
    <>
      <div className="sudostream-series-chrome" data-series-chrome="">
        <div
          className={cn(
            "sudostream-series-chrome__top",
            !controlsVisible && "sudostream-series-chrome__top--idle",
          )}
          aria-hidden={!controlsVisible}
        >
          <div className="sudostream-series-chrome__pickers">
            <Menu.Root side="bottom" align="start">
              <Menu.Trigger
                className="sudostream-series-chrome__trigger"
                aria-label={t("player.seasonMenu")}
                render={<button type="button" />}
              >
                {seasonLabel}
              </Menu.Trigger>
              <Menu.Popup className="sudostream-series-chrome__menu">
                <Menu.Content className="sudostream-series-chrome__menu-content">
                  <Menu.RadioGroup
                    className="sudostream-series-chrome__group"
                    value={String(currentSeason)}
                    onValueChange={(value) => {
                      const season = Number(value);
                      const first = episodesForSeason(episodes, season)[0];
                      if (first) {
                        onSelectEpisode(first.path);
                      }
                    }}
                  >
                    <Menu.GroupLabel className="sudostream-series-chrome__label">
                      {t("player.seasonMenu")}
                    </Menu.GroupLabel>
                    {seasons.map((season) => (
                      <Menu.RadioItem
                        key={season}
                        className="sudostream-series-chrome__item"
                        value={String(season)}
                      >
                        <Menu.ItemIndicator className="sudostream-series-chrome__indicator" />
                        {t("player.seasonValue", { number: season })}
                      </Menu.RadioItem>
                    ))}
                  </Menu.RadioGroup>
                </Menu.Content>
              </Menu.Popup>
            </Menu.Root>

            <Menu.Root side="bottom" align="start">
              <Menu.Trigger
                className="sudostream-series-chrome__trigger"
                aria-label={t("player.episodeMenu")}
                disabled={seasonsLoading || seasonEpisodes.length === 0}
                render={<button type="button" />}
              >
                {episodeLabel}
              </Menu.Trigger>
              <Menu.Popup className="sudostream-series-chrome__menu">
                <Menu.Content className="sudostream-series-chrome__menu-content">
                  <Menu.RadioGroup
                    className="sudostream-series-chrome__group"
                    value={mediaPath}
                    onValueChange={(value) => {
                      if (value && value !== mediaPath) {
                        onSelectEpisode(value);
                      }
                    }}
                  >
                    <Menu.GroupLabel className="sudostream-series-chrome__label">
                      {t("player.episodeMenu")}
                    </Menu.GroupLabel>
                    {seasonEpisodes.map((ep) => (
                      <Menu.RadioItem
                        key={ep.path}
                        className="sudostream-series-chrome__item"
                        value={ep.path}
                      >
                        <Menu.ItemIndicator className="sudostream-series-chrome__indicator" />
                        {t("player.episodeValue", { number: ep.episode })}
                      </Menu.RadioItem>
                    ))}
                  </Menu.RadioGroup>
                </Menu.Content>
              </Menu.Popup>
            </Menu.Root>
          </div>

          {onUploadSubtitle ? (
            <div className="sudostream-series-chrome__upload">
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
          ) : null}
        </div>

        {showActions ? (
          <div className="sudostream-series-chrome__actions">
            {skipIntro ? <SkipIntroAction segment={skipIntro} /> : null}
            {showNext ? (
              <button
                type="button"
                className="sudostream-series-chrome__next"
                onClick={() => onSelectEpisode(nav.next!.path)}
              >
                {t("player.nextEpisode")}
              </button>
            ) : null}
          </div>
        ) : null}
      </div>

      <Dialog.Root
        open={congratsOpen}
        onOpenChange={(open) => {
          if (!open) {
            setDismissedKey(endKey);
          }
        }}
      >
        <Dialog.Backdrop className="sudostream-congrats__backdrop" />
        <Dialog.Popup className="sudostream-congrats__dialog">
          <div className="sudostream-congrats__card">
            <Dialog.Title className="sudostream-congrats__title">
              {congratsKind === "series"
                ? t("player.seriesCompleteTitle")
                : t("player.seasonCompleteTitle")}
            </Dialog.Title>
            <Dialog.Description className="sudostream-congrats__description">
              {congratsKind === "series"
                ? t("player.seriesCompleteBody", { show: showName })
                : t("player.seasonCompleteBody", {
                    show: showName,
                    season: currentSeason,
                  })}
            </Dialog.Description>
            <div className="sudostream-congrats__actions">
              {congratsKind === "season" && nav.next ? (
                <button
                  type="button"
                  className="sudostream-congrats__primary"
                  onClick={() => {
                    setDismissedKey(endKey);
                    onSelectEpisode(nav.next!.path);
                  }}
                >
                  {t("player.continueNextSeason")}
                </button>
              ) : null}
              <Dialog.Close
                className="sudostream-congrats__close"
                render={<button type="button" />}
              >
                {t("player.closeCongrats")}
              </Dialog.Close>
            </div>
          </div>
        </Dialog.Popup>
      </Dialog.Root>
    </>
  );
}
