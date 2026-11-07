# Troubleshooting

## Permission / UID

!!! failure "cache or logs not writable"
    Container runs as **65532:65532** by default.

    ```shell
    sudo chown -R 65532:65532 cache logs
    ```

    Dev builds can override `APP_UID` / `APP_GID` via `compose.dev.yml`.

## Media mount

| Symptom | Check |
|---------|--------|
| Cannot “save tags to file” | `/media` is `:ro` — switch to `:rw` |
| Soft-delete fails | Trash needs write under `/media/.trash` (same filesystem as source) |
| Empty browse | No library roots / no `read` grant / path outside `/media` |

## Playback

??? question "Always transcoding / weird colors"
    - Confirm Admin **hwAccel** matches the compose overlay — [Transcoding](transcoding.md)
    - Tonemap **off** on HDR sources can look wrong — expected
    - Try Direct Play–capable browsers; pin a lower HLS rung

??? question "Cast dies after a while"
    Access token is snapshotted at cast start. Restart cast after refresh; see [Features — Chromecast](../features.md#player).

## DLNA

| Symptom | Likely fix |
|---------|------------|
| TV never sees the server | Host networking + SSDP; set `SUDOSTREAM_DLNA_ADVERTISE_HOST` |
| Empty library on TV | TV user lacks `read` grants; DLNA disabled in admin |
| Plays on web, not TV | Generic DLNA profile limits — early-stage |

## Providers

| Symptom | Check |
|---------|--------|
| Job errors / timeouts | Admin Network proxy; API keys in env |
| Fanart empty | Needs prior `tmdb_id` / `tvdb_id` from metadata provider |
| OpenSubtitles miss | Hash/size identity; username is **account name**, not email |
