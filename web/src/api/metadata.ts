import { client } from "@/api/client";
import { catchAllUrl } from "@/api/catch-all-url";
import type {
  GetApiMetadataByPathErrors,
  GetApiMetadataByPathResponses,
  PatchApiMetadataByPathErrors,
  PatchApiMetadataByPathResponses,
  SudoStreamInternalMetadataMetadataResponse,
  SudoStreamInternalMetadataVideoFields,
} from "@/client/types.gen";

export type MetadataResponse = SudoStreamInternalMetadataMetadataResponse;
export type VideoFields = SudoStreamInternalMetadataVideoFields;
export type PatchRequest = {
  target: "override" | "file";
  fields: Record<string, string | number | string[] | null>;
};

export async function fetchMetadata(path: string): Promise<MetadataResponse> {
  const trimmed = path.replace(/^\/+/, "");
  const response = await client.get<GetApiMetadataByPathResponses, GetApiMetadataByPathErrors>({
    url: catchAllUrl("/api/metadata", trimmed),
  });

  if (response.error || !response.data) {
    throw new Error(getErrorMessage(response.error) ?? "metadata fetch failed");
  }

  return response.data;
}

export async function patchMetadata(path: string, body: PatchRequest): Promise<MetadataResponse> {
  const trimmed = path.replace(/^\/+/, "");
  const response = await client.patch<
    PatchApiMetadataByPathResponses,
    PatchApiMetadataByPathErrors
  >({
    url: catchAllUrl("/api/metadata", trimmed),
    body,
  });

  if (response.error || !response.data) {
    throw new Error(getErrorMessage(response.error) ?? "metadata patch failed");
  }

  return response.data;
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
