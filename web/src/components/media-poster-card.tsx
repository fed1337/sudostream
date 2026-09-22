import { Link } from "react-router";
import { useState, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
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
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { authenticatedMediaUrl, playerPath } from "@/lib/media-urls";
import { cn } from "@/lib/utils";

type MediaPosterCardProps = {
  title: string;
  subtitle?: string;
  /** Preferred image (e.g. provider poster). */
  thumbnailUrl?: string;
  /** Tried when thumbnailUrl fails (e.g. generated /api/thumbnail). */
  fallbackThumbnailUrl?: string;
  href?: string;
  mediaPath?: string;
  downloadUrl?: string;
  watched?: boolean;
  favorited?: boolean;
  progressRatio?: number;
  returnTo?: string;
  footer?: ReactNode;
  className?: string;
};

export function MediaPosterCard({
  title,
  subtitle,
  thumbnailUrl,
  fallbackThumbnailUrl,
  href,
  mediaPath,
  downloadUrl,
  watched = false,
  favorited = false,
  progressRatio,
  returnTo,
  footer,
  className,
}: MediaPosterCardProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const sources = [thumbnailUrl, fallbackThumbnailUrl].filter(
    (value, index, list): value is string => Boolean(value) && list.indexOf(value) === index,
  );
  const sourceKey = sources.join("|");
  const playReturnTo =
    returnTo ??
    (typeof window !== "undefined" ? window.location.pathname + window.location.search : "/");
  const playHref =
    mediaPath && !href
      ? playerPath(mediaPath, playReturnTo, {
          name: mediaPath.split("/").pop() ?? title,
        })
      : undefined;
  const linkHref = href ?? playHref;

  const toggleWatched = useMutation({
    mutationFn: () => patchWatchState(mediaPath!, !watched),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({ queryKey: ["watch", mediaPath] });
      void queryClient.invalidateQueries({ queryKey: getApiMeWatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeUnwatchedQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeContinueQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getApiMeStatsQueryKey() });
    },
  });

  const toggleFavorite = useMutation({
    mutationFn: () => patchFavoriteState(mediaPath!, !favorited),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browse"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog"] });
      void queryClient.invalidateQueries({ queryKey: ["catalog-show"] });
      void queryClient.invalidateQueries({ queryKey: getApiMeFavoritesQueryKey() });
    },
  });

  const letter = (
    <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
      {title.slice(0, 1).toUpperCase()}
    </div>
  );
  const poster = (
    <div className="relative aspect-video w-full overflow-hidden rounded-md bg-muted">
      <PosterSources key={sourceKey} sources={sources} title={title} letter={letter} />
      {progressRatio !== undefined ? (
        <div className="absolute inset-x-0 bottom-0 h-1 bg-muted/80">
          <div
            className="h-full bg-primary"
            style={{ width: `${Math.min(100, Math.max(0, progressRatio * 100))}%` }}
          />
        </div>
      ) : null}
    </div>
  );

  return (
    <article className={cn("flex flex-col gap-2", className)}>
      {linkHref ? (
        <Link to={linkHref} className="block transition-opacity hover:opacity-90">
          {poster}
        </Link>
      ) : (
        poster
      )}
      <div className="flex flex-col gap-1">
        {linkHref ? (
          <Link to={linkHref} className="line-clamp-2 text-sm font-medium hover:underline">
            {title}
          </Link>
        ) : (
          <p className="line-clamp-2 text-sm font-medium">{title}</p>
        )}
        {subtitle ? <p className="text-xs text-muted-foreground">{subtitle}</p> : null}
      </div>
      <div className="mt-auto flex flex-wrap gap-0.5">
        {mediaPath ? (
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
                  to={playerPath(mediaPath, playReturnTo, {
                    name: mediaPath.split("/").pop() ?? title,
                  })}
                >
                  <PlayIcon />
                </Link>
              </Button>
            </IconTooltip>
          </>
        ) : null}
        {downloadUrl ? (
          <IconTooltip label={t("browse.download")}>
            <Button variant="ghost" size="icon" asChild>
              <a href={authenticatedMediaUrl(downloadUrl)} download>
                <DownloadIcon />
              </a>
            </Button>
          </IconTooltip>
        ) : null}
        {footer}
      </div>
    </article>
  );
}

function PosterSources({
  sources,
  title,
  letter,
}: {
  sources: string[];
  title: string;
  letter: ReactNode;
}) {
  const [sourceIndex, setSourceIndex] = useState(0);
  const activeSrc = sources[sourceIndex];
  if (!activeSrc) {
    return letter;
  }

  return (
    <AuthenticatedImage
      key={activeSrc}
      src={activeSrc}
      alt={title}
      className="size-full object-cover object-center"
      onError={() => setSourceIndex((index) => index + 1)}
    />
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
