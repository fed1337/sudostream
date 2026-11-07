# Scheduled maintenance jobs

Background work runs on a **cron** expression or **Run now**. **Cancel** is available while a run is active.

| Scope | UI |
|-------|-----|
| Global | **Admin → Schedule** |
| Per library | **Admin → Libraries → (library)** maintenance / provider cards |

!!! tip "First boot"
    After attaching roots, run a one-shot **metadata scan** — [First steps](../getting-started/first-steps.md). Then schedule what you want recurring.

??? note "Cron format"
    Standard 5-field cron (e.g. `0 3 * * *`). Empty cron is rejected when the schedule is enabled. Unfamiliar? Use [crontab.guru](https://crontab.guru/).

---

## Global jobs

| Action ID | Name | What it does |
|-----------|------|----------------|
| `libraries.scan` | Libraries scan | Reconcile roots / inventory vs disk |
| `playback.cache.purge` | Playback cache purge | Clears HLS (and related) cache; UI may show cache size |

---

## Recycle bin retention

Recycle-bin **retention** is not technically a cron job.

It lives at **Admin → Settings** and expired items `current time >= deletedAt + retention` purge at **midnight UTC** when enabled.

---

## Per-library jobs

Open **Admin → Libraries → (library)**. Every card shares the same schedule controls; provider cards add slot-specific settings.

### Shared schedule controls

| Control | Meaning |
|---------|---------|
| **Schedule enabled** | When on, cron must be non-empty; job fires on that schedule |
| **Cron** | Five-field expression in **server local time** (e.g. `0 3 * * *`). Empty = manual-only (**Run now**) |
| **Save** | Persist schedule (and provider settings on provider cards) |
| **Run now** | Start immediately (ignored while already running) |
| **Cancel** | Cooperative stop while a run is active |

!!! tip "Order"
    **Metadata scan** first → thumbs / providers / skip-intro. Run once manually to gauge duration, then schedule.

| Action ID | Card | Library types |
|-----------|------|----------------|
| `metadata.scan` | Scan metadata | All |
| `thumbnails.warm` | Warm thumbnails | All |
| `playback.skip.detect` | Detect skip intro | **series** only |
| `providers.metadata` | Metadata provider | **film** / **series** |
| `providers.posters` | Poster provider | **film** / **series** |
| `providers.subtitles` | Subtitles provider | **film** / **series** |

---

### `metadata.scan` — Scan metadata

Probes video files under the library roots with ffprobe and refreshes the SQL metadata index (titles, duration, codecs, identity hashes, …). Catalog **tiles** stay empty or stale until this has run successfully at least once; **browse** still shows files without it.

| Setting | Description |
|---------|-------------|
| Schedule enabled / Cron | Shared schedule controls only — no extra options |

**Run summary** includes `filesIndexed`. Re-run after adding files or when tags on disk changed.

---

### `thumbnails.warm` — Warm thumbnails

Generates missing **video poster** thumbnails into the thumbs cache for files in this library. Skips titles that already have a cached poster.

| Setting | Description |
|---------|-------------|
| Schedule enabled / Cron | Shared schedule controls only |

**Run summary** includes `generated`. Prefer after a metadata scan so the indexer knows which videos exist.

---

### `playback.skip.detect` — Detect skip intro

**Series libraries only.** Offline chromaprint pass: fingerprints early audio across episodes in a season, finds a shared opening theme, and stores intro ranges for the player **Skip intro** button.

| Setting | Description |
|---------|-------------|
| Schedule enabled / Cron | Shared schedule controls only |

Chapter-title intros still **win** at playback when present — this job only fills audio-based segments when chapters do not yield an intro. Prefer after metadata/catalog grouping is in place. See [Features — Skip intro](../features.md#skip-intro).

---

### `providers.metadata` — Metadata provider

Matches titles via the chosen enricher and writes metadata. API keys: [Configuration](../getting-started/configuration.md#provider-api-keys-optional).

Setup egress proxy if needed: **Admin → Settings → Network**.

| Setting | Description |
|---------|-------------|
| **Provider** | Slot enricher (`tmdb`, `tvdb`, `tvmaze`, `anilist`, `anidb`, …) or **None** (job no-ops) |
| **Metadata write target** | **Database** — DB overrides only (effective tags in UI). **File** — ffmpeg remux tags into the media file (needs writable `/media`) |
| **Metadata apply mode** | **Fill missing only** — write empty fields only. **Full rewrite** — overwrite existing provider/effective fields according to the next flag |
| **Allow overriding user-edited metadata** | **Off (default):** any file a user has ever edited (`user:…` provenance) is **skipped**. **On:** automation may overwrite those too |
| Schedule enabled / Cron | Shared schedule controls |

Per-file failures are counted in the run summary; provider-wide blocks (e.g. HTTP 403 / rate limit) fail the whole run.

---

### `providers.posters` — Poster provider

Fetches and caches provider artwork for the player / catalog. Fanart typically needs a prior `tmdb_id` / `tvdb_id` from a metadata run.

| Setting | Description |
|---------|-------------|
| **Provider** | Poster slot (`tmdb`, `tvdb`, `fanart`, …) or **None** |
| Schedule enabled / Cron | Shared schedule controls |

No write-target / apply-mode on this card — art goes to the provider poster cache.

---

### `providers.subtitles` — Subtitles provider

Downloads subtitles into cache for auto-attach in the player (e.g. OpenSubtitles via moviehash).

| Setting | Description |
|---------|-------------|
| **Provider** | Subtitle slot (e.g. `opensubtitles`) or **None** |
| **Subtitle languages** | One or more ISO 639-1 codes. **Required** when a provider is selected (save is rejected if empty). Cleared when provider is **None** |
| Schedule enabled / Cron | Shared schedule controls |

OpenSubtitles username must be the **account name**, not email — [Troubleshooting](troubleshooting.md#providers).

---


## Runs & cancel

- **Admin → Schedule → History** (and status on each card) lists recent runs
- **Cancel** requests cooperative stop → terminal status `canceled` when supported
- Prometheus: `maintenance_*` — [Metrics](metrics.md)
