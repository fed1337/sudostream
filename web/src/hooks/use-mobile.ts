import * as React from "react";

/**
 * Single UI breakpoint — keep in sync with `--breakpoint-*` / `--container-*` in `src/index.css`
 * and Video.js `@container media-root (width > 42rem)` (672px at 16px root).
 *
 * Mobile: `default` controls. Desktop/TV (≥672px): `lg` controls.
 */
export const MOBILE_BREAKPOINT_PX = 672;

function matchesMobile() {
  return window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT_PX - 1}px)`).matches;
}

export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean>(() =>
    typeof window === "undefined" ? false : matchesMobile(),
  );

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT_PX - 1}px)`);
    const onChange = () => {
      setIsMobile(mql.matches);
    };
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return isMobile;
}

/** Explicit control size when a surface is locked to one density (e.g. desktop sidebar). */
export function useControlSize(): "default" | "lg" {
  return useIsMobile() ? "default" : "lg";
}
