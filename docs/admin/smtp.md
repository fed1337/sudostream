# SMTP

Outbound mail is optional. When configured, sudoStream can send invites, email confirmation, and password resets.

## Environment

| Variable | Role |
|----------|------|
| `SUDOSTREAM_SMTP_HOST` | Empty = mail disabled |
| `SUDOSTREAM_SMTP_PORT` | Sample `587` |
| `SUDOSTREAM_SMTP_USER` / `_PASSWORD` | Auth |
| `SUDOSTREAM_SMTP_FROM` / `_FROM_NAME` | From header |

Link TTL ENVs: `SUDOSTREAM_INVITE_TTL_HOURS`, `SUDOSTREAM_CONFIRM_EMAIL_TTL_HOURS`, `SUDOSTREAM_RESET_PASSWORD_TTL_HOURS`.

!!! note
    `SUDOSTREAM_BASE_URL` is embedded in emailed links — keep it correct

## Development (Mailpit)

`compose.dev.yml` starts [Mailpit](https://github.com/axllent/mailpit):

| Port | UI / SMTP |
|------|-----------|
| `8025` | Web inbox |
| `1025` | SMTP → app |

```env
SUDOSTREAM_SMTP_HOST=mailpit
SUDOSTREAM_SMTP_PORT=1025
```

??? tip "No SMTP in production?"
    Create users as admin and enable users manually (or disable email confirmation). Invites without SMTP are not useful.
