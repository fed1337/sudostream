import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { FolderOpenIcon, HardDriveIcon } from "lucide-react";

import {
  fetchBrowse,
  fetchLibraries,
  libraryBrowsePath,
  libraryRoots,
  type MediaItem,
} from "@/api/media";
import {
  BrowseBreadcrumbs,
  BrowseItemList,
  BrowseLayoutToggle,
  type BrowseLayout,
} from "@/components/browse-items";
import { ListPager } from "@/components/list-pager";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { getLibraryViewMode, setLibraryViewMode } from "@/lib/library-view";
import { MEDIA_PAGE_SIZE, pageToOffset, parsePageParam } from "@/lib/pagination";
import { cn } from "@/lib/utils";

function folderHref(slug: string, roots: string[], itemPath: string | undefined): string {
  if (!itemPath) {
    return `/libraries/${slug}/browse`;
  }

  if (roots.length <= 1) {
    const libraryRelPath = roots[0] ?? "";
    const prefix = `${libraryRelPath}/`;
    const subPath = itemPath.startsWith(prefix)
      ? itemPath.slice(prefix.length)
      : itemPath === libraryRelPath
        ? ""
        : itemPath;

    return subPath ? `/libraries/${slug}/browse/${subPath}` : `/libraries/${slug}/browse`;
  }

  return `/libraries/${slug}/browse/${itemPath}`;
}

function libraryRelativeCrumbs(
  roots: string[],
  breadcrumbs: Array<{ name?: string; path?: string }>,
) {
  return breadcrumbs.filter((crumb) => {
    if (!crumb.path) {
      return false;
    }

    return roots.some(
      (root) => crumb.path === root || (crumb.path?.startsWith(`${root}/`) ?? false),
    );
  });
}

