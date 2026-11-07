---
hide:
  - path
  - navigation
---

# Features

## Skip intro

Offline intro detection - no ML. One **Skip intro** button in the player when a segment is known.

| Layer | What happens |
|-------|----------------|
| **Chapters** | Intro-like chapter titles → Skip button |
| **Audio fingerprint** | Job `playback.skip.detect` does magic and provides you the same Skip button |
| **Precedence** | Chapter hit wins over audio |

---

## Player

Latest Video.js plus sudoStream chrome *inside* the player.

| Capability | Notes |
|------------|--------|
| **Season / Episode menus** | Choose season & episode without leaving the player |
| **Next episode** | Appears at late-timeline + end-of-file dialog |
| **Chapters** | Timeline markers when the file has chapter atoms |
| **Subtitles** | Provider VTT + per-user upload; size / color / outline / background knobs |
| **Quality** | Instant-VOD HLS; Auto prefers original / high rung; manual pick stays pinned |
| **Resume** | Continue from per-user watch progress |
| **Chromecast** | Web sender (`?access_token=` for the receiver) |
| **Picture-in-picture** | One-click PiP |

??? tip "Stuck / low network speed"
    Pin a lower quality rung. The player prefers freezes over automatic quality changes. Auto BBR should follow connection speed.

---

## Playback

Shared decide path for web and progressive DLNA: **Direct Play → remux → HLS encode**.

| Mode | Behavior |
|------|----------|
| **Direct Play** | Original file over HTTP Range — no ffmpeg |
| **Remux / HLS** | Instant-VOD playlist; remux when possible, else encode ladder |
| **HW-aceleration / tone map / downmix** | Admin settings|

---

## Libraries & ACL

Mapped folders become permissioned collections; the on-disk tree stays visible in browse.

| Concept | Behavior |
|---------|----------|
| **Library** | Name + type + one or more exclusive roots |
| **Root** | Path relative to `/media`; exclusive — no overlapping ownership |
| **Unassigned** | On disk but hidden from browse/search until attached |

Non-admins get per-library **create / read / update / delete** access. Admins bypass. Soft-delete goes to `/media/.trash` with path preserved.

### Library Types

| Type | Notes |
|------|-------|
| `film` / `series` | Browse and tiles views; fully featured & implemented |
| `music` / `photos` / `other` | Browse only, no features shipped |

---

## Providers

Optional enrichment only. Assign per library; run on cron or **Run now**.

| Provider | Metadata | Poster | Subs | Per-ep titles |
|----------|:--------:|:------:|:----:|:-------------:|
| TMDB | ✓ | ✓ | | ✓ |
| TVDB | ✓ | ✓ | | ✓ |
| TVmaze | ✓ | ✓ | | ✓ |
| AniList | ✓ | ✓ | | |
| AniDB | ✓ | ✓ | | ✓ |
| Fanart | | ✓ | | |
| OpenSubtitles | | | ✓ |  |

- Set **priorities across metadata sources** — original file tags, manual edits, and automated provider writes — including whether automation only fills gaps or rewrites fields
- Optional save-tags-to-file needs a writable media mount
- Provider egress proxy

---

## Maintenance / Scheduled jobs

!!! tip "First boot"
    After attaching roots, run a one-shot **metadata scan** — [First steps](getting-started/first-steps.md). Everything else is optional / recurring.
    
!!! tip "Run now or schedule later"
    Run now option is available for all jobs. While useful for an immediate run, use cron to schedule consequitive
    runs to be always up-to-date with the contents of your libraries.

| Scope | UI |
|-------|-----|
| Global | **Admin → Schedule** |
| Per library | **Library -> detail** under Admin sidebar section |

Each card: **enabled**, **cron** (required when enabled), **Save**, **Run now**, **Cancel**.

??? note "Cron when enabled"
    Empty cron is rejected when enabled. Standard 5-field cron (e.g. `0 3 * * *`).

### Global

| Action ID | Name | What it does |
|-----------|------|----------------|
| `libraries.scan` | Libraries scan | Reconcile roots / inventory vs disk |
| `playback.cache.purge` | Playback cache purge | Clears HLS and related cache; UI shows cache size |

