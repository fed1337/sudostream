import { Link } from "react-router";
import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  DownloadIcon,
  EyeIcon,
  EyeOffIcon,
  FileIcon,
  FolderIcon,
  HeartIcon,
  LayoutGridIcon,
  LayoutListIcon,
  PlayIcon,
} from "lucide-react";

import type { MediaItem } from "@/api/media";
import { patchFavoriteState } from "@/api/favorite";
import { patchWatchState } from "@/api/watch";
import { DeleteMediaButton } from "@/components/delete-media-button";
import { AuthenticatedImage } from "@/components/authenticated-image";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useMetadata } from "@/hooks/use-metadata";
import {
  formatDuration,
  formatFileSize,
  isPhotoItem,
  isPlayableVideo,
  itemShowsDuration,
} from "@/lib/media-item";
import { authenticatedMediaUrl, playerPath } from "@/lib/media-urls";
import { cn } from "@/lib/utils";

export type BrowseLayout = "list" | "grid";

type BrowseLayoutToggleProps = {
  layout: BrowseLayout;
  onLayoutChange: (layout: BrowseLayout) => void;
  /** When true, grid is enabled (film/series catalog). Otherwise stays disabled stub. */
  gridEnabled?: boolean;
};

export function BrowseLayoutToggle({
  layout,
  onLayoutChange,
  gridEnabled = false,
}: BrowseLayoutToggleProps) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-1 rounded-lg border p-1 md:gap-1.5 md:p-1.5">
      <Button
        type="button"
        variant={layout === "list" ? "secondary" : "ghost"}
        size="icon"
        aria-pressed={layout === "list"}
        aria-label={t("browse.layoutList")}
        onClick={() => onLayoutChange("list")}
      >
        <LayoutListIcon />
      </Button>
      <Button
        type="button"
        variant={layout === "grid" ? "secondary" : "ghost"}
        size="icon"
        disabled={!gridEnabled}
        aria-pressed={layout === "grid"}
        aria-label={t("browse.layoutGrid")}
        title={gridEnabled ? t("browse.layoutGrid") : t("browse.layoutGridDisabled")}
        onClick={() => onLayoutChange("grid")}
      >
        <LayoutGridIcon />
      </Button>
    </div>
  );
}

type BrowseItemListProps = {
  items: MediaItem[];
  onOpenFolder: (item: MediaItem) => void;
  returnTo: string;
};

export function BrowseItemList({ items, onOpenFolder, returnTo }: BrowseItemListProps) {
  const { t } = useTranslation();

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="w-0 px-2" />
          <TableHead className="w-16 px-2" />
          <TableHead className="px-3">{t("browse.name")}</TableHead>
          <TableHead className="w-28 px-3">{t("browse.duration")}</TableHead>
          <TableHead className="w-24 px-3">{t("browse.size")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <BrowseItemRow
            key={item.path}
            item={item}
            onOpenFolder={onOpenFolder}
            returnTo={returnTo}
          />
        ))}
      </TableBody>
    </Table>
  );
}

type BrowseItemRowProps = {
  item: MediaItem;
  onOpenFolder: (item: MediaItem) => void;
  returnTo: string;
};

function BrowseItemRow({ item, onOpenFolder, returnTo }: BrowseItemRowProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const isFolder = item.isDir === true;
  const canPlay = !isFolder && isPlayableVideo(item) && Boolean(item.path);
  const canDownload = !isFolder && Boolean(item.actions?.download);
  const watched = item.watched === true;
  const favorited = item.favorited === true;

  const toggleWatched = useMutation({
    mutationFn: () => patchWatchState(item.path!, !watched),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({ queryKey: ["watch", item.path] });
    },
  });

  const toggleFavorite = useMutation({
    mutationFn: () => patchFavoriteState(item.path!, !favorited),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
    },
  });

  return (
    <TableRow
      className={cn(isFolder && "cursor-pointer")}
      onClick={isFolder ? () => onOpenFolder(item) : undefined}
    >
      <TableCell className="w-0 px-2 py-2">
        <div
          className="flex items-center gap-0.5"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          {canPlay && item.path ? (
            <>
              <IconTooltip label={watched ? t("browse.markUnwatched") : t("browse.markWatched")}>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-pressed={watched}
                  disabled={toggleWatched.isPending}
                  onClick={() => toggleWatched.mutate()}
                >
                  {watched ? <EyeOffIcon /> : <EyeIcon />}
                </Button>
              </IconTooltip>
              <IconTooltip label={favorited ? t("browse.unfavorite") : t("browse.favorite")}>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-pressed={favorited}
                  disabled={toggleFavorite.isPending}
                  onClick={() => toggleFavorite.mutate()}
                >
                  <HeartIcon className={favorited ? "fill-current text-red-500" : undefined} />
                </Button>
              </IconTooltip>
              <IconTooltip label={t("browse.play")}>
                <Button variant="ghost" size="icon" asChild>
                  <Link
                    to={playerPath(item.path, returnTo, {
                      mimeType: item.mimeType,
                      name: item.name,
                    })}
                  >
                    <PlayIcon />
                  </Link>
                </Button>
              </IconTooltip>
            </>
          ) : null}
          {canDownload && item.actions?.download ? (
            <IconTooltip label={t("browse.download")}>
              <Button variant="ghost" size="icon" asChild>
                <a href={authenticatedMediaUrl(item.actions.download)}>
                  <DownloadIcon />
                </a>
              </Button>
            </IconTooltip>
          ) : null}
          <DeleteMediaButton item={item} iconOnly />
        </div>
      </TableCell>
      <TableCell className="px-2 py-2">
        <BrowseItemThumbnail item={item} />
      </TableCell>
      <TableCell className="max-w-md px-3 py-2">
        <span className="truncate font-medium">{item.name}</span>
      </TableCell>
      <TableCell className="px-3 py-2 text-muted-foreground">
        <BrowseItemMetric item={item} />
      </TableCell>
      <TableCell className="px-3 py-2 text-muted-foreground">
        {isFolder ? "—" : formatFileSize(item.size)}
      </TableCell>
    </TableRow>
  );
}