function MediaEmptyState({ title, description }: { title: string; description: string }) {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <HardDriveIcon />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function BrowsePageShell({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn("flex flex-col gap-4", className)}>{children}</div>;
}

function BrowsePageHeader({
  libraryName,
  crumbs,
  librarySlug,
  roots,
  layout,
  onLayoutChange,
  gridEnabled,
}: {
  libraryName: string;
  crumbs: Array<{ name?: string; path?: string }>;
  librarySlug: string;
  roots: string[];
  layout: BrowseLayout;
  onLayoutChange: (layout: BrowseLayout) => void;
  gridEnabled: boolean;
}) {
  return (
    <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-3xl font-semibold tracking-tight">{libraryName}</h1>
        <BrowseBreadcrumbs crumbs={crumbs} librarySlug={librarySlug} roots={roots} />
      </div>
      <BrowseLayoutToggle
        layout={layout}
        onLayoutChange={onLayoutChange}
        gridEnabled={gridEnabled}
      />
    </div>
  );
}

export default function LibraryBrowsePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { slug, "*": splatPath } = useParams<{ slug: string; "*": string | undefined }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parsePageParam(searchParams.get("page"));
  const [layout, setLayout] = useState<BrowseLayout>("list");
  const isLibraryRoot = !splatPath;

  const librariesQuery = useQuery({
    queryKey: ["libraries"],
    queryFn: fetchLibraries,
  });

  const library = librariesQuery.data?.find((entry) => entry.slug === slug);
  const catalogCapable = library?.type === "film" || library?.type === "series";
  const roots = library ? libraryRoots(library) : [];
  const isVirtualRoot = roots.length > 1 && isLibraryRoot;

  useEffect(() => {
    if (catalogCapable && slug && isLibraryRoot && getLibraryViewMode(slug) === "catalog") {
      void navigate(`/libraries/${slug}`, { replace: true });
    }
  }, [catalogCapable, slug, isLibraryRoot, navigate]);

  const prevSplatPath = useRef(splatPath);
  useEffect(() => {
    if (prevSplatPath.current === splatPath) {
      return;
    }
    prevSplatPath.current = splatPath;
    setSearchParams(
      (prev) => {
        if (!prev.has("page")) {
          return prev;
        }
        const next = new URLSearchParams(prev);
        next.delete("page");
        return next;
      },
      { replace: true },
    );
  }, [splatPath, setSearchParams]);

  const browseQuery = useQuery({
    queryKey: ["browse", library?.id, splatPath, page],
    queryFn: () =>
      fetchBrowse(libraryBrowsePath(library!, splatPath), {
        limit: MEDIA_PAGE_SIZE,
        offset: pageToOffset(page),
      }),
    enabled: Boolean(library) && !isVirtualRoot,
  });

  const setPage = (next: number) => {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        if (next <= 1) {
          params.delete("page");
        } else {
          params.set("page", String(next));
        }
        return params;
      },
      { replace: true },
    );
  };

  const handleLayoutChange = (next: BrowseLayout) => {
    if (next === "grid" && catalogCapable && slug) {
      setLibraryViewMode(slug, "catalog");
      void navigate(`/libraries/${slug}`);
      return;
    }

    if (next === "list" && catalogCapable && slug) {
      setLibraryViewMode(slug, "files");
    }

    setLayout(next);
  };

  if (librariesQuery.isLoading) {
    return (
      <BrowsePageShell>
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-5 w-80" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </BrowsePageShell>
    );
  }

  if (librariesQuery.isError) {
    return (
      <BrowsePageShell>
        <Alert variant="destructive">
          <AlertTitle>{t("library.loadError")}</AlertTitle>
          <AlertDescription>{t("common.error")}</AlertDescription>
        </Alert>
      </BrowsePageShell>
    );
  }

  if (!library) {
    return (
      <BrowsePageShell>
        <MediaEmptyState
          title={t("library.libraryNotFoundTitle")}
          description={t("library.libraryNotFoundDescription")}
        />
      </BrowsePageShell>
    );
  }

  if (!isVirtualRoot && browseQuery.isLoading) {
    return (
      <BrowsePageShell>
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-5 w-80" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </BrowsePageShell>
    );
  }

  if (!isVirtualRoot && (browseQuery.isError || !browseQuery.data)) {
    if (isLibraryRoot) {
      return (
        <BrowsePageShell>
          <MediaEmptyState
            title={t("library.mediaEmptyTitle")}
            description={t("library.mediaEmptyDescription")}
          />
        </BrowsePageShell>
      );
    }

    return (
      <BrowsePageShell>
        <Alert variant="destructive">
          <AlertTitle>{t("library.loadError")}</AlertTitle>
          <AlertDescription>
            {browseQuery.error instanceof Error ? browseQuery.error.message : t("common.error")}
          </AlertDescription>
        </Alert>
      </BrowsePageShell>
    );
  }

  const librarySlug = library.slug ?? slug ?? "";
  const virtualItems: MediaItem[] = isVirtualRoot
    ? roots.map((root) => ({
        name: root.split("/").pop() ?? root,
        path: root,
        isDir: true,
        mimeType: "inode/directory",
      }))
    : [];
  const children = isVirtualRoot ? virtualItems : (browseQuery.data?.folder?.children ?? []);
  const total = isVirtualRoot ? virtualItems.length : (browseQuery.data?.total ?? children.length);
  const crumbs = isVirtualRoot
    ? []
    : libraryRelativeCrumbs(roots, browseQuery.data?.breadcrumbs ?? []);
  const returnTo = splatPath
    ? `/libraries/${librarySlug}/browse/${splatPath}`
    : `/libraries/${librarySlug}/browse`;

  const openFolder = (item: MediaItem) => {
    void navigate(folderHref(librarySlug, roots, item.path));
  };

  if (total === 0) {
    return (
      <BrowsePageShell>
        <BrowsePageHeader
          libraryName={library.name ?? librarySlug}
          crumbs={crumbs}
          librarySlug={librarySlug}
          roots={roots}
          layout={layout}
          onLayoutChange={handleLayoutChange}
          gridEnabled={catalogCapable}
        />
        {isLibraryRoot ? (
          <MediaEmptyState
            title={t("library.mediaEmptyTitle")}
            description={t("library.mediaEmptyDescription")}
          />
        ) : (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FolderOpenIcon />
              </EmptyMedia>
              <EmptyTitle>{t("library.emptyFolder")}</EmptyTitle>
              <EmptyDescription>{t("library.emptyFolderDescription")}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </BrowsePageShell>
    );
  }

  return (
    <BrowsePageShell>
      <BrowsePageHeader
        libraryName={library.name ?? librarySlug}
        crumbs={crumbs}
        librarySlug={librarySlug}
        roots={roots}
        layout={layout}
        onLayoutChange={handleLayoutChange}
        gridEnabled={catalogCapable}
      />

      <section className="flex flex-col gap-3">
        {layout === "list" ? (
          <BrowseItemList items={children} onOpenFolder={openFolder} returnTo={returnTo} />
        ) : (
          <p className="text-muted-foreground">{t("browse.layoutGridDisabled")}</p>
        )}
        <ListPager page={page} total={total} pageSize={MEDIA_PAGE_SIZE} onPageChange={setPage} />
      </section>
    </BrowsePageShell>
  );
}
