import { type MouseEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { paginationItems, totalPages as calcTotalPages } from "@/lib/pagination";
import { cn } from "@/lib/utils";

type ListPagerProps = {
  page: number;
  total: number;
  pageSize: number;
  onPageChange: (page: number) => void;
};

/** Prev / next pager for catalog, browse, and search lists. */
export function ListPager({ page, total, pageSize, onPageChange }: ListPagerProps) {
  const { t } = useTranslation();
  const pages = calcTotalPages(total, pageSize);
  if (total <= pageSize) {
    return null;
  }

  const safePage = Math.min(Math.max(page, 1), pages);

  const goTo = (next: number, event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    if (next < 1 || next > pages || next === safePage) {
      return;
    }
    onPageChange(next);
  };

  return (
    <div className="flex flex-col items-center gap-2 pt-2">
      <p className="text-sm text-muted-foreground">
        {t("catalog.pageOf", { page: safePage, pages })}
      </p>
      <Pagination>
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href={`?page=${Math.max(1, safePage - 1)}`}
              text={t("catalog.prevPage")}
              aria-disabled={safePage <= 1}
              className={cn(safePage <= 1 && "pointer-events-none opacity-50")}
              onClick={(event) => {
                goTo(safePage - 1, event);
              }}
            />
          </PaginationItem>
          {paginationItems(safePage, pages).map((item, index) =>
            item === "ellipsis" ? (
              <PaginationItem key={`ellipsis-${index}`}>
                <PaginationEllipsis />
              </PaginationItem>
            ) : (
              <PaginationItem key={item}>
                <PaginationLink
                  href={`?page=${item}`}
                  isActive={item === safePage}
                  onClick={(event) => {
                    goTo(item, event);
                  }}
                >
                  {item}
                </PaginationLink>
              </PaginationItem>
            ),
          )}
          <PaginationItem>
            <PaginationNext
              href={`?page=${Math.min(pages, safePage + 1)}`}
              text={t("catalog.nextPage")}
              aria-disabled={safePage >= pages}
              className={cn(safePage >= pages && "pointer-events-none opacity-50")}
              onClick={(event) => {
                goTo(safePage + 1, event);
              }}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
