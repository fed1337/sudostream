import type { Library, LibraryType } from "@/api/media";

export const LIBRARY_TYPES: LibraryType[] = ["film", "series", "music", "photos", "other"];

export function libraryTypeLabelKey(type: LibraryType): string {
  return `library.types.${type}`;
}

export function libraryEntryPath(library: Library): string {
  const slug = library.slug ?? library.relPath ?? "";
  if (library.type === "film" || library.type === "series") {
    return `/libraries/${slug}`;
  }

  return `/libraries/${slug}/browse`;
}
