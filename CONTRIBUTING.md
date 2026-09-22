# Contributing to sudoStream

Thank you for your interest in contributing. This project is early-stage. Small, focused changes are easier to review,
therefore much appreciated.

## Development setup

Run the stack via Docker Compose — no local Go binary required. To list all targets: `make help`.

**API docs:** when Go handlers change, run `make openapi && make frontend-typegen` — never hand-edit `openapi/`.

**Coverage:** project floor **≥ 60%**; touched packages / PR patch **≥ 70%** ([`codecov.yml`](codecov.yml)).

**Tests:** Go tests use [allure-go](https://github.com/allure-framework/allure-go) (`commons/gotest`). Match the `allure.Test` pattern in existing `*_test.go`
files. Do not put secrets in test titles or attachments — published reports are treated as public QA.

## Pull requests

1. Branch from `dev`
2. Keep diffs small; match existing style and patterns.
3. Add or update tests for behavior changes.
4. Run `make fix && make check && make test` (and `make frontend-fix && make frontend-check` if you touched `web/`) before opening a PR.

## License

By contributing, you agree that your contributions are licensed under the same terms as the project ([
`license`](license) — MIT).
