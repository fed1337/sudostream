const storagePrefix = "sudostream.sidebar.";

export type SidebarStorageId = "main" | "metadata";

export function getSidebarOpen(id: SidebarStorageId, fallback = true): boolean {
  if (typeof window === "undefined") {
    return fallback;
  }
  const raw = window.localStorage.getItem(storagePrefix + id);
  if (raw === "true") {
    return true;
  }
  if (raw === "false") {
    return false;
  }

  return fallback;
}

export function setSidebarOpen(id: SidebarStorageId, open: boolean): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(storagePrefix + id, String(open));
}
