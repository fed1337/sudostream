import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  fetchMetadata,
  patchMetadata,
  type MetadataResponse,
  type PatchRequest,
} from "@/api/metadata";

export function metadataQueryKey(path: string) {
  return ["metadata", path] as const;
}

export function useMetadata(path: string | undefined, enabled = true) {
  return useQuery({
    queryKey: metadataQueryKey(path ?? ""),
    queryFn: () => fetchMetadata(path!),
    enabled: Boolean(path) && enabled,
  });
}

export function usePatchMetadata(path: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: PatchRequest) => patchMetadata(path, body),
    onSuccess: (data: MetadataResponse) => {
      queryClient.setQueryData(metadataQueryKey(path), data);
      void queryClient.invalidateQueries({ queryKey: ["metadata"] });
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
    },
  });
}

export function metadataDisplayName(
  metadata: MetadataResponse | undefined,
  fallback: string,
): string {
  if (!metadata) {
    return fallback;
  }

  if (!metadata.uses_metadata_for_display) {
    return fallback;
  }

  return metadata.display_name || fallback;
}
