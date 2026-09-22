const STORAGE_KEY = "sudostream.subtitleStyle";
const STYLE_ELEMENT_ID = "sudostream-subtitle-cue-style";

export const SUBTITLE_SIZE_MIN = 8;
export const SUBTITLE_SIZE_MAX = 72;
export const SUBTITLE_SIZE_STEP = 4;
export const SUBTITLE_SIZE_DEFAULT = 16;

export const SUBTITLE_COLORS = [
  { id: "white", labelKey: "player.subtitleColorWhite", value: "#ffffff" },
  { id: "yellow", labelKey: "player.subtitleColorYellow", value: "#ffe082" },
  { id: "cyan", labelKey: "player.subtitleColorCyan", value: "#80deea" },
] as const;

export const SUBTITLE_OUTLINES = [
  { id: "on", labelKey: "player.subtitleOutlineOn" },
  { id: "off", labelKey: "player.subtitleOutlineOff" },
] as const;

export const SUBTITLE_BACKGROUNDS = [
  { id: "off", labelKey: "player.subtitleBackgroundOff", value: "transparent" },
  { id: "dim", labelKey: "player.subtitleBackgroundDim", value: "rgba(0, 0, 0, 0.5)" },
  { id: "solid", labelKey: "player.subtitleBackgroundSolid", value: "rgba(0, 0, 0, 1)" },
] as const;

export type SubtitleStyle = {
  /** Font size in px (8–72, step 4). */
  sizePx: number;
  color: (typeof SUBTITLE_COLORS)[number]["id"];
  outline: (typeof SUBTITLE_OUTLINES)[number]["id"];
  background: (typeof SUBTITLE_BACKGROUNDS)[number]["id"];
};

export const DEFAULT_SUBTITLE_STYLE: SubtitleStyle = {
  sizePx: SUBTITLE_SIZE_DEFAULT,
  color: "white",
  outline: "on",
  background: "off",
};

/** Hard multi-offset shadow — soft blur shadows are nearly invisible on native cues. */
const OUTLINE_SHADOW =
  "-1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000, " +
  "0 -1px 0 #000, 0 1px 0 #000, -1px 0 0 #000, 1px 0 0 #000";

function pick<T extends { id: string }>(options: readonly T[], id: string, fallback: T): T {
  return options.find((option) => option.id === id) ?? fallback;
}

export function clampSubtitleSizePx(value: number): number {
  if (!Number.isFinite(value)) {
    return SUBTITLE_SIZE_DEFAULT;
  }

  const stepped =
    Math.round((value - SUBTITLE_SIZE_MIN) / SUBTITLE_SIZE_STEP) * SUBTITLE_SIZE_STEP +
    SUBTITLE_SIZE_MIN;

  return Math.min(SUBTITLE_SIZE_MAX, Math.max(SUBTITLE_SIZE_MIN, stepped));
}

function migrateLegacySize(parsed: { sizePx?: unknown; size?: unknown }): number {
  if (typeof parsed.sizePx === "number") {
    return clampSubtitleSizePx(parsed.sizePx);
  }

  switch (parsed.size) {
    case "sm":
      return 12;
    case "lg":
      return 20;
    case "md":
      return SUBTITLE_SIZE_DEFAULT;
    default:
      return SUBTITLE_SIZE_DEFAULT;
  }
}

export function readSubtitleStyle(): SubtitleStyle {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return DEFAULT_SUBTITLE_STYLE;
    }

    const parsed = JSON.parse(raw) as Partial<SubtitleStyle> & {
      size?: string;
      shadow?: string;
    };
    // Migrate the old soft-shadow toggle from the first styling pass.
    const outline =
      parsed.outline ??
      (parsed.shadow === "none" ? "off" : parsed.shadow === "outline" ? "on" : undefined);

    return {
      sizePx: migrateLegacySize(parsed),
      color: pick(SUBTITLE_COLORS, parsed.color ?? "", SUBTITLE_COLORS[0]).id,
      outline: pick(SUBTITLE_OUTLINES, outline ?? "", SUBTITLE_OUTLINES[0]).id,
      background: pick(SUBTITLE_BACKGROUNDS, parsed.background ?? "", SUBTITLE_BACKGROUNDS[0]).id,
    };
  } catch {
    return DEFAULT_SUBTITLE_STYLE;
  }
}

export function persistSubtitleStyle(style: SubtitleStyle) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(style));
  } catch {
    // Private-mode storage failures must not break playback.
  }
}

export function stepSubtitleSize(sizePx: number, direction: -1 | 1): number {
  return clampSubtitleSizePx(sizePx + direction * SUBTITLE_SIZE_STEP);
}

/**
 * Native WebVTT cues ignore CSS variables inside `::cue` in most engines, so we inject
 * concrete declarations. Video.js v10 has no caption-appearance menu — page chrome owns this.
 */
export function applySubtitleStyle(style: SubtitleStyle) {
  const sizePx = clampSubtitleSizePx(style.sizePx);
  const color = pick(SUBTITLE_COLORS, style.color, SUBTITLE_COLORS[0]).value;
  const background = pick(SUBTITLE_BACKGROUNDS, style.background, SUBTITLE_BACKGROUNDS[0]).value;
  const textShadow = style.outline === "on" ? OUTLINE_SHADOW : "none";

  const css = `
.sudostream-video-player video::cue {
  font-size: ${sizePx}px !important;
  color: ${color} !important;
  background-color: ${background} !important;
  text-shadow: ${textShadow} !important;
}
.sudostream-video-player video::-webkit-media-text-track-display-backdrop {
  background-color: ${background} !important;
}
`.trim();

  let element = document.getElementById(STYLE_ELEMENT_ID) as HTMLStyleElement | null;
  if (!element) {
    element = document.createElement("style");
    element.id = STYLE_ELEMENT_ID;
    document.head.appendChild(element);
  }

  element.textContent = css;
}
