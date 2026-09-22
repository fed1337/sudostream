import { client } from "@/api/client";
import { catchAllUrl } from "@/api/catch-all-url";
import { getApiBrowse, getApiLibraries } from "@/client/sdk.gen";
import type {
  GetApiBrowseByPathErrors,
  GetApiBrowseByPathResponses,
  SudoStreamInternalAccessLibrary,
  SudoStreamInternalMediafsBrowseResponse,
  SudoStreamInternalMediafsItem,
} from "@/client/types.gen";

export type Library = SudoStreamInternalAccessLibrary & {
  roots?: string[];
};
export type LibraryType = NonNullable<Library["type"]>;
export type BrowseResponse = SudoStreamInternalMediafsBrowseResponse;
export type MediaItem = SudoStreamInternalMediafsItem;

export async function fetchBrowse(
  path?: string,
  opts?: { limit?: number; offset?: number },
): Promise<BrowseResponse> {
  const trimmed = path?.replace(/^\/+/, "").replace(/\/+$/, "") ?? "";
  const query = {
    limit: opts?.limit,
    offset: opts?.offset,
  };

  const response = trimmed
    ? await client.get<GetApiBrowseByPathResponses, GetApiBrowseByPathErrors>({
        url: catchAllUrl("/api/browse", trimmed),
        query,
      })
    : await getApiBrowse({
        query,
      });

  if (response.error || !response.data) {
    throw new Error(getErrorMessage(response.error) ?? "browse failed");
  }

  return response.data;
}

export async function fetchLibraries(): Promise<Library[]> {
  const response = await getApiLibraries();

  if (response.error || !response.data) {
    throw new Error(getErrorMessage(response.error) ?? "list libraries failed");
  }

  return response.data.libraries ?? [];
}

export function libraryRoots(library: Library): string[] {
  if (library.roots && library.roots.length > 0) {
    return library.roots;
  }
  const fallback = (library.relPath ?? "").replace(/^\/+/, "");
  return fallback ? [fallback] : [];
}

export function pathWithinRoot(relPath: string, root: string): boolean {
  return relPath === root || relPath.startsWith(`${root}/`);
}

export function rootsOverlap(left: string, right: string): boolean {
  return pathWithinRoot(left, right) || pathWithinRoot(right, left);
}

/** Toggle a folder root, dropping any overlapping ancestor/descendant selection. */
export function toggleExclusiveRoot(selected: string[], relPath: string): string[] {
  if (selected.includes(relPath)) {
    return selected.filter((path) => path !== relPath);
  }

  return [...selected.filter((path) => !rootsOverlap(path, relPath)), relPath];
}

export function libraryBrowsePath(library: Library, subPath?: string): string {
  const nested = subPath?.replace(/^\/+/, "").replace(/\/+$/, "") ?? "";
  const roots = libraryRoots(library);
  if (roots.length <= 1) {
    const base = (roots[0] ?? library.slug ?? "").replace(/^\/+/, "");
    return nested ? `${base}/${nested}` : base;
  }

  return nested;
}

function getErrorMessage(error: unknown): string | undefined {
  if (!error || typeof error !== "object") {
    return undefined;
  }

  if ("error" in error && typeof error.error === "string") {
    return error.error;
  }

  return undefined;
}
