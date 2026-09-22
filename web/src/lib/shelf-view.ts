const storageKey = "sudostream.shelfView";

export type ShelfViewMode = "tiles" | "list";

export function getShelfViewMode(): ShelfViewMode {
  if (typeof window === "undefined") {
    return "tiles";
  }
  const raw = window.localStorage.getItem(storageKey);
  if (raw === "list" || raw === "tiles") {
    return raw;
  }

  return "tiles";
}

export function setShelfViewMode(mode: ShelfViewMode): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(storageKey, mode);
}
