# Features

## Skip intro

Offline engine for intro detection, no  ML, skip intro with just 1 button in the player

| Layer | What happens |
|-------|----------------|
| **Chapters** | Intro-like chapter titles (`intro`, `opening`, `theme`, …) → **Skip intro** in the player |
| **Audio fingerprint** | Job `playback.skip.detect` runs **chromaprint** offline across a season, finds the shared theme window, stores per-episode ranges |
| **Precedence** | Chapter hit **wins** over audio when both exist |

---

## Player

The latest Video.js + sudoStream chrome *inside* the player.

| Capability                 | Notes                                                                                 |
|----------------------------|---------------------------------------------------------------------------------------|
| **Season / Episode menus** | Catalog-fed; choose season & episode without leaving the player                       |
| **Next episode**           | Late-timeline control + end-of-file dialog                                            |
| **Chapters**               | TimeSlider markers for any file with chapter atoms                                    |
| **Subtitles**              | Provider VTT auto-attach + per-user upload; size / color / outline / background knobs |
| **Quality**                | Instant-VOD HLS; **Auto** prefers original / high rung; manual pick stays pinned      |
| **Resume**                 | Continue from per-user `watch_state`                                                  |
| **Chromecast**             | Chromecast-ready                                                                      |
| **Picture-in-picture**     | Just press 1 button                                                                   | 

??? tip "Stuck / low network speed"
    Choose a lower quality rung manually. The player prefers freezes over automatic quality changes. Auto BBR should follow connection speed.

---

## Playback

Shared decide path for the web player and progressive DLNA streams: **Direct Play** → **remux** → **HLS encode**.

```text
device profile + source codecs
        │
        ├─ eligible ──► Direct Play  ──► GET /api/stream (HTTP Range)
        │
        ├─ remux OK ──► HLS remux     ──► playlist + segments (no video re-encode)
        │
        └─ else    ──► HLS transcode ──► ladder (libx264 or HW encoder)
```

| Mode | Behavior |
|------|----------|
| **Direct Play** | Original file over HTTP Range; no ffmpeg job. Prefer for archival quality / low CPU |
| **Remux / HLS** | Instant-VOD: arithmetic playlist, lazy segments. Remux keeps original video when allowed; else encode ladder |
| **Quality Auto** | Prefers original / high rung when estimates allow; a pinned quality sticks for the session |

---

## Hardware acceleration

**Admin → Settings → Transcoding → hwAccel**:

| Value | Typical host |
|-------|----------------|
| `off` | Software `libx264` |
| `qsv` | Intel Quick Sync |
| `vaapi` | Intel / AMD VA-API |
| `nvenc` | NVIDIA |
| `rockchip` | RK3588-class (arm64) |

---

## Tone mapping

When the source needs HDR/wide → SDR for the 8-bit HLS ladder, remux is disabled and ffmpeg applies a tone-map path (admin: `bt2390`, `hable`, `reinhard`, `mobius`).

!!! warning
    With tonemap **off**, the pipeline may only scale/format — colors can look wrong. That is expected.

---

## Surround downmix

Admin downmix algorithms apply on **remux / encode / HLS AAC** only. **Direct Play** leaves multi-channel alone when the client accepts it. User **audio language preferences** (top-3) influence the default HLS/encode track.

---

## Libraries & ACL

Libraries turn mapped folders into permissioned collections. The on-disk tree stays visible in browse.

| Concept | Behavior |
|---------|----------|
| **Library** | Name + type (`film`, `series`, …) + one or more roots (folders) |
| **Root** | Path relative to `/media`; exclusive — no overlapping ownership |
| **Match** | Longest-prefix wins for a given file path |
| **Unassigned** | On disk; **hidden** from browse/search until attached |

Admins bypass ACL. Everyone else needs per-library grants (**Admin → Users → Libraries**):

### ACL

| Flag | Typical use |
|------|-------------|
| `read` | Browse, search, play, catalog |
| `create` | Add / write paths where the product allows writes |
| `update` | Metadata edits and similar updates |
| `delete` | Soft-delete to trash |

### Library types

| Type | Notes |
|------|-------|
| `film` / `series` | Browse and tiles views; fully featured & implemented |
| `music` | Browse OK; dedicated music app not shipped |
| `photos` | Not shipped |

### Soft-delete & trash

Delete moves items to `/media/.trash` with the original relative path preserved.

| Who | Action |
|-----|--------|
| User with `delete` | Soft-delete |
| Admin | Restore to original path or purge forever; optional retention under Settings |

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

Effective metadata = original file tags + DB overrides. Provenance shows `provider:<key>` vs `user:<id>`. Optional **save tags to file** (ffmpeg remux) needs a writable media mount.

### Proxy

**Admin → Settings → Network** — HTTP/SOCKS proxy for provider egress (+ probe; useful for AniList / region locks). 

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
