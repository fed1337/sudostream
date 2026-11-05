# First library

## 1. Sign in as admin

Open the UI (`SUDOSTREAM_BASE_URL`, usually `http://localhost:8080`). Change the bootstrap password immediately.

## 2. Create a library

**Admin → Libraries → Create**

| Field    | Guidance                                                                                          |
|----------|---------------------------------------------------------------------------------------------------|
| **Name** | Shown in the UI only (e.g. `Movies`)                                                              |
| **Type** | `film` or `series` enable pretty tile-view with a browse fallback; files only provide browse view |

!!! note "Exclusive roots"
    Each folder under `/media` belongs to **at most one** library (longest-prefix match)

## 3. Attach folder roots

Open the library → **Folders** → add paths from the folder picker (relative to `/media`).

??? example "Example layout"
    ```text
    /media
      Movies/          → library "Movies" (film)
      Shows/           → library "Shows" (series)
      Shows/Anime/     → could be a separate series library instead
      Incoming/        → unassigned = hidden from browse until attached
      Mix/             → library "I'll sort it someday" (files) = no pretty series grouping etc
    ```

Unassigned folders stay on disk but **do not** appear in browse or search.

## 4. Grant users (optional)

**Admin → Users → (user) → Libraries**

Non-admins need explicit `read` (and optionally `create` / `update` / `delete`). Admins bypass ACL.

!!! tip "TV devices"
    For DLNA, create a user with role **TV**, grant `read` on the libraries the TV should see,
    then pick that user under **Admin → Settings → DLNA**. TV accounts cannot sign in to the web UI.

## 5. Run a metadata scan

On the library detail page, run **metadata scan** once (`metadata.scan` — ffprobe index into SQL).

Until that finishes, catalog shelves may look empty even though **browse** still shows files.

!!! tip "Other jobs later"
    Thumbnails, providers, trash purge, skip-intro detect, and global schedules are separate admin maintenance
    actions — run them from **Admin → Maintenance** when you need them.

## 6. Browse vs Tiles

| Surface    | What you see                                                       |
|------------|--------------------------------------------------------------------|
| **Browse** | Real directories and files                                         |
| **Tiles**  | Film/series views from the SQL index (titles, posters / thumbnails |
| **Search** | ACL-filtered; scene-release-friendly matching                      |


## Checklist

- [ ] Library created with the right type
- [ ] At least one root attached
- [ ] Non-admin grants set (if you have other users)
- [ ] Metadata scan completed once
- [ ] Media mount `:rw` if you need soft-delete or “save tags to file”
    * [ ] `/media/.trash` exist for soft-delete support