Recycle-bin retention managed at **Admin → Settings**; expired items (now>=deleted at+retention) purge at **midnight UTC** when enabled.

### Per-library

| Action ID | Name | What it does |
|-----------|------|----------------|
| `metadata.scan` | Metadata scan | ffprobe / index into SQL (needed before catalog is useful) |
| `thumbnails.warm` | Thumbnails warm | Pre-generate thumbnails |
| `providers.metadata` | Providers → metadata | Enrich into DB/file per library settings |
| `providers.posters` | Providers → posters | Fetch / cache poster art |
| `providers.subtitles` | Providers → subtitles | Download subs (e.g. OpenSubtitles) for the player |
| `playback.skip.detect` | Skip-intro detect | Chromaprint intro pass (series); chapters still win — [Skip intro](#skip-intro) |

#### Metadata job options

| Apply mode | Behavior |
|------------|----------|
| `fill_missing` | Only write empty fields |
| `full_rewrite` | Overwrite existing fields per job settings |

Runs: **Admin → Schedule → History**. Cancel → cooperative stop → `canceled` when supported. Metrics: [Metrics](admin/metrics.md).

---

## DLNA / UPnP

Optional, off by default.

1. Create a user with role **TV** and grant `read` on libraries to publish
2. **Admin → Settings → DLNA** — enable and select that TV user
3. Publish HTTP port **8200** (and SSDP if you need discovery)

| Variable | Role |
|----------|------|
| `SUDOSTREAM_DLNA_HTTP_PORT` | Default `8200` |
| `SUDOSTREAM_DLNA_HTTP_BIND` | Empty = all interfaces |
| `SUDOSTREAM_DLNA_SSDP_MULTICAST` | Default `239.255.255.250:1900` |
| `SUDOSTREAM_DLNA_ADVERTISE_HOST` | Force advertised host when auto-detect is wrong |

!!! danger
    Do not expose DLNA / SSDP to the WAN. Switch to host networking if discovery is problematic.

??? warning "Discovery flaky on bridge networks?"
    Uncomment `network_mode: host` for the app service (and fix DB URL / remove conflicting `ports:`). Many TVs never see SSDP across Docker user-defined bridges.

ContentDirectory lists catalog + files the TV user can read. Streams are signed progressive URLs using the same [playback](#playback) decisions where applicable.

!!! note "Early stage"
    Generic device profile. Odd firmwares may still need a browser or Cast.

!!! note "TV role"
    Can hold library grants; **cannot** sign in to the web UI.

---

## Glossary

| Term | Meaning |
|------|---------|
| **Library** | Named collection with one or more **exclusive folder roots** |
| **Root** | Folder path attached to a library. Unassigned folders stay on disk but are **hidden** until attached |
| **ACL / grant** | Per-user, per-library `create` / `read` / `update` / `delete`. Admins bypass |
| **TV role** | DLNA principal; can hold grants; **cannot** web-login |
| **Browse** | Explorer/Finder-style view of the real directory tree |
| **Catalog** | Tiles from indexed metadata (empty until scan). Not a replacement for browse |
| **Override** | DB field merged over original tags for **effective** metadata |
| **Save to file** | Writes tags back via ffmpeg remux — needs writable media mount |
| **Provider** | Optional enricher (TMDB, …); per-library metadata / poster / subtitle slots |
| **Apply mode** | Wether a provider only `fill_missing` tags or `full_rewrite` |
| **Direct Play** | Stream original file if codecs are supported by the client; no transcode |
| **Remux** | Repackage without re-encoding video |
| **HLS / transcode** | Adaptive ladder when Direct Play / remux is not supported or a presice quiality chosen |
| **Device profile** | Capability hints for the playback planner |
| **Hardware acceleration** | `qsv` / `vaapi` / `nvenc` / `rockchip` (else `libx264`) |
| **Tone mapping** | HDR/wide → SDR for the 8-bit HLS ladder |
| **Downmix** | Surround → stereo on remux/encode AAC only |
| **Continue / watch state** | Per-user per-file playback position|
| **Trash** | Soft-delete under `/media/.trash` with path preserved |
| **Scheduled jobs** | Scheduled or Run now jobs|
