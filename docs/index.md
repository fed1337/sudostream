# sudoStream

A lightweight video server that is **not keen on automation**.

Play your videos instantly — folder layout stays exactly how it is on disk.
Explorer/Finder-style browse, modern web player, robust search, optional metadata providers that never block playback.

Libraries are explicit folder roots + per-user ACL. TMDB, TVDB, AniList, and friends run in the background
only when you want them — zero requirement to browse or hit play.

---

## What you get

| Area            | Included                                                                                                                       |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------|
| **Browse**      | List / Tiles views. Folder tree preserved; film/series catalog from indexed metadata; global search                            |
| **Access**      | Local users, invites, 2FA, sessions; per-library create / read / update / delete                                               |
| **Playback**    | Direct Play → remux → HLS transcode (hardware acceleration QSV / VA-API / NVENC / Rockchip); tone map & downmix admin settings |
| **Player**      | Continue watching, chapters, season/episode menus, next episode, user + provider subs, Chromecast (web sender)                 |
| **Metadata**    | Edit manually, save overrides in DB or write to the original file or schedule metadata providers                               |
| **Living room** | Optional **LAN-only** DLNA/UPnP Media Server (opt-in; dedicated port; TV identity with library grants, no web login)           |
| **Library ops** | Soft-delete recycle bin; admin restore / purge; maintenance cron (metadata scan, providers, thumbs, …)                         |
| **Ops**         | Docker Compose + Postgres 18, distroless image, Prometheus `/metrics`,                                                         |

---

## Fit / skip

**Use it if**

- Your library already lives in folders and you want to keep it that way
- You need multi-user ACL without handing everyone the admin password
- You need first-class browser support + Chromecast (DLNA is very early stage)
- You run Docker and would rather not babysit a full media hub

**Skip if**

- You want AIO app with photo library, a music player, or in-app file management
- You need native mobile/TV apps or not ready for weak DLNA support
- You expect Plex-style “identify everything and fix my filenames”

---

## Features at a glance

### Manual control over automation

You decide identity and layout. Providers (TMDB, TVDB, AniList, TVmaze, AniDB, Fanart, OpenSubtitles) are **optional
enrichment** — cron or “Run now” in admin. Discovery, search and playback are never blocked on external APIs or
incorrent metadata.

- **ffprobe index** on scan; effective metadata = original tags + overrides
- **Manual editor** — PATCH overrides or save tags back to the file (ffmpeg remux)
- **Provenance** — see whether a field came from you or a provider

### Streaming & transcoding

