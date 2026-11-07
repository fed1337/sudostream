---
hide:
  - toc
  - path
  - navigation
---

# :lucide-square-play: sudoStream

A lightweight video server that streams, remuxes and transcodes, while keeping your media library browsable - never miss your videos again.

Browse like Explorer/Finder or switch to tiles view, play in the browser, skip intros and enrich metadata only when you ask or manually.

sudoStream's goal is not to rewrite Plex/Jellyfin. It offers a different philosophy and easy access to:

- Broken matadata files
- Family videos (which often does not have useful metadata and stay hidden)

Uses [Jellyfin-ffmpeg](https://github.com/jellyfin/jellyfin-ffmpeg) under the hood.

## Key features

|                |                                                                                                  |
|----------------|--------------------------------------------------------------------------------------------------|
| **Browse**     | Explorer/Finder-like or tiles; Fast, powerful search                                             |
| **Playback**   | Direct Play → remux → HLS (QSV / VA-API / NVENC / Rockchip / software)                           |
| **Skip intro** | Finds intros and shows a skip button in the player, no chapters needed                           |
| **Player**     | Better UX with season & episode picker right in the player; quality, audio and subtitle settings |
| **Metadata**   | Manually or automated, off by default, ability to override only in DB or save to file            |
| **Ops**        | Metrics, logs, cron job management, proxy for automation providers                               |
| **ACL**        | Per-library-per-user CRUD grants                                                                 |

## Does it suit me?

!!! tip "Use it if"
    - Your library already lives in categorized folders
    - You need multiple users with fine grained access to libraries
    - You prefer modern WebUI with PiP & Chromecast over native apps
    - You tend to low resources overhead

!!! warning "Skip it if"
    - You expect AIO photos/music/books/file management app
    - You prefer Plex-style automation
    - You need “every TV just works” DLNA

## Getting started
<div class="grid cards" markdown>

-   :material-docker: __Install__

    ---

    Deploy using Docker compose

    [:octicons-arrow-right-24: Install](getting-started/install.md)

-   :material-cog: __Configure__

    ---

    Configure envrionment variables, volumes and run for the first time

    [:octicons-arrow-right-24: Configuration](getting-started/configuration.md)

-   :material-folder-plus: __First steps__

    ---

    First login, quick overview and create your first library

    [:octicons-arrow-right-24: First steps](getting-started/first-steps.md)

</div>

---

MIT — see the [license](https://github.com/fed1337/sudostream/blob/master/license).
The image redistributes Jellyfin FFmpeg (GPLv3).
