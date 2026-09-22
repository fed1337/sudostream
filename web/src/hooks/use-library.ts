import { useQuery } from "@tanstack/react-query";

import { fetchLibraries, type Library } from "@/api/media";

export function useLibrary(slug: string | undefined) {
  const librariesQuery = useQuery({
    queryKey: ["libraries"],
    queryFn: fetchLibraries,
  });

  const library = librariesQuery.data?.find((entry) => entry.slug === slug);

  return {
    library,
    librariesQuery,
  };
}

export function libraryPaths(library: Library | undefined, slug: string | undefined) {
  const libraryRelPath = library?.relPath ?? library?.slug ?? slug ?? "";
  const librarySlug = library?.slug ?? slug ?? "";

  return { libraryRelPath, librarySlug };
}
