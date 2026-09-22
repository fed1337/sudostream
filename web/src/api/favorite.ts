import { client } from "@/api/client";
import { catchAllUrl } from "@/api/catch-all-url";
import type {
  PatchApiFavoriteByPathErrors,
  PatchApiFavoriteByPathResponses,
} from "@/client/types.gen";

export async function patchFavoriteState(
  mediaPath: string,
  favorite: boolean,
): Promise<{ favorite: boolean }> {
  const trimmed = mediaPath.replace(/^\/+/, "");
  const response = await client.patch<
    PatchApiFavoriteByPathResponses,
    PatchApiFavoriteByPathErrors
  >({
    url: catchAllUrl("/api/favorite", trimmed),
    body: { favorite },
  });

  if (response.error || !response.data) {
    throw new Error("favorite update failed");
  }

  return response.data as { favorite: boolean };
}
