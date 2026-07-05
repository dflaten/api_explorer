# Publishing GitHub Releases

API Explorer can be distributed as prebuilt binaries. End users do not need Go installed; they download the archive for their operating system and CPU architecture.

The repository uses GoReleaser and GitHub Actions to publish release archives from version tags.

## Release Requirements

Normal development uses Git, Go 1.26 or newer, Make, and Staticcheck. Local release validation additionally requires GoReleaser v2:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/goreleaser/goreleaser/v2@latest
```

Ensure `$(go env GOPATH)/bin` is on `PATH`, then verify the tools:

```bash
git --version
go version
make --version
staticcheck -version
goreleaser --version
```

The GitHub CLI (`gh`) is optional. Git and the GitHub web interface are sufficient because pushing a `v*` tag starts the release workflow.

## Release Targets

Build these four targets initially:

| Operating system | Architecture | Typical hardware |
| --- | --- | --- |
| Linux | `amd64` | Intel/AMD 64-bit systems |
| Linux | `arm64` | ARM servers and Raspberry Pi 64-bit systems |
| macOS | `amd64` | Intel Macs |
| macOS | `arm64` | Apple Silicon Macs |

Each GitHub release should contain:

```text
api-explorer_VERSION_linux_amd64.tar.gz
api-explorer_VERSION_linux_arm64.tar.gz
api-explorer_VERSION_darwin_amd64.tar.gz
api-explorer_VERSION_darwin_arm64.tar.gz
checksums.txt
```

## Implemented Automation

### Version Information

The Go program exposes a `--version` option backed by build metadata variables:

```go
var (
	version = "0.5.0"
	commit  = "none"
	date    = "unknown"
)
```

GoReleaser will set them through linker flags. The CLI should print something similar to:

```text
apix 0.5.0 (commit abc1234, built 2026-06-14T12:00:00Z)
```

Native Go and CLI integration tests cover `--version`.

### GoReleaser Configuration

The repository root contains `.goreleaser.yaml` with the following release structure:

```yaml
version: 2

project_name: api-explorer

builds:
  - id: apix
    main: ./cmd/apix
    binary: apix
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    flags:
      - -trimpath
    ldflags:
      - >-
        -s -w
        -X main.version={{ .Version }}
        -X main.commit={{ .Commit }}
        -X main.date={{ .Date }}

archives:
  - id: release-archives
    formats:
      - tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    files:
      - README.md
      - schemas/*.json

checksum:
  name_template: checksums.txt

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
```

Including `schemas/*.json` in each archive gives users the machine-readable configuration contract alongside the executable.

### GitHub Actions Workflow

The `.github/workflows/release.yml` workflow runs for pushed `v*` tags:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Check out repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Install Staticcheck
        run: |
          go install honnef.co/go/tools/cmd/staticcheck@latest
          echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"

      - name: Validate source
        run: make check

      - name: Create GitHub release
        uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

`permissions: contents: write` allows the workflow's built-in `GITHUB_TOKEN` to create the release and upload artifacts. No personal access token should be required for this repository.

The native Go test suite includes localhost HTTP integration tests, so `make check` validates unit and integration behavior before publishing.

### Local Release Commands

The Makefile provides:

```make
.PHONY: release-check release-snapshot

release-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean
```

`release-snapshot` creates local artifacts under the ignored `dist/` directory without publishing a GitHub release.

## First-Time Validation

Before pushing the first release tag:

```bash
make check
goreleaser check
goreleaser release --snapshot --clean
```

Inspect `dist/` and verify all four archives exist. Extract and test the archive matching the local machine:

```bash
tar -xzf dist/api-explorer_*_linux_amd64.tar.gz
./apix --version
./apix --help
```

For stronger validation, run each Linux binary through a container or matching machine. The macOS binaries should be tested on both Intel and Apple Silicon before announcing broad support.

## Publishing a Release

1. Merge all intended changes into `main`.
2. Confirm the `main` branch checks pass.
3. Choose a semantic version such as `v0.5.0`.
4. Create and push an annotated tag:

```bash
git switch main
git pull --ff-only
git tag -a v0.5.0 -m "API Explorer v0.5.0"
git push origin v0.5.0
```

5. Open the repository's Actions page and monitor the `Release` workflow.
6. Open the GitHub Releases page and verify the four archives, checksum file, and generated release notes.
7. Download at least one archive and verify its checksum and executable.

Do not move or reuse a published version tag. If a release is defective, fix the problem and publish a new patch version such as `v0.4.1`.

## Verifying Downloads

Linux users can verify an archive with:

```bash
grep 'api-explorer_0.5.0_linux_amd64.tar.gz' checksums.txt | sha256sum --check
```

macOS users can use:

```bash
grep 'api-explorer_0.5.0_darwin_arm64.tar.gz' checksums.txt | shasum -a 256 --check
```

## End-User Installation

Linux example:

```bash
tar -xzf api-explorer_0.5.0_linux_amd64.tar.gz
sudo install apix /usr/local/bin/apix
apix --help
```

macOS example:

```bash
tar -xzf api-explorer_0.5.0_darwin_arm64.tar.gz
install -m 755 apix /usr/local/bin/apix
apix --help
```

Apple may warn about or quarantine unsigned binaries downloaded from GitHub. The initial release can document that limitation. A polished macOS distribution should add Apple Developer ID signing and notarization rather than asking users to bypass Gatekeeper.

## Later Improvements

After archive releases are stable, consider adding:

- A Homebrew tap for macOS and Linux
- Signed checksums or build provenance
- SBOM generation
- Linux `.deb` and `.rpm` packages
- Apple signing and notarization
- Windows `amd64` and `arm64` binaries

## References

- [GoReleaser quick start](https://goreleaser.com/getting-started/quick-start/)
- [GoReleaser GitHub Actions integration](https://goreleaser.com/customization/ci/actions/)
- [GoReleaser Go builds](https://goreleaser.com/customization/builds/builders/go/)
- [GitHub Actions `GITHUB_TOKEN` authentication](https://docs.github.com/en/actions/tutorials/authenticate-with-github_token)
