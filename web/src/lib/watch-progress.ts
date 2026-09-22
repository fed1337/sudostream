export const MIN_PROGRESS_SECONDS = 10;
export const COMPLETE_RATIO = 0.9;
export const COMPLETE_REMAINING_SECONDS = 30;

export function isPlaybackComplete(position: number, duration: number): boolean {
  if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0 || position < 0) {
    return false;
  }
  if (position / duration >= COMPLETE_RATIO) {
    return true;
  }
  return duration - position <= COMPLETE_REMAINING_SECONDS;
}

export function shouldSaveProgress(position: number): boolean {
  return Number.isFinite(position) && position >= MIN_PROGRESS_SECONDS;
}

export function progressRatio(
  position?: number | null,
  duration?: number | null,
): number | undefined {
  if (
    position === undefined ||
    position === null ||
    duration === undefined ||
    duration === null ||
    duration <= 0
  ) {
    return undefined;
  }
  return Math.min(1, Math.max(0, position / duration));
}
