import { useMetadata, metadataDisplayName } from "@/hooks/use-metadata";

type MetadataItemLabelProps = {
  path: string;
  fallback: string;
  usesMetadata?: boolean;
};

export function MetadataItemLabel({ path, fallback, usesMetadata = true }: MetadataItemLabelProps) {
  const metadataQuery = useMetadata(path, usesMetadata);

  if (!usesMetadata || metadataQuery.isLoading || metadataQuery.isError) {
    return <>{fallback}</>;
  }

  return <>{metadataDisplayName(metadataQuery.data, fallback)}</>;
}
