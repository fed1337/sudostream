# Install

sudoStream runs as **Docker Compose**: app + Postgres 18.
There is no supported “bare metal” Go binary, but feel free to build from this repo and meet all necessary runtime
dependencies.

!!! tip "Prerequisites"
    - Docker Engine + Compose v2
    - Host folders for media, cache, and logs
    - Disk for HLS / thumbnail cache under `./cache` (grows with watching)

## 1. Clone and prepare dirs

```shell
git clone https://github.com/fed1337/sudostream.git
cd sudostream

mkdir -p cache logs
# `cache` and `logs` binds must be writable by runtime user`
# Defaults to uid/gid 65532
sudo chown 65532:65532 cache logs
```

Put library files under `./media` (or change the bind in [`compose.yml`](https://github.com/fed1337/sudostream/blob/master/compose.yml)).

## 2. Edit compose

Before the first start:

1. Set secrets — at least `SUDOSTREAM_JWT_SECRET` (≥32 bytes hex i.e. `openssl rand -hex 32`) and `SUDOSTREAM_TOTP_ENCRYPTION_KEY` (base64 32-byte AES key i.e. `openssl rand -base64 32`.
2. Change `SUDOSTREAM_ADMIN_EMAIL` and `SUDOSTREAM_ADMIN_PASSWORD`
3. Set `SUDOSTREAM_BASE_URL` to the URL browsers will use (important for invites and cookies behind a proxy).
4. Add media volumes. They must be at least readable.
5. Decide media mount mode:

    === "Read-only (default)"
        `:ro` — browse / play / metadata DB override work. Prefer this for a watch-only host. Soft-delete and “save tags to file” **will not work** on a read-only libraries.

    === "Read-write"
        `:rw` — required for soft-delete (trash) and for **save tags to file** (ffmpeg remux into the original).

## 3. Start

```shell
docker compose -f compose.yml up -d
```

Open `SUDOSTREAM_BASE_URL`, login with credentials set and change the password.

## Hardware acceleration (optional)

Base `compose.yml` does **not** pass GPU devices. Add **one** vendor overlay, then set **Admin → Settings → Transcoding → hwAccel**.

```shell
# Intel Quick Sync
RENDER_GID=$(getent group render | cut -d: -f3) \
  docker compose -f compose.yml -f compose.qsv.yml up -d

# Intel / AMD VA-API
RENDER_GID=$(getent group render | cut -d: -f3) \
  docker compose -f compose.yml -f compose.vaapi.yml up -d

# NVIDIA (Container Toolkit required on host)
docker compose -f compose.yml -f compose.nvidia.yml up -d

# Rockchip RKMPP (arm64 BSP)
docker compose -f compose.yml -f compose.rockchip.yml up -d
```

## Trash

UI delete **moves** (rename) items to `/media/.trash/<id>/<original-relative-path>` — path preserved for restore.
Restore is the same rename in reverse.

That means:

1. Volumes that you delete from must be writable
2. Something bindin to `/media/.trash` bind must be present and writable
3. Source and `/media/.trash` must be the same filesystem
4. `.trash` is reserved — hidden from browse; do not attach it as a library root

### One host folder (simplest)

```yaml
volumes:
  - /srv/sudostream/media:/media:rw
```

Host layout example: `/srv/sudostream/media/{Movies,Shows,.trash}`.

### Several host paths under `/media`

Common when libraries live in different places (`/torrents`, photos, NAS share, …):

```yaml
volumes:
  - /torrents:/media/Torrents:rw
  - /home/user/photos:/media/Photos:rw
  - /srv/sudostream/trash:/media/.trash:rw
```

## Image tags

| Tag                                            | Typical use                        |
|------------------------------------------------|------------------------------------|
| `ghcr.io/fed1337/sudostream:latest` / `vX.Y.Z` | Release (`compose.yml`)            |
| `ghcr.io/fed1337/sudostream:dev`               | Development branch / `make dev-up` |

Runtime is **distroless** — no shell in release versions. Prefer logs bind + `docker compose logs` for debugging.
