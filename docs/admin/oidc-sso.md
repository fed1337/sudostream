# OIDC SSO (external IdP)

sudoStream can act as an OpenID Connect **relying party** against Authelia, Authentik, Keycloak, or social OIDC issuers.
After a successful IdP login it issues **our** JWT + refresh cookie (same as password login).

Configuration is **env-only** (`SUDOSTREAM_OIDC_*`). There is no admin UI for secrets.

## Environment

| Variable | Purpose |
|----------|---------|
| `SUDOSTREAM_OIDC_ENABLED` | Master switch |
| `SUDOSTREAM_OIDC_ISSUER_URL` | Issuer (OIDC discovery) |
| `SUDOSTREAM_OIDC_ISSUER_NAME` | Login button label |
| `SUDOSTREAM_OIDC_CLIENT_ID` | Confidential client id |
| `SUDOSTREAM_OIDC_CLIENT_SECRET` | Client secret (server-only) |
| `SUDOSTREAM_OIDC_ONLY` | Hide local password form |
| `SUDOSTREAM_OIDC_AUTO_REDIRECT` | Bounce `/login` to the IdP |
| `SUDOSTREAM_OIDC_ADMIN_GROUP` | `groups` claim value → sudoStream **admin** |
| `SUDOSTREAM_OIDC_USERNAME_CLAIM` | Display claim (default `preferred_username`) |
| `SUDOSTREAM_OIDC_SCOPE` | Default `openid email profile groups` |
| `SUDOSTREAM_OIDC_AUTO_REGISTER` | Create users for unknown subjects |
| `SUDOSTREAM_OIDC_MERGE_USERS` | Link unknown `sub` to an existing user by email |
| `SUDOSTREAM_OIDC_REQUIRE_EMAIL_VERIFICATION` | Require `email_verified` before merge/register |
| `SUDOSTREAM_OIDC_INSECURE_SKIP_VERIFY` | Skip TLS verify for IdP (local Authelia self-signed only) |

Redirect URI (register on the IdP):

`{SUDOSTREAM_BASE_URL}/api/auth/oidc/callback`

## Authelia config sample

```shell
    clients:
      - client_id: 'sudostream'
        client_name: 'sudoStream'
        client_secret: '$pbkdf2-sha512$310000$.qFs7.GyQDArf4E2sfiGEg$qURQuIiQ/GkRplifXVM5UvlYwaHV43CDx/hxCbj2W6cnfir8OthVTXXcgcbfCXsr3Tbv//LxiZIC6rPORfMYEg'
        public: false
        authorization_policy: 'one_factor'
        redirect_uris:
          - 'http://localhost:8080/api/auth/oidc/callback'
        scopes:
          - 'openid'
          - 'profile'
          - 'email'
          - 'groups'
        response_types:
          - 'code'
        grant_types:
          - 'authorization_code'
        token_endpoint_auth_method: 'client_secret_basic'
        userinfo_signed_response_alg: 'none'
```
