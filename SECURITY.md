# Security policy

## Supported versions

Security fixes are applied to the latest release on `master` / `ghcr.io/fed1337/sudostream:latest`
and the current `dev` branch. Older tags are not supported unless noted in a security advisory.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security bugs.**

Report privately via [GitHub Security Advisories](https://github.com/fed1337/sudostream/security/advisories/new)
(preferred) or by contacting the repository owner through GitHub.

Include:

- Description and impact
- Steps to reproduce
- Affected version or commit
- Suggested fix (if any)

We aim to acknowledge reports within a few business days. Coordinated disclosure is appreciated.

## Scope

In scope:

- Authentication, session, and authorization bypass
- Path traversal or symlink escapes outside the media root (`/media`)
- Remote code execution in the server or container
- Secrets or credentials exposed in responses and logs

Out of scope (unless chained with the above):

- Denial of service from very large libraries (report as a performance issue)
- Missing hardening on optional compose examples (document expected host setup)
- Issues in third-party dependencies without a demonstrable exploit path in sudoStream
