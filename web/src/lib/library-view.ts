const storagePrefix = "sudostream.libraryView.";

export type LibraryViewMode = "catalog" | "files";

export function getLibraryViewMode(slug: string): LibraryViewMode {
  if (typeof window === "undefined") {
    return "catalog";
  }
  const raw = window.localStorage.getItem(storagePrefix + slug);
  if (raw === "files" || raw === "catalog") {
    return raw;
  }

  return "catalog";
}

export function setLibraryViewMode(slug: string, mode: LibraryViewMode): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(storagePrefix + slug, mode);
}
