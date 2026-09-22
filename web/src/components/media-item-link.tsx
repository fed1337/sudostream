import { Link } from "react-router";
import type { ReactNode } from "react";

import type { MediaItem } from "@/api/media";
import { playerPath } from "@/lib/media-urls";
import { isPlayableVideo } from "@/lib/media-item";
import { cn } from "@/lib/utils";

type MediaItemLinkProps = {
  item: MediaItem;
  returnTo: string;
  children: ReactNode;
  className?: string;
};

export function MediaItemLink({ item, returnTo, children, className }: MediaItemLinkProps) {
  const openable = Boolean(item.path) && (isPlayableVideo(item) || Boolean(item.actions?.download));

  if (!openable || !item.path) {
    return <span className={className}>{children}</span>;
  }

  return (
    <Link
      to={playerPath(item.path, returnTo, {
        mimeType: item.mimeType,
        name: item.name,
      })}
      className={cn("hover:text-primary hover:underline", className)}
    >
      {children}
    </Link>
  );
}
