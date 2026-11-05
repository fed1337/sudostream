# First Steps

## 1. Login as admin

Open the UI at `SUDOSTREAM_BASE_URL`. Login with `SUDOSTREAM_ADMIN_EMAIL` and `SUDOSTREAM_ADMIN_PASSWORD`.
Change the bootstrap password immediately.

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

## 4. Create users (optional)

### Invite

1. Go to **Admin → Users** and click **Invite**.
2. Enter the email address, choose a role and click **Send**.
3. Open the user from the user list on the same page to grant access to libraries, activcate/deactivate, etc

### Create

You can also create a user by clicking **Create** and filling in their password. Useful if SMTP is unavailable.
The password must be changed on the first login.

### Grant access to libraries

**Admin → Users → (user) → Libraries**

Non-admins need explicit `read` (and optionally `create` / `update` / `delete`). Admins bypass ACL.

!!! tip "TV devices"
    For DLNA, create a user with role **TV**, grant `read` on the libraries the TV should see,
    then pick that user under **Admin → Settings → DLNA**. TV accounts cannot sign in to the web UI.

## 5. Run a metadata scan

Go to **Admin → Libraries → (your library)** to open the details page, find **scan metadata** and run it once.
It will index files into DB.
Once it's finished (**Admin -> Schedule tasks -> Execution history**) you may set up a periodic metadata scan.

Until that finishes, catalog shelves may look empty even though **browse** view still shows files.

!!! tip "Other jobs later"
    Once metadata is scanned you can setup warm thumbnails, skip-intro detect, automation and global jobs

!!! tip "Scheduling jobs"
    Run each job manually once to get the idea of how much time is needed for the next step
    (i.e. metadata must be present before running automations).
    Then setup a schedule. If you are unfamiliar with cron format use [crontab.guru](https://crontab.guru/)

## 6. Browse vs Tiles

| Surface    | What you see                                        |
|------------|-----------------------------------------------------|
| **Browse** | Real directories and files                          |
| **Tiles**  | Film/series views from the scanned metadata         |
| **Search** | ACL-filtered; scene-release-friendly title matching |

## Checklist

- [ ] Library created with the right type
- [ ] At least one root attached
- [ ] Non-admin grants set (if you have other users)
- [ ] Metadata scan completed once
- [ ] Media mount `:rw` if you need soft-delete or “save tags to file”
    * [ ] `/media/.trash` exist for soft-delete support
