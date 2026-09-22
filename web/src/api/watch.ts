import { client } from "@/api/client";
import { catchAllUrl } from "@/api/catch-all-url";
import type {
  GetApiWatchByPathErrors,
  GetApiWatchByPathResponses,
  PatchApiWatchByPathErrors,
  PatchApiWatchByPathResponses,
} from "@/client/types.gen";

export type WatchState = {
  watched: boolean;
  positionSeconds?: number;
  durationSeconds?: number;
};

export type WatchPatch = {
  watched?: boolean;
  positionSeconds?: number;
  durationSeconds?: number;
};

export async function fetchWatchState(mediaPath: string): Promise<WatchState> {
  const trimmed = mediaPath.replace(/^\/+/, "");
  const response = await client.get<GetApiWatchByPathResponses, GetApiWatchByPathErrors>({
    url: catchAllUrl("/api/watch", trimmed),
  });

  if (response.error || !response.data) {
    throw new Error("watch state failed");
  }

  return response.data as WatchState;
}

export async function patchWatchState(
  mediaPath: string,
  patch: WatchPatch | boolean,
): Promise<WatchState> {
  const body: WatchPatch = typeof patch === "boolean" ? { watched: patch } : patch;
  const trimmed = mediaPath.replace(/^\/+/, "");
  const response = await client.patch<PatchApiWatchByPathResponses, PatchApiWatchByPathErrors>({
    url: catchAllUrl("/api/watch", trimmed),
    body,
  });

  if (response.error || !response.data) {
    throw new Error("watch update failed");
  }

  return response.data as WatchState;
}
