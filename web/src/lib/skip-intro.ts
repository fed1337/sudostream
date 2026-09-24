/** Grace after segment start before showing Skip intro (matches E-23 spec). */
export const SKIP_INTRO_SHOW_GRACE_SEC = 1.5;

/** Small pad after seek target for keyframe landing. */
export const SKIP_INTRO_SEEK_PAD_SEC = 0.25;

export type SkipIntroSegment = {
  startMs: number;
  endMs: number;
};

export function isInSkipIntroSegment(
  segment: SkipIntroSegment | undefined,
  currentTimeSec: number,
): boolean {
  if (!segment || !Number.isFinite(currentTimeSec)) {
    return false;
  }
  const startSec = segment.startMs / 1000 + SKIP_INTRO_SHOW_GRACE_SEC;
  const endSec = segment.endMs / 1000;
  if (!Number.isFinite(startSec) || !Number.isFinite(endSec) || endSec <= startSec) {
    return false;
  }
  return currentTimeSec >= startSec && currentTimeSec < endSec;
}

export function skipIntroSeekSeconds(segment: SkipIntroSegment): number {
  return segment.endMs / 1000 + SKIP_INTRO_SEEK_PAD_SEC;
}