function IconTooltip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent sideOffset={6}>{label}</TooltipContent>
    </Tooltip>
  );
}

function BrowseItemThumbnail({ item }: { item: MediaItem }) {
  const isFolder = item.isDir === true;

  return (
    <div className="flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
      {isFolder ? (
        <FolderIcon className="size-5 text-muted-foreground" />
      ) : item.actions?.thumbnail ? (
        <AuthenticatedImage
          src={item.actions.thumbnail}
          alt={item.name ?? ""}
          className="size-full object-cover"
        />
      ) : (
        <FileIcon className="size-5 text-muted-foreground" />
      )}
    </div>
  );
}

function BrowseItemMetric({ item }: { item: MediaItem }) {
  if (item.isDir) {
    return <>—</>;
  }

  if (isPhotoItem(item) && item.actions?.thumbnail) {
    return <PhotoResolution src={item.actions.thumbnail} />;
  }

  if (itemShowsDuration(item) && item.path) {
    return <VideoDuration path={item.path} />;
  }

  return <>—</>;
}

function VideoDuration({ path }: { path: string }) {
  const metadataQuery = useMetadata(path, true);
  const duration = metadataQuery.data?.effective?.duration_seconds;

  if (metadataQuery.isLoading) {
    return <>…</>;
  }

  if (duration === undefined || duration === null) {
    return <>—</>;
  }

  return <>{formatDuration(duration)}</>;
}

function PhotoResolution({ src }: { src: string }) {
  const [resolution, setResolution] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const image = new Image();

    image.onload = () => {
      if (active) {
        setResolution(`${image.naturalWidth}×${image.naturalHeight}`);
      }
    };
    image.src = authenticatedMediaUrl(src);

    return () => {
      active = false;
    };
  }, [src]);

  return <>{resolution ?? "…"}</>;
}

type BrowseBreadcrumbsProps = {
  crumbs: Array<{ name?: string; path?: string }>;
  librarySlug: string;
  roots: string[];
};

function toBrowseSubPath(roots: string[], fullPath: string | undefined): string | undefined {
  if (!fullPath) {
    return undefined;
  }
  if (roots.length <= 1) {
    const libraryRelPath = roots[0] ?? "";
    if (fullPath === libraryRelPath) {
      return undefined;
    }
    const prefix = `${libraryRelPath}/`;
    if (fullPath.startsWith(prefix)) {
      return fullPath.slice(prefix.length);
    }

    return undefined;
  }

  return fullPath;
}

export function BrowseBreadcrumbs({ crumbs, librarySlug, roots }: BrowseBreadcrumbsProps) {
  const { t } = useTranslation();
  const trail = crumbs.filter((crumb) => {
    if (!crumb.path) {
      return false;
    }

    return roots.some(
      (root) => crumb.path === root || (crumb.path?.startsWith(`${root}/`) ?? false),
    );
  });

  return (
    <nav aria-label="Breadcrumb">
      <ol className="flex flex-wrap items-center gap-1 text-base text-muted-foreground">
        <li>
          <Link to="/" className="hover:text-foreground">
            {t("nav.home")}
          </Link>
        </li>
        <li className="flex items-center gap-1.5">
          <span>/</span>
          <Link to={`/libraries/${librarySlug}/browse`} className="hover:text-foreground">
            {librarySlug}
          </Link>
        </li>
        {trail.map((crumb) => {
          const subPath = toBrowseSubPath(roots, crumb.path);
          return (
            <li key={crumb.path} className="flex items-center gap-1.5">
              <span>/</span>
              {subPath ? (
                <Link
                  to={`/libraries/${librarySlug}/browse/${subPath}`}
                  className="hover:text-foreground"
                >
                  {crumb.name}
                </Link>
              ) : (
                <span>{crumb.name}</span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
