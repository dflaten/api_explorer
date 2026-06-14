# Agent Notes

This repository is a Go CLI project. Use these conventions when working here:

## Core Commands

- Build the local binary with `make build`.
- Run the main verification flow with `make check`.
- Run the compatibility suite with `make compat`.
- Run Go-only checks directly with `go vet ./...` and `go test ./...`.
- Build a release snapshot with `make release-snapshot`.
- Validate the release config with `make release-check`.

## Tooling

- The installed command is `apix`.
- Source builds use `go install ./cmd/apix`.
- GoReleaser v2 is required for local release validation and snapshot builds.
- Git, Go 1.26+, and Make are the normal development prerequisites.

## Release Flow

- GitHub Releases are created from pushed `v*` tags.
- The release workflow lives in [`.github/workflows/release.yml`](./.github/workflows/release.yml).
- Release archives are configured in [`.goreleaser.yaml`](./.goreleaser.yaml).
- Do not rename the published archive prefix; it stays `api-explorer` even though the executable is `apix`.

## Repository Layout

- Application code lives in [`cmd/apix`](./cmd/apix).
- Release and setup notes live in [`docs/RELEASING.md`](./docs/RELEASING.md).
- YAML schemas live in [`schemas/`](./schemas).
- Runtime fixtures for tests live in [`cmd/apix/testdata`](./cmd/apix/testdata).

## Local Files

- `config.yaml` is a local example config and is ignored by Git.
- `response.json` is a generated response artifact and is ignored by Git.
- `configs/` may contain user-specific API configs; do not overwrite or delete them unless explicitly asked.

## Editing Rules

- Keep edits focused on the requested change.
- Prefer existing repo patterns over introducing new abstractions.
- Use `apply_patch` for file edits.
- Do not remove or rewrite local untracked files that are not part of the task.

## Verification Expectations

- For code changes, run the relevant Go tests at minimum.
- For CLI or release changes, run `make check`.
- For release work, validate with `make release-check` and `make release-snapshot`.
