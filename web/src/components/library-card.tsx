import { Link } from "react-router";

import type { Library } from "@/api/media";
import { LibraryTypeBadge } from "@/components/library-type-badge";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { libraryEntryPath } from "@/lib/library-types";

type LibraryCardProps = {
  library: Library;
};

export function LibraryCard({ library }: LibraryCardProps) {
  return (
    <Link
      to={libraryEntryPath(library)}
      className="block h-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
    >
      <Card className="h-full min-h-32 transition-colors hover:bg-muted/40 md:min-h-40">
        <CardHeader className="flex h-full flex-col justify-between gap-4">
          <CardTitle className="text-base leading-snug md:text-xl">
            {library.name ?? library.slug}
          </CardTitle>
          <LibraryTypeBadge type={library.type} />
        </CardHeader>
      </Card>
    </Link>
  );
}