Powered by [Jellyfin FFmpeg](https://github.com/jellyfin/jellyfin-ffmpeg). **Direct Play** when the device profile
allows it; otherwise instant-VOD HLS with remux or encode.

- **Hardware acceleration** — **AMD** (VA-API), **Intel** (QSV / VA-API), **NVIDIA** (NVENC), **Rockchip** (RKMPP on
  arm64)
- **Tone mapping** — Admin → Settings → Transcoding; Available algorithms: `bt2390` / `hable` / `reinhard` / `mobius`.
  When a source needs tone mapping (PQ/HLG / wide gamut / 10-bit into the 8-bit SDR H.264 ladder), remux stays disabled;
  with tonemap on, ffmpeg applies a path selected by the active encoder (CUDA / Vulkan on AMD VA-API / OpenCL on
  Rockchip / CPU `tonemapx`+algo; QSV uses software `tonemapx` for algo fidelity). Off skips the TMO (scale/format
  only); colors may look wrong — that is expected.
- **Stereo downmix** — dialogue-aware AC-4 pan for surround (`ac4` dialogue-aware pan for surround) or ffmpeg matrix
  only (`none`, defaults to `-ac 2`); Stereo/mono sources skip pan.
- Thumbnails, provider posters, subtitles auto-attached in the player

### Explorer / Finder-like view

Your directory layout stays visible. Switch between **list** and **tiles**. Unassigned folders on disk stay hidden until
you attach them to a library. Soft-delete moves items to a recycle bin under `/media/.trash` with path preserved.

### Footprint & attack surface

Runs via **Docker Compose** (Postgres + app). Distroless runtime in production; busybox shell in the dev image only.

### Admin

Users, invites, sessions, 2FA reset, per-user library grants (including a dedicated **TV** role for DLNA devices).
Library roots, provider slots, transcode settings, DLNA enable, maintenance scheduler, folder tree picker,
trash restore / purge, and optional provider network proxy (HTTP/SOCKS).

### Home & personal shelves

Continue watching, favorites, watched / unwatched carousels, and stats — resume picks up from saved progress in the
player.

---

## Docker runtime

| Environment                      | Distroless tag  | Shell                 |
|----------------------------------|-----------------|-----------------------|
| Prod / GHCR `latest`             | `nonroot`       | No                    |
| Dev (`make dev-up` / GHCR `dev`) | `debug-nonroot` | BusyBox `/busybox/sh` |

The **amd64** image bundles jellyfin-ffmpeg plus Intel/AMD VA user-space libraries (including OneVPL for QSV) and
Vulkan/OpenCL loaders. **arm64** ships jellyfin-ffmpeg with embedded RKMPP support plus an OpenCL ICD loader (Mali
OpenCL comes from the host when `/dev/mali0` is passed).

**Hardware encoder** is chosen in **Admin → Settings → Transcoding** (`off` / `qsv` / `vaapi` / `nvenc` / `rockchip`).
Falls back to software (`libx264`).

| Arch  | Admin `hwAccel`                | Compose overlay                                                                                                                                                                                                                                                                     |
|-------|--------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| amd64 | `off`, `qsv`, `vaapi`, `nvenc` | [`compose.qsv.yml`](https://github.com/fed1337/sudostream/blob/master/compose.qsv.yml), [`compose.vaapi.yml`](https://github.com/fed1337/sudostream/blob/master/compose.vaapi.yml), or [`compose.nvidia.yml`](https://github.com/fed1337/sudostream/blob/master/compose.nvidia.yml) |
| arm64 | `off`, `rockchip`              | [`compose.rockchip.yml`](https://github.com/fed1337/sudostream/blob/master/compose.rockchip.yml)                                                                                                                                                                                    |

Base [`compose.yml`](https://github.com/fed1337/sudostream/blob/master/compose.yml) does **not** mount GPU devices.
Pick **one** vendor overlay:

| Vendor       | Host setup                                                                                                               |
|--------------|--------------------------------------------------------------------------------------------------------------------------|
| **QSV**      | `/dev/dri` + `RENDER_GID`; `LIBVA_DRIVER_NAME=iHD` (default in overlay)                                                  |
| **VA-API**   | Same device passthrough; Intel often uses `iHD`, AMD `radeonsi`                                                          |
| **NVIDIA**   | `deploy.resources.reservations.devices` (nvidia, `count: all`) + driver capability env; do not combine with DRI overlays |
| **Rockchip** | `dri`, `dma_heap`, `mali0`, `rga`, `mpp_service`                                                                         |

---

## Providers

| Provider      | Metadata | Poster | Subtitles | Per-episode titles |
|---------------|:--------:|:------:|:---------:|:------------------:|
| TMDB          |    ✓    |   ✓   |           |         ✓         |
| TVDB          |    ✓    |   ✓   |           |         ✓         |
| TVmaze        |    ✓    |   ✓   |           |         ✓         |
| AniList       |    ✓    |   ✓   |           |                    |
| AniDB         |    ✓    |   ✓   |           |         ✓         |
| Fanart        |          |   ✓   |           |                    |
| OpenSubtitles |          |        |    ✓     |                    |

Apply modes: `fill_missing` or `full_rewrite`. Credentials and env placeholders are documented in
[`compose.yml`](https://github.com/fed1337/sudostream/blob/master/compose.yml).


---

## Get started

- Follow [Quickstart](https://github.com/fed1337/sudostream#quickstart)
- Open the web UI, login as admin, set up new password, create libraries and scan their metadata

---

## License & disclaimer

MIT — see [`license`](https://github.com/fed1337/sudostream/blob/master/license). The Docker image redistributes
Jellyfin FFmpeg (GPLv3). Go and TypeScript sources in this repository were largely AI-assisted; treat as early-stage
software and report issues on GitHub.
