# Configuration

---

## Ports & volumes

| Port         | Purpose                                                            |
|--------------|--------------------------------------------------------------------|
| **8080**     | Web UI + API                                                       |
| **8200**     | DLNA / UPnP HTTP — only useful when DLNA is setup                  |
| **1900/udp** | SSDP discovery (optional; often needs **host networking**)         |
| **5432**     | Postgres (sample compose publishes it — lock down on shared hosts) |

| Host (sample) | Container                   | Notes                         |
|---------------|-----------------------------|-------------------------------|
| `./media`     | `/media`                    | Library files; `:ro` or `:rw` |
| `./cache`     | `/var/lib/sudostream/cache` | HLS, thumbs, provider cache   |
| `./logs`      | `/var/lib/sudostream/logs`  | App logs                      |

!!! tip "Trash usage"
    Trash is fixed at **`/media/.trash`** (not configurable).
    Soft-delete **renames** into that tree — same filesystem as the source, and writable mounts.
    Multi-bind setups (`/torrents`, photos, … under `/media/…`) need care; see [Install → Trash](install.md#trash).

!!! tip "DLNA discovery"
    Prefer `network_mode: host` for the app when TVs must see SSDP
    (adjust `SUDOSTREAM_DATABASE_URL` & remove conflicting `ports:`).

---

## Environment variables

### Core & secrets

| Variable                         | Description                                                                    |
|----------------------------------|--------------------------------------------------------------------------------|
| `SUDOSTREAM_DATABASE_URL`        | Postgres DSN (sample points at the compose `postgres` service)                 |
| `SUDOSTREAM_ADMIN_EMAIL`         | Bootstrap admin email — change the sample                                      |
| `SUDOSTREAM_ADMIN_PASSWORD`      | Bootstrap admin password — change immediately after first login                |
| `SUDOSTREAM_JWT_SECRET`          | Signs access + refresh tokens (≥32 bytes hex)                                  |
| `SUDOSTREAM_TOTP_ENCRYPTION_KEY` | Encrypts stored TOTP secrets (base64 32-byte AES key)                          |
| `SUDOSTREAM_BASE_URL`            | Canonical public URL (invites, email links). Must match how users reach the UI |
| `SUDOSTREAM_COOKIE_SECURE`       | `false` only for plain HTTP on a trusted LAN                                   |

### Auth token & mail link TTLs

| Variable                              | Sample default | Description                  |
|---------------------------------------|----------------|------------------------------|
| `SUDOSTREAM_JWT_ACCESS_TTL`           | `15m`          | Bearer access token lifetime |
| `SUDOSTREAM_JWT_REFRESH_TTL`          | `168h`         | Refresh cookie lifetime      |
| `SUDOSTREAM_INVITE_TTL_HOURS`         | `24`           | Invite link validity         |
| `SUDOSTREAM_CONFIRM_EMAIL_TTL_HOURS`  | `24`           | Email confirm link           |
| `SUDOSTREAM_RESET_PASSWORD_TTL_HOURS` | `24`           | Password reset link          |
| `SUDOSTREAM_TOTP_LEEWAY_SECONDS`      | `30`           | TOTP clock-skew window       |

### Database pool

| Variable                           | Sample | Description                |
|------------------------------------|--------|----------------------------|
| `SUDOSTREAM_DB_MAX_CONNS`          | `10`   | Max open connections       |
| `SUDOSTREAM_DB_MIN_CONNS`          | `2`    | Min idle connections       |
| `SUDOSTREAM_DB_MAX_CONN_LIFETIME`  | `1h`   | Max connection age         |
| `SUDOSTREAM_DB_MAX_CONN_IDLE_TIME` | `30m`  | Max idle time before close |

### SMTP

Leave `SUDOSTREAM_SMTP_HOST` empty to disable outbound mail, therefore user invites and password reset links.

| Variable                    | Description                           |
|-----------------------------|---------------------------------------|
| `SUDOSTREAM_SMTP_HOST`      | SMTP server host; empty disables mail |
| `SUDOSTREAM_SMTP_PORT`      | Port (sample `587`; Mailpit `1025`)   |
| `SUDOSTREAM_SMTP_USER`      | SMTP username                         |
| `SUDOSTREAM_SMTP_PASSWORD`  | SMTP password                         |
| `SUDOSTREAM_SMTP_FROM`      | From address                          |
| `SUDOSTREAM_SMTP_FROM_NAME` | From display name                     |

### DLNA

Admin must still **enable** DLNA and pick a **TV** user. Envs only sets network behavior:

| Variable                         | Description                                                       |
|----------------------------------|-------------------------------------------------------------------|
| `SUDOSTREAM_DLNA_HTTP_PORT`      | HTTP port for description / SOAP / streams (default `8200`)       |
| `SUDOSTREAM_DLNA_HTTP_BIND`      | Listen address; empty = all interfaces                            |
| `SUDOSTREAM_DLNA_SSDP_MULTICAST` | SSDP multicast `host:port` (default `239.255.255.250:1900`)       |
| `SUDOSTREAM_DLNA_ADVERTISE_HOST` | Force LOCATION / stream host when auto-detect picks the wrong NIC |

### Provider API keys

Providers never block browse or play. Keys only matter when you want to enable a certain provider.

| Variable                            | Description                                                 |
|-------------------------------------|-------------------------------------------------------------|
| `SUDOSTREAM_TMDB_API_KEY`           | TMDB v3 API key                                             |
| `SUDOSTREAM_TMDB_ACCESS_TOKEN`      | TMDB v4 read access token (optional alternative/complement) |
| `SUDOSTREAM_TVDB_API_KEY`           | TheTVDB v4 API key                                          |
| `SUDOSTREAM_TVDB_PIN`               | Optional PIN for some subscriber keys                       |
| `SUDOSTREAM_FANART_API_KEY`         | Fanart.tv API key (needs prior `tmdb_id` / `tvdb_id`)       |
| `SUDOSTREAM_FANART_CLIENT_KEY`      | Fanart.tv client key                                        |
| `SUDOSTREAM_OPENSUBTITLES_API_KEY`  | OpenSubtitles API key                                       |
| `SUDOSTREAM_OPENSUBTITLES_USERNAME` | OpenSubtitles **account username** (not email)              |
| `SUDOSTREAM_OPENSUBTITLES_PASSWORD` | OpenSubtitles password                                      |
| `SUDOSTREAM_ANIDB_CLIENT`           | AniDB client name (required for titles/XML)                 |
| `SUDOSTREAM_ANIDB_USERNAME`         | Optional AniDB login for UDP ed2k FILE                      |
| `SUDOSTREAM_ANIDB_PASSWORD`         | Optional AniDB password                                     |

TVmaze / AniList need no key in the sample compose for basic use.

---

## Admin UI settings

### Auth (`Admin → Settings → Auth`)

| Setting                     | Description                                                 |
|-----------------------------|-------------------------------------------------------------|
| Email confirmation required | New accounts start disabled until confirm (or admin Enable) |
| Two-factor required         | Users must set up TOTP before normal use                    |

### Network (`Admin → Settings → Network`)

| Setting               | Description                                                                  |
|-----------------------|------------------------------------------------------------------------------|
| Proxy URL             | HTTP/HTTPS or SOCKS (`socks5h` preferred for remote DNS) for provider egress |
| Proxy user / password | Optional proxy auth (password can stay stored server-side)                   |
| No-proxy              | Host/suffix list that bypasses the proxy                                     |
| Test                  | Probes connectivity through the current draft settings                       |

### Transcoding (`Admin → Settings → Transcoding`)

| Setting                | Description                                                                         |
|------------------------|-------------------------------------------------------------------------------------|
| `hwAccel`              | `off` / `qsv` / `vaapi` / `nvenc` / `rockchip` — needs the matching compose overlay |
| Tone mapping enabled   | HDR/wide → SDR for the 8-bit HLS ladder when needed                                 |
| Tone mapping algorithm | `bt2390` / `hable` / `reinhard` / `mobius`                                          |
| Downmix algorithm      | Surround → stereo for remux/encode AAC (`ac4`, `none`, pan variants)                |
| Downmix boost          | Optional volume factor after pan (empty = algorithm default)                        |

Direct Play does not force stereo downmix — multi-channel audio stays as-is until remux/encode.

### DLNA (`Admin → Settings → DLNA`)

| Setting | Description                                                               |
|---------|---------------------------------------------------------------------------|
| Enabled | Turns the LAN Media Server on/off                                         |
| TV user | Principal with role **TV** whose library grants define the published tree |

### Recycle bin retention (`Admin → Settings`)

| Setting        | Description                                                                                   |
|----------------|-----------------------------------------------------------------------------------------------|
| Enabled        | When on, items past retention are removed at midnight UTC                                     |
| Retention days | Keep soft-deleted items this many days after delete (integer ≥ 1)                             |

Manual Empty all remains on **Admin → Deleted items**. Turn retention off to keep items until emptied manually.

### Libraries & providers (not cron)

| Where                   | Settings                                                                        |
|-------------------------|---------------------------------------------------------------------------------|
| Library create / detail | Name, type (`film` / `series` / …), folder roots                                |
| Library → providers     | Metadata / poster / subtitle provider, apply mode, write target, override flags |
| Users → Libraries       | Per-library `create` / `read` / `update` / `delete` grants                      |
| Users                   | Role (`admin` / `user` / `tv`), enable/disable, invites, 2FA reset              |
