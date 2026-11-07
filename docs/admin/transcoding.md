# Transcoding settings

**Admin → Settings → Transcoding** controls how remux / HLS encode behave. **Direct Play** streams the original file and does not apply tone-map or stereo downmix.

Base [`compose.yml`](https://github.com/fed1337/sudostream/blob/master/compose.yml) does **not** mount GPUs. Add **one** vendor overlay.

## Settings

| Setting | Description |
|---------|-------------|
| **Hardware acceleration** | `off` / `qsv` / `vaapi` / `nvenc` / `rockchip` — falls back to `libx264` if HW init fails |
| **Tone mapping enabled** | HDR/wide → SDR for the 8-bit HLS ladder when the source needs it |
| **Tone mapping algorithm** | `bt2390` (default) / `hable` / `reinhard` / `mobius` |
| **Downmix algorithm** | Surround → stereo for remux/encode AAC (`ac4`, `none`, `dave750`, `nightmodeDialogue`, `rfc7845`) |
| **Downmix boost** | Optional volume factor after pan (empty = algorithm default) |

!!! warning "Tonemap off + HDR"
    With tonemap disabled, the pipeline may only scale/format — colors can look wrong. That is expected.

!!! note
    Direct Play leaves multi-channel audio alone when the client accepts it.

## Image capabilities

| Arch | Bundled support |
|------|-----------------|
| **amd64** | jellyfin-ffmpeg + Intel/AMD VA user-space (OneVPL for QSV) + Vulkan/OpenCL loaders |
| **arm64** | jellyfin-ffmpeg with RKMPP; OpenCL ICD loader (Mali OpenCL from host when `/dev/mali0` is passed) |

## Overlays

=== "QSV (Intel)"

    ```shell
    RENDER_GID=$(getent group render | cut -d: -f3) \
      docker compose -f compose.yml -f compose.qsv.yml up -d
    ```

    - Devices: `/dev/dri`
    - Admin: `hwAccel=qsv`
    - Default `LIBVA_DRIVER_NAME=iHD`

=== "VA-API (Intel fallback or AMD)"

    ```shell
    RENDER_GID=$(getent group render | cut -d: -f3) \
      docker compose -f compose.yml -f compose.vaapi.yml up -d
    ```

    - Intel often `iHD`; AMD `radeonsi`
    - Admin: `hwAccel=vaapi`

=== "NVIDIA"

    ```shell
    docker compose -f compose.yml -f compose.nvidia.yml up -d
    ```

    - Needs NVIDIA Container Toolkit on host
    - Admin: `hwAccel=nvenc`
    - Do **not** combine with DRI overlays

=== "Rockchip"

    ```shell
    docker compose -f compose.yml -f compose.rockchip.yml up -d
    ```

    - Devices such as `dri`, `dma_heap`, `mali0`, `rga`, `mpp_service`
    - Admin: `hwAccel=rockchip`

!!! warning
    Pick **one** vendor overlay

## Tone map paths

When tonemap is enabled and the source needs it, ffmpeg selects a path by encoder (CUDA / Vulkan / OpenCL / CPU `tonemapx`). QSV uses software `tonemapx` for algorithm fidelity.

## Cache

HLS packages live under `/var/lib/sudostream/cache`. Size shows on the schedule UI near cache purge. Clear with global job `playback.cache.purge` — [Cron jobs](cron-jobs.md).
