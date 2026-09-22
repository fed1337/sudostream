import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRightIcon, FolderIcon } from "lucide-react";

import { useQuery } from "@tanstack/react-query";

import type { InternalHttpapiFolderTreeEntry } from "@/client/types.gen";
import { getApiAdminFolderTreeOptions } from "@/client/@tanstack/react-query.gen";
import { toggleExclusiveRoot } from "@/api/media";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Spinner } from "@/components/ui/spinner";

type FolderTreeEntry = InternalHttpapiFolderTreeEntry;

type FolderTreePickerProps = {
  currentLibraryId?: string;
  selected: string[];
  onSelectedChange: (paths: string[]) => void;
};

export function FolderTreePicker({
  currentLibraryId,
  selected,
  onSelectedChange,
}: FolderTreePickerProps) {
  const { t } = useTranslation();
  const rootQuery = useQuery({
    ...getApiAdminFolderTreeOptions({ query: {} }),
  });

  if (rootQuery.isLoading) {
    return <Spinner />;
  }

  const entries = rootQuery.data?.entries ?? [];

  return (
    <div className="bg-background h-72 overflow-auto rounded-lg border p-2">
      {entries.length === 0 ? (
        <p className="text-muted-foreground p-2 text-sm">{t("admin.folderTreeEmpty")}</p>
      ) : (
        <ul className="flex flex-col">
          {entries.map((entry) => (
            <FolderTreeItem
              key={entry.relPath}
              entry={entry}
              currentLibraryId={currentLibraryId}
              selected={selected}
              onSelectedChange={onSelectedChange}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

type FolderTreeItemProps = {
  entry: FolderTreeEntry;
  currentLibraryId?: string;
  selected: string[];
  onSelectedChange: (paths: string[]) => void;
};

function FolderTreeItem({
  entry,
  currentLibraryId,
  selected,
  onSelectedChange,
}: FolderTreeItemProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const relPath = entry.relPath ?? "";
  const assignedOther = Boolean(
    entry.assignedLibraryId && entry.assignedLibraryId !== currentLibraryId,
  );
  const assignedHere = Boolean(currentLibraryId && entry.assignedLibraryId === currentLibraryId);
  const isSelected = selected.includes(relPath);

  const childrenQuery = useQuery({
    ...getApiAdminFolderTreeOptions({ query: { path: relPath || undefined } }),
    enabled: open && Boolean(relPath),
  });

  return (
    <li>
      <Collapsible className="group/collapsible" open={open} onOpenChange={setOpen}>
        <div className="flex items-center gap-1">
          <CollapsibleTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("admin.expandFolder", { name: entry.name })}
            >
              <ChevronRightIcon className="transition-transform group-data-[state=open]/collapsible:rotate-90" />
            </Button>
          </CollapsibleTrigger>
          <Button
            type="button"
            variant={isSelected ? "secondary" : "ghost"}
            className="min-w-0 flex-1 justify-start"
            disabled={assignedOther || assignedHere || !relPath}
            aria-pressed={isSelected}
            onClick={() => {
              onSelectedChange(toggleExclusiveRoot(selected, relPath));
            }}
          >
            <FolderIcon data-icon="inline-start" />
            <span className="truncate">{entry.name}</span>
            {assignedHere ? (
              <Badge variant="secondary">{t("admin.folderAssignedHere")}</Badge>
            ) : assignedOther ? (
              <Badge variant="outline">{t("admin.folderAssignedOther")}</Badge>
            ) : null}
          </Button>
        </div>
        <CollapsibleContent>
          <div className="ml-5 border-l pl-2">
            {childrenQuery.isLoading ? (
              <Spinner />
            ) : (childrenQuery.data?.entries ?? []).length === 0 ? (
              <p className="text-muted-foreground px-2 py-1 text-sm">{t("admin.noSubfolders")}</p>
            ) : (
              <ul className="flex flex-col">
                {(childrenQuery.data?.entries ?? []).map((child) => (
                  <FolderTreeItem
                    key={child.relPath}
                    entry={child}
                    currentLibraryId={currentLibraryId}
                    selected={selected}
                    onSelectedChange={onSelectedChange}
                  />
                ))}
              </ul>
            )}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </li>
  );
}
