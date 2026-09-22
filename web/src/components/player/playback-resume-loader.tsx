import { selectPlayback, selectTime, usePlayer } from "@videojs/react";
import { useEffect, useRef } from "react";

type TimeState = {
  currentTime: number;
  duration: number;
  seek: (time: number) => Promise<number>;
};

type PlaybackState = {
  paused: boolean;
  play: () => Promise<void>;
};

type PlaybackResumeLoaderProps = {
  resumeAtSeconds?: number;
  autoPlay?: boolean;
  resumeKey: string;
};

/**
 * Seeks to the saved resume position once HLS reports a duration. Renditions share one absolute
 * timeline, so this runs on load only — quality and audio switches keep the current position.
 */
export function PlaybackResumeLoader({
  resumeAtSeconds,
  autoPlay = false,
  resumeKey,
}: PlaybackResumeLoaderProps): null {
  const time = usePlayer(selectTime) as TimeState | undefined;
  const playback = usePlayer(selectPlayback) as PlaybackState | undefined;
  const appliedRef = useRef<number | undefined>(undefined);
  const timeRef = useRef(time);
  const playbackRef = useRef(playback);
  const duration = time?.duration ?? 0;

  useEffect(() => {
    timeRef.current = time;
    playbackRef.current = playback;
  }, [time, playback]);

  useEffect(() => {
    appliedRef.current = undefined;
  }, [resumeKey]);

  useEffect(() => {
    const timeState = timeRef.current;
    const playbackState = playbackRef.current;

    if (resumeAtSeconds === undefined || resumeAtSeconds < 0 || !timeState || !playbackState) {
      return;
    }

    if (duration <= 0) {
      return;
    }

    if (appliedRef.current === resumeAtSeconds) {
      return;
    }

    let cancelled = false;
    let attempts = 0;

    const finish = () => {
      if (!cancelled) {
        appliedRef.current = resumeAtSeconds;
      }
    };

    const applyResume = async () => {
      const activeTime = timeRef.current;
      const activePlayback = playbackRef.current;
      if (cancelled || !activeTime || !activePlayback) {
        return;
      }

      attempts += 1;
      if (Math.abs(activeTime.currentTime - resumeAtSeconds) > 0.5) {
        await activeTime.seek(resumeAtSeconds);
      }

      if (cancelled) {
        return;
      }

      if (Math.abs(activeTime.currentTime - resumeAtSeconds) <= 0.5) {
        if (autoPlay && activePlayback.paused) {
          try {
            await activePlayback.play();
          } catch {
            // Browser may block autoplay without a fresh gesture.
          }
        }

        finish();

        return;
      }

      if (attempts < 8) {
        window.setTimeout(() => {
          void applyResume();
        }, 250);
      }
    };

    void applyResume();

    return () => {
      cancelled = true;
    };
  }, [autoPlay, duration, resumeAtSeconds, resumeKey]);

  return null;
}
