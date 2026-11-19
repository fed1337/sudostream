import type { ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { DownloadIcon, EyeIcon, EyeOffIcon, HeartIcon, PlayIcon } from "lucide-react";

import { patchFavoriteState } from "@/api/favorite";
import { patchWatchState } from "@/api/watch";
import {
  getApiMeContinueQueryKey,
  getApiMeFavoritesQueryKey,
  getApiMeStatsQueryKey,
  getApiMeUnwatchedQueryKey,
  getApiMeWatchedQueryKey,
} from "@/client/@tanstack/react-query.gen";
import { AuthenticatedImage } from "@/components/authenticated-image";
import type { HomeShelfItem } from "@/components/home-shelf-carousel";
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
import { authenticatedMediaUrl, mediaActionsForPath, playerPath } from "@/lib/media-urls";
import { progressRatio } from "@/lib/watch-progress";

type ShelfItemListProps = {
  items: HomeShelfItem[];
  watched?: boolean;
  favorited?: boolean;
  showProgress?: boolean;
  returnTo: string;
};

export function ShelfItemList({
  items,
  watched = false,
  favorited = false,
  showProgress = false,
  returnTo,
}: ShelfItemListProps) {
  const { t } = useTranslation();

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="w-16 px-2" />
          <TableHead className="px-3">{t("browse.name")}</TableHead>
          {showProgress ? <TableHead className="w-32 px-3">{t("home.progress")}</TableHead> : null}
          <TableHead className="w-40 px-2" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <ShelfItemRow
            key={item.path}
            item={item}
            watched={watched}
            favorited={favorited}
            showProgress={showProgress}
            returnTo={returnTo}
          />
        ))}
      </TableBody>
    </Table>
  );
}

function ShelfItemRow({
  item,
  watched,
  favorited,
  showProgress,
  returnTo,
}: {
  item: HomeShelfItem;
  watched: boolean;
  favorited: boolean;
  showProgress: boolean;
  returnTo: string;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const path = item.path ?? "";
  const actions = mediaActionsForPath(path, { name: item.title });
  const ratio = progressRatio(item.positionSeconds, item.durationSeconds);

  const isWatched = item.watched ?? watched;
  const isFavorited = item.favorited ?? favorited;
  const libraryLabel = item.libraryName || item.librarySlug;

  const toggleWatched = useMutation({
    mutationFn: () => patchWatchState(path, !isWatched),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: getApiMeWatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeUnwatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeContinueQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeFavoritesQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeStatsQueryKey() });
    },
  });
  const toggleFavorite = useMutation({
    mutationFn: () => patchFavoriteState(path, !isFavorited),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: getApiMeFavoritesQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeWatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeUnwatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeContinueQueryKey() });
    },
  });

  return (
    <TableRow>
      <TableCell className="px-2">
        <div className="size-12 overflow-hidden rounded-md bg-muted">
          {item.posterUrl || actions.thumbnail ? (
            <AuthenticatedImage
              src={item.posterUrl ?? actions.thumbnail!}
              alt={item.title || path}
              className="size-full object-cover"
            />
          ) : null}
        </div>
      </TableCell>
      <TableCell className="px-3">
        <Link
          to={playerPath(path, returnTo, { name: path.split("/").pop() ?? item.title })}
          className="font-medium hover:underline"
        >
          {item.title || path}
        </Link>
        {libraryLabel ? <p className="text-xs text-muted-foreground">{libraryLabel}</p> : null}
      </TableCell>
      {showProgress ? (
        <TableCell className="px-3">
          {ratio !== undefined ? (
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <div className="h-full bg-primary" style={{ width: `${ratio * 100}%` }} />
            </div>
          ) : null}
        </TableCell>
      ) : null}
      <TableCell className="px-2">
        <div className="flex justify-end">
          <IconTooltip label={isWatched ? t("browse.markUnwatched") : t("browse.markWatched")}>
            <Button
              variant="ghost"
              size="icon"
              aria-pressed={isWatched}
              disabled={toggleWatched.isPending}
              onClick={() => toggleWatched.mutate()}
            >
              {isWatched ? <EyeOffIcon /> : <EyeIcon />}
            </Button>
          </IconTooltip>
          <IconTooltip label={isFavorited ? t("browse.unfavorite") : t("browse.favorite")}>
            <Button
              variant="ghost"
              size="icon"
              aria-pressed={isFavorited}
              disabled={toggleFavorite.isPending}
              onClick={() => toggleFavorite.mutate()}
            >
              <HeartIcon className={isFavorited ? "fill-current text-red-500" : undefined} />
            </Button>
          </IconTooltip>
          <IconTooltip label={t("browse.play")}>
            <Button variant="ghost" size="icon" asChild>
              <Link to={playerPath(path, returnTo, { name: path.split("/").pop() ?? item.title })}>
                <PlayIcon />
              </Link>
            </Button>
          </IconTooltip>
          {actions.download ? (
            <IconTooltip label={t("browse.download")}>
              <Button variant="ghost" size="icon" asChild>
                <a href={authenticatedMediaUrl(actions.download)} download>
                  <DownloadIcon />
                </a>
              </Button>
            </IconTooltip>
          ) : null}
        </div>
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
