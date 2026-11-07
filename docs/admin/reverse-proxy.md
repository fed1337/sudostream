# Reverse proxy & cookies

sudoStream expects **you** to terminate TLS.

## Checklist

| Item | Guidance |
|------|----------|
| `SUDOSTREAM_BASE_URL` | Exact public URL users type (scheme + host + port if non-default) |
| `SUDOSTREAM_COOKIE_SECURE` | `true` when the UI is HTTPS |
| WebSocket / long requests | Allow normal HTTP/1.1 or HTTP/2 to the app |
| Forwarded headers | Set `X-Forwarded-*` / `Forwarded` as your stack requires for absolute URLs |

!!! warning
    Mismatched `BASE_URL` breaks invite/confirm/reset links and confuses cookie scope behind a path prefix. Prefer hosting at a hostname root.

## Cookies

- **Refresh** — httpOnly cookie
- **Access** — Bearer token (also `?access_token=` for Cast/HLS receivers)

??? tip "HTTP on LAN only"
    Leaving `COOKIE_SECURE=false` is fine for plain `http://192.168.x.x:8080` on a trusted network. Flip to `true` the moment you put HTTPS in front.

## Caddy Example

### Caddyfile

```caddy
stream.example.com {
    reverse_proxy sudostream:8080
}
```

### Environment variables

```env
SUDOSTREAM_BASE_URL=https://stream.example.com
SUDOSTREAM_COOKIE_SECURE=true
```
