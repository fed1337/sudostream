import { useTranslation } from "react-i18next";

import type { LibraryType } from "@/api/media";
import { Badge } from "@/components/ui/badge";
import { libraryTypeLabelKey } from "@/lib/library-types";

type LibraryTypeBadgeProps = {
  type: LibraryType | undefined;
};

export function LibraryTypeBadge({ type }: LibraryTypeBadgeProps) {
  const { t } = useTranslation();

  if (!type) {
    return null;
  }

  return <Badge variant="secondary">{t(libraryTypeLabelKey(type))}</Badge>;
}
