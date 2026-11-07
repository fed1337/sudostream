# Users

Accounts, roles, library grants, and sessions.

## Roles

| Role | Web UI | Typical use |
|------|--------|-------------|
| **admin** | Full | Settings, users, libraries, trash, schedule |
| **user** | Yes | Browse / watch / download where granted |
| **tv** | **No** | DLNA principal only — holds library grants, cannot sign in |

Login is gated on `users.enabled`. When **email confirmation required** is on, new accounts start disabled until 
confirm (or admin **Enable**).

## Create & invite

| Path | When |
|------|------|
| **Admin → Users → Invite** | SMTP configured — email with accept link |
| **Admin → Users → Create** | No mail / local-only — set a temporary password (must change on first login) |

## Library grants (ACL)

**Admin → Users → (user) → Libraries**

Non-admins need explicit flags per library. Admins bypass all media ACL.

| Flag | Typical use |
|------|-------------|
| `read` | Browse, search, play, download |
| `create` | Writes where the product allows |
| `update` | Metadata edits and similar |
| `delete` | Soft-delete to trash |

!!! tip "TV devices"
    Create a **TV** user, grant `read` on libraries to publish, then select that user under **Admin → Settings → DLNA**.

## Security settings

**Admin → Settings → Authentication**

| Setting | Effect |
|---------|--------|
| Email confirmation required | New signups start disabled until confirm / admin Enable |
| Two-factor required | Users must enroll TOTP before normal use |

Users can enroll TOTP themselves; admins can **reset 2FA**. Sessions: list / revoke under the user.

User's personal page (details):

| Action | Who |
|--------|-----|
| Change password / email | User |
| Enroll / disable TOTP | User; admin can **reset 2FA** |
| List / revoke sessions | User (self) and admin (per user) |

---

## User preferences

These are **per-user**, not admin transcoding settings. Users edit them under avatar menu.

Currently allows to (and remembers last option): change theme, UI language, collapse/expand sidebars.

### Audio language priority

**Account → Playback**

| Setting | Description |
|---------|-------------|
| **Audio language 1–3** | Up to three language codes in priority order (e.g. `en`, `ru`, `gr`) |

Used when choosing the **default audio track for remux / HLS encode** (and the DEFAULT flag on the HLS master playlist).
**Direct Play** does not re-pick tracks for the user — the client plays the file as-is.

| Behavior | Detail |
|----------|--------|
| Empty list | File’s default / first suitable track |
| Match | commentary / descriptive tracks are skipped when a non-commentary alternative exists |
| Codes | Case-insensitive; short codes can match longer tags (e.g. `en` ↔ `eng`) |
