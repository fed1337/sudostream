import { useEffect, useRef } from "react";

import { isPlaybackComplete, shouldSaveProgress } from "@/lib/watch-progress";

const progressSaveIntervalMs = 10_000;

type PlaybackProgressSaverProps = {
  video: HTMLVideoElement | null;
  onProgress: (positionSeconds: number, durationSeconds: number) => void;
};

export function PlaybackProgressSaver({ video, onProgress }: PlaybackProgressSaverProps): null {
  const onProgressRef = useRef(onProgress);
  const lastSavedRef = useRef(0);

  useEffect(() => {
    onProgressRef.current = onProgress;
  }, [onProgress]);

  useEffect(() => {
    if (!video) {
      return;
    }

    const save = (force: boolean) => {
      const position = video.currentTime;
      const duration = video.duration;
      if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) {
        return;
      }
      if (!force && !isPlaybackComplete(position, duration) && !shouldSaveProgress(position)) {
        return;
      }
      const now = Date.now();
      if (!force && now - lastSavedRef.current < progressSaveIntervalMs) {
        return;
      }
      lastSavedRef.current = now;
      onProgressRef.current(position, duration);
    };

    const onTimeUpdate = () => {
      save(false);
    };
    const onPauseOrSeek = () => {
      save(true);
    };
    const onHide = () => {
      save(true);
    };

    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("pause", onPauseOrSeek);
    video.addEventListener("seeked", onPauseOrSeek);
    window.addEventListener("pagehide", onHide);

    return () => {
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("pause", onPauseOrSeek);
      video.removeEventListener("seeked", onPauseOrSeek);
      window.removeEventListener("pagehide", onHide);
    };
  }, [video]);

  return null;
}
