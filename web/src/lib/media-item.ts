import type { MediaItem } from "@/api/media";

export function isPlayableVideo(item: MediaItem): boolean {
  return Boolean(item.actions?.play);
}

export function isPhotoItem(item: MediaItem): boolean {
  return item.mimeType?.startsWith("image/") ?? false;
}

export function itemShowsDuration(item: MediaItem): boolean {
  if (item.isDir) {
    return false;
  }

  return isPlayableVideo(item) || (item.mimeType?.startsWith("audio/") ?? false);
}

export function formatFileSize(bytes: number | undefined): string {
  if (bytes === undefined || bytes < 0) {
    return "—";
  }

  if (bytes < 1024) {
    return `${bytes} B`;
  }

  const units = ["KB", "MB", "GB", "TB"] as const;
  let value = bytes / 1024;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unitIndex]}`;
}

export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;

  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
  }

  return `${minutes}:${String(secs).padStart(2, "0")}`;
}
