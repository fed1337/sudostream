import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRightIcon } from "lucide-react";
import { Link } from "react-router";

import { MediaPosterCard } from "@/components/media-poster-card";
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from "@/components/ui/carousel";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { mediaActionsForPath } from "@/lib/media-urls";
import { progressRatio } from "@/lib/watch-progress";

export type HomeShelfItem = {
  path?: string;
  title?: string;
  librarySlug?: string;
  posterUrl?: string;
  positionSeconds?: number;
  durationSeconds?: number;
};

type HomeShelfCarouselProps = {
  title: string;
  href: string;
  emptyTitle: string;
  emptyDescription: string;
  icon: ReactNode;
  items: HomeShelfItem[];
  watched?: boolean;
  favorited?: boolean;
  showProgress?: boolean;
};

export function HomeShelfCarousel({
  title,
  href,
  emptyTitle,
  emptyDescription,
  icon,
  items,
  watched = false,
  favorited = false,
  showProgress = false,
}: HomeShelfCarouselProps) {
  const { t } = useTranslation();

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xl font-semibold tracking-tight">
        <Link to={href} className="inline-flex items-center gap-1 hover:underline">
          {title}
          <ChevronRightIcon />
        </Link>
      </h2>
      {items.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">{icon}</EmptyMedia>
            <EmptyTitle>{emptyTitle}</EmptyTitle>
            <EmptyDescription>{emptyDescription}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Carousel
          className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 md:gap-4"
          opts={{ align: "start", dragFree: true }}
          aria-label={title}
        >
          <CarouselPrevious
            aria-label={t("home.carouselPrevious")}
            className="static inset-auto left-auto my-0 translate-none"
          />
          <CarouselContent className="min-w-0">
            {items.map((item) => {
              const path = item.path ?? "";
              const actions = mediaActionsForPath(path, { name: item.title });
              return (
                <CarouselItem key={path} className="basis-1/2 md:basis-80">
                  <MediaPosterCard
                    title={item.title || path}
                    subtitle={item.librarySlug}
                    thumbnailUrl={item.posterUrl}
                    fallbackThumbnailUrl={actions.thumbnail}
                    mediaPath={path}
                    downloadUrl={actions.download}
                    watched={watched}
                    favorited={favorited}
                    progressRatio={
                      showProgress
                        ? progressRatio(item.positionSeconds, item.durationSeconds)
                        : undefined
                    }
                    returnTo="/"
                  />
                </CarouselItem>
              );
            })}
          </CarouselContent>
          <CarouselNext
            aria-label={t("home.carouselNext")}
            className="static inset-auto right-auto my-0 translate-none"
          />
        </Carousel>
      )}
    </section>
  );
}
