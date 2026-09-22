/** Default page size for catalog tiles and browse lists (server contract). */
export const MEDIA_PAGE_SIZE = 28;

/** Convert 1-based page to API offset. */
export function pageToOffset(page: number, pageSize = MEDIA_PAGE_SIZE): number {
  const safePage = Number.isFinite(page) && page > 0 ? Math.floor(page) : 1;
  return (safePage - 1) * pageSize;
}

/** Parse 1-based page from URL search params. */
export function parsePageParam(raw: string | null): number {
  if (!raw) {
    return 1;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return 1;
  }
  return parsed;
}

/** Total page count from API total. */
export function totalPages(total: number, pageSize = MEDIA_PAGE_SIZE): number {
  if (total <= 0) {
    return 1;
  }
  return Math.max(1, Math.ceil(total / pageSize));
}

/** Compact page list for Pagination: numbers and ellipsis slots. */
export function paginationItems(page: number, pages: number): Array<number | "ellipsis"> {
  if (pages <= 7) {
    return Array.from({ length: pages }, (_, index) => index + 1);
  }

  const items: Array<number | "ellipsis"> = [1];
  const start = Math.max(2, page - 1);
  const end = Math.min(pages - 1, page + 1);
  if (start > 2) {
    items.push("ellipsis");
  }
  for (let number = start; number <= end; number += 1) {
    items.push(number);
  }
  if (end < pages - 1) {
    items.push("ellipsis");
  }
  items.push(pages);

  return items;
}
