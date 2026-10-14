import { client } from "@/api/client";
import { catchAllUrl } from "@/api/catch-all-url";
import type {
  DeleteApiMediaByPathErrors,
  DeleteApiMediaByPathResponses,
  GetApiPlaybackByPathErrors,
  GetApiPlaybackByPathResponses,
} from "@/client/types.gen";
import { getAccessToken } from "@/lib/auth-storage";
import type { DeviceProfile } from "@/lib/device-profile";
import { absoluteApiUrl } from "@/lib/media-urls";

export type PlaybackStatus = "idle" | "processing" | "ready" | "error";

export type PlayMethod = "directPlay" | "remux" | "transcode";

export class DeleteMediaError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "DeleteMediaError";
    this.status = status;
  }
}

export type PlaybackInfo = {
  status: PlaybackStatus;
  error?: string;
  durationSeconds?: number;
  masterUrl?: string;
  streamUrl?: string;
  playMethod?: PlayMethod;
  packagingMode?: "remux" | "transcode";
  qualities?: Array<{ height: number; label: string }>;
  audioTracks?: Array<{ id: string; label: string }>;
  subtitleTracks?: Array<{ id: string; label: string }>;
  providerSubtitleTracks?: Array<{ lang: string; label: string; url: string }>;
  userSubtitle?: { lang: string; label: string; url: string };
  /** Container chapters for any video file (not series S/E identity). */
  chapters?: Array<{ startSeconds: number; endSeconds: number; title: string }>;
  skipIntro?: { startMs: number; endMs: number; source?: string };
  series?: {
    librarySlug: string;
    showKey: string;
    showName: string;
    season: number;
    episode: number;
  };
};

type FetchPlaybackOptions = {
  transcode?: boolean;
  fallback?: boolean;
  deviceProfile?: DeviceProfile;
  qualityHeight?: number;
};

export async function fetchPlayback(
  mediaPath: string,
  options: FetchPlaybackOptions = {},
): Promise<PlaybackInfo> {
  const trimmed = mediaPath.replace(/^\/+/, "");

  if (options.deviceProfile) {
    const response = await client.post({
      url: catchAllUrl("/api/playback", trimmed),
      body: {
        deviceProfile: options.deviceProfile,
        qualityHeight: options.qualityHeight || undefined,
        transcode: options.transcode ? true : undefined,
        fallback: options.fallback ? true : undefined,
      },
      headers: { "Content-Type": "application/json" },
    });

    if (response.error || !response.data) {
      throw new Error("playback negotiate failed");
    }

    return response.data as PlaybackInfo;
  }

  const response = await client.get<GetApiPlaybackByPathResponses, GetApiPlaybackByPathErrors>({
    url: catchAllUrl("/api/playback", trimmed),
    query: {
      transcode: options.transcode ? true : undefined,
      fallback: options.fallback ? true : undefined,
    },
  });

  if (response.error || !response.data) {
    throw new Error("playback status failed");
  }

  return response.data as PlaybackInfo;
}

export async function deleteMedia(path: string, recursive = false): Promise<void> {
  const trimmed = path.replace(/^\/+/, "");
  const response = await client.delete<DeleteApiMediaByPathResponses, DeleteApiMediaByPathErrors>({
    url: catchAllUrl("/api/media", trimmed),
    query: recursive ? { recursive: true } : undefined,
  });

  const status = response.response?.status;
  if (response.error || (status !== 204 && status !== 200)) {
    const body = response.error as { error?: string } | undefined;
    const message = body?.error ?? "delete failed";
    throw new DeleteMediaError(message, status ?? 0);
  }
}

export function isHlsPlaybackReady(status: PlaybackStatus | undefined): boolean {
  return status === "ready";
}

export async function uploadUserSubtitle(
  mediaPath: string,
  file: File,
  options?: { lang?: string; label?: string },
): Promise<{ lang: string; label: string; url: string }> {
  const trimmed = mediaPath.replace(/^\/+/, "");
  const body = new FormData();
  body.append("file", file);
  if (options?.lang) {
    body.append("lang", options.lang);
  }
  if (options?.label) {
    body.append("label", options.label);
  }

  const token = getAccessToken();
  const headers: HeadersInit = {};
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const response = await fetch(absoluteApiUrl(catchAllUrl("/api/user-subtitle", trimmed)), {
    method: "PUT",
    headers,
    body,
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error("subtitle upload failed");
  }

  return (await response.json()) as { lang: string; label: string; url: string };
}
