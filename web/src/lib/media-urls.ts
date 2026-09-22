import { getAccessToken } from "@/lib/auth-storage";

const host = import.meta.env.VITE_HOST ?? "";

export function absoluteApiUrl(path: string): string {
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }

  return `${host}${path}`;
}

/**
 * Appends the current access token as a query param. Chromecast receivers and
 * other clients that cannot set an Authorization header rely on this instead
 * of the Bearer header the browser player uses (`/api/*` accepts either).
 */
export function withAccessToken(url: string): string {
  const token = getAccessToken();
  if (!token) {
    return url;
  }

  const separator = url.includes("?") ? "&" : "?";
  return `${url}${separator}access_token=${encodeURIComponent(token)}`;
}

export function authenticatedMediaUrl(apiPath: string): string {
  return withAccessToken(absoluteApiUrl(apiPath));
}

export function encodeMediaPath(relPath: string): string {
  return relPath
    .replace(/^\/+/, "")
    .split("/")
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}

const VIDEO_EXTENSIONS = new Set([
  ".mp4",
  ".mkv",
  ".webm",
  ".avi",
  ".mov",
  ".m4v",
  ".wmv",
  ".mpg",
  ".mpeg",
]);

export function isVideoFilename(name: string): boolean {
  const dot = name.lastIndexOf(".");
  if (dot < 0) {
    return false;
  }

  return VIDEO_EXTENSIONS.has(name.slice(dot).toLowerCase());
}

export function isVideoMime(mimeType: string | null | undefined): boolean {
  return mimeType?.startsWith("video/") ?? false;
}

export function canStreamMedia(options: { name: string; mimeType?: string | null }): boolean {
  return isVideoMime(options.mimeType) || isVideoFilename(options.name);
}

export function mediaActionsForPath(
  relPath: string,
  options?: { mimeType?: string | null; name?: string },
): {
  play?: string;
  thumbnail?: string;
  download: string;
  name: string;
  canDelete?: boolean;
} {
  const trimmed = relPath.replace(/^\/+/, "");
  const encoded = encodeMediaPath(trimmed);
  const fileName = trimmed.split("/").pop() ?? trimmed;
  // Display/download name may be a metadata title without an extension; streamability
  // and thumbnails must use the real path basename.
  const name = options?.name ?? fileName;
  const streamable = canStreamMedia({ name: fileName, mimeType: options?.mimeType });
  const hasThumbnail = streamable || options?.mimeType?.startsWith("image/");

  return {
    name,
    download: `/api/download/${encoded}`,
    play: streamable ? `/api/play/${encoded}/master.m3u8` : undefined,
    thumbnail: hasThumbnail ? `/api/thumbnail/${encoded}` : undefined,
  };
}

export function playerPath(
  mediaPath: string,
  returnTo: string,
  options?: { mimeType?: string | null; name?: string },
): string {
  const trimmed = mediaPath.replace(/^\/+/, "");
  const params = new URLSearchParams({ return: returnTo });
  const name = options?.name ?? trimmed.split("/").pop() ?? trimmed;
  if (options?.mimeType) {
    params.set("mime", options.mimeType);
  }
  params.set("title", name);
  return `/play/${trimmed}?${params.toString()}`;
}
