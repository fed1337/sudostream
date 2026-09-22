import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2Icon } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { DeleteMediaError, deleteMedia } from "@/api/playback";
import type { MediaItem } from "@/api/media";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type DeleteMediaButtonProps = {
  item: MediaItem;
  onDeleted?: () => void;
  variant?: "ghost" | "outline" | "destructive";
  size?: "default" | "lg" | "icon" | "icon-lg";
  iconOnly?: boolean;
};

function deleteErrorMessage(error: unknown, t: (key: string) => string): string {
  if (error instanceof DeleteMediaError) {
    if (error.message && error.message !== "delete failed") {
      return error.message;
    }
    if (error.status === 403) {
      return t("media.deleteForbidden");
    }
    if (error.status === 409) {
      return t("media.deleteNotEmpty");
    }
    if (error.status === 404) {
      return t("media.deleteNotFound");
    }
  }

  return t("media.deleteFailed");
}

export function DeleteMediaButton({
  item,
  onDeleted,
  variant = "ghost",
  size = "lg",
  iconOnly = false,
}: DeleteMediaButtonProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const isDir = item.isDir === true;

  const mutation = useMutation({
    mutationFn: async () => {
      if (!item.path) {
        throw new Error("missing path");
      }

      await deleteMedia(item.path, isDir);
    },
    onSuccess: async () => {
      setOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["browse"] });
      await queryClient.invalidateQueries({ queryKey: ["metadata"] });
      onDeleted?.();
    },
  });

  if (!item.actions?.canDelete || !item.path) {
    return null;
  }

  const trigger = (
    <Button
      variant={iconOnly ? "ghost" : variant}
      size={iconOnly ? "icon" : size}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        mutation.reset();
        setOpen(true);
      }}
      aria-label={t("media.delete")}
    >
      <Trash2Icon />
      {!iconOnly && size !== "icon" && size !== "icon-lg" ? t("media.delete") : null}
    </Button>
  );

  return (
    <AlertDialog
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) {
          mutation.reset();
        }
      }}
    >
      {iconOnly ? (
        <Tooltip>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
          <TooltipContent sideOffset={6}>{t("media.delete")}</TooltipContent>
        </Tooltip>
      ) : (
        trigger
      )}

      <AlertDialogContent
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>
            {isDir ? t("media.deleteConfirmFolderTitle") : t("media.deleteConfirmFileTitle")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isDir ? t("media.deleteConfirmFolder") : t("media.deleteConfirmFile")}
            {item.name ? (
              <span className="mt-2 block font-medium text-foreground">{item.name}</span>
            ) : null}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {mutation.isError ? (
          <p className="text-sm text-destructive">{deleteErrorMessage(mutation.error, t)}</p>
        ) : null}

        <AlertDialogFooter>
          <Button
            variant="outline"
            size="default"
            disabled={mutation.isPending}
            onClick={() => setOpen(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            size="default"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t("media.delete")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
