import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router";
import { LibraryIcon, PlusIcon } from "lucide-react";

import type { Library, LibraryType } from "@/api/media";
import { libraryRoots } from "@/api/media";
import { getApiAdminLibrariesOptions } from "@/client/@tanstack/react-query.gen";
import { postApiAdminLibraries, postApiAdminLibrariesByIdRoots } from "@/client/sdk.gen";
import { FolderTreePicker } from "@/components/folder-tree-picker";
import { LibraryTypeBadge } from "@/components/library-type-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { extractApiErrorMessage } from "@/lib/api-error";
import { LIBRARY_TYPES } from "@/lib/library-types";

export default function AdminLibrariesPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [type, setType] = useState<LibraryType>("other");
  const [selectedRoots, setSelectedRoots] = useState<string[]>([]);

  const librariesQuery = useQuery({
    ...getApiAdminLibrariesOptions(),
  });

  const createMutation = useMutation({
    mutationFn: async () => {
      const response = await postApiAdminLibraries({
        body: { name: name.trim(), type },
      });
      if (response.error || !response.data?.library?.id) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.createLibraryFailed"));
      }
      const library = response.data.library;
      const libraryId = library.id;
      if (!libraryId) {
        throw new Error(t("admin.createLibraryFailed"));
      }
      for (const relPath of selectedRoots) {
        const rootResponse = await postApiAdminLibrariesByIdRoots({
          path: { id: libraryId },
          body: { relPath },
        });
        if (rootResponse.error) {
          throw new Error(extractApiErrorMessage(rootResponse.error) ?? t("admin.addFolderFailed"));
        }
      }
      return library;
    },
    onSuccess: (library) => {
      setCreateOpen(false);
      setName("");
      setType("other");
      setSelectedRoots([]);
      void librariesQuery.refetch();
      if (library.id) {
        void navigate(`/admin/libraries/${library.id}`);
      }
    },
  });

  if (librariesQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  const libraries = librariesQuery.data?.libraries ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t("admin.librariesTitle")}</h1>
          <p className="text-muted-foreground">{t("admin.librariesDescription")}</p>
        </div>
        <Button
          onClick={() => {
            setCreateOpen(true);
          }}
        >
          <PlusIcon data-icon="inline-start" />
          {t("admin.createLibrary")}
        </Button>
      </div>

      {libraries.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <LibraryIcon />
            </EmptyMedia>
            <EmptyTitle>{t("admin.emptyTitle")}</EmptyTitle>
            <EmptyDescription>{t("admin.emptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.name")}</TableHead>
              <TableHead>{t("admin.folders")}</TableHead>
              <TableHead>{t("admin.type")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {libraries.map((library) => (
              <AdminLibraryRow key={library.id ?? library.slug} library={library} />
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setSelectedRoots([]);
            createMutation.reset();
          }
        }}
      >
        <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("admin.createLibrary")}</DialogTitle>
            <DialogDescription>{t("admin.createLibraryDescription")}</DialogDescription>
          </DialogHeader>
          <FieldGroup className="min-h-0 overflow-auto">
            <Field>
              <FieldLabel>{t("admin.name")}</FieldLabel>
              <Input
                value={name}
                onChange={(event) => {
                  setName(event.target.value);
                }}
              />
            </Field>
            <Field>
              <FieldLabel>{t("admin.type")}</FieldLabel>
              <Select
                value={type}
                onValueChange={(value) => {
                  setType(value as LibraryType);
                }}
              >
                <SelectTrigger className="w-48">
                  <SelectValue>
                    <LibraryTypeBadge type={type} />
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {LIBRARY_TYPES.map((libraryType) => (
                      <SelectItem key={libraryType} value={libraryType}>
                        <LibraryTypeBadge type={libraryType} />
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <FieldSet>
              <FieldLegend variant="label">{t("admin.folders")}</FieldLegend>
              <FieldDescription>{t("admin.folderTreeHelp")}</FieldDescription>
              {createOpen ? (
                <FolderTreePicker selected={selectedRoots} onSelectedChange={setSelectedRoots} />
              ) : null}
              {selectedRoots.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {selectedRoots.map((root) => (
                    <Badge key={root} variant="secondary">
                      {root}
                    </Badge>
                  ))}
                </div>
              ) : null}
            </FieldSet>
          </FieldGroup>
          {createMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("admin.createLibraryFailed")}</AlertTitle>
              <AlertDescription>
                {createMutation.error instanceof Error
                  ? createMutation.error.message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              disabled={createMutation.isPending || !name.trim()}
              onClick={() => {
                createMutation.mutate();
              }}
            >
              {createMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              {t("admin.createLibrary")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function AdminLibraryRow({ library }: { library: Library }) {
  const { t } = useTranslation();
  const label = library.name ?? library.slug ?? library.relPath ?? "";
  const type = (library.type ?? "other") as LibraryType;
  const folders = libraryRoots(library);

  return (
    <TableRow>
      <TableCell className="font-medium">
        {library.id ? (
          <Link
            to={`/admin/libraries/${library.id}`}
            className="underline-offset-4 hover:underline"
          >
            {label}
          </Link>
        ) : (
          label
        )}
      </TableCell>
      <TableCell>
        {folders.length === 0 ? (
          <span className="text-muted-foreground">{t("admin.noFolders")}</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {folders.map((root) => (
              <Badge key={root} variant="outline">
                {root}
              </Badge>
            ))}
          </div>
        )}
      </TableCell>
      <TableCell>
        <LibraryTypeBadge type={type} />
      </TableCell>
    </TableRow>
  );
}
