# API Explorer

<p align="center">
  <img alt="CLI" src="https://img.shields.io/badge/cli-apix-0f766e" />
  <img alt="Language" src="https://img.shields.io/badge/language-Go-2563eb" />
  <img alt="Config" src="https://img.shields.io/badge/config-YAML-f59e0b" />
  <img alt="Secrets" src="https://img.shields.io/badge/secrets-.env-7c3aed" />
  <img alt="Tests" src="https://img.shields.io/badge/tests-go%20%2B%20compat-16a34a" />
</p>

`apix` is a Go CLI for exploring and testing HTTP APIs from reusable YAML configs.

Define an API once, keep secrets in environment variables, preview the exact request, and then send it from the terminal.

| Item | Details |
| --- | --- |
| Command | `apix` |
| Config format | YAML API definitions |
| Default config home | `~/.config/apix` |
| API aliases | Files in `~/.config/apix/configs/` |
| Secrets | `~/.config/apix/.env` and `${ENV_VAR}` placeholders |
| Request workflow | List, describe, preview, execute, save response |
| Validation | Go tests, compatibility tests, and JSON schemas |

`apix` currently supports named API configs, request previews, bearer and basic auth, endpoint and CLI parameter merging, YAML request collections, response saving, and access-token persistence back into the local `.env` file.

## Install

Download the archive for your operating system and CPU from the
[latest GitHub Release](https://github.com/dflaten/api_explorer/releases/latest).

Extract the archive and install the `apix` binary somewhere on your `PATH`:

```bash
tar -xzf api-explorer_VERSION_linux_amd64.tar.gz
sudo install apix /usr/local/bin/apix
apix --version
```

On macOS, use the `darwin` archive for your CPU and install the binary the same way:

```bash
tar -xzf api-explorer_VERSION_darwin_arm64.tar.gz
install -m 755 apix /usr/local/bin/apix
apix --version
```

## Quick Start

Create a starter API config:

```bash
apix init github
```

Add local secrets:

```bash
mkdir -p ~/.config/apix
printf 'GITHUB_TOKEN=your_github_token_here\n' > ~/.config/apix/.env
```

Edit `~/.config/apix/.env`, then inspect and run the API:

```bash
apix configs
apix list github
apix preview github health
apix github health
```

## Example Config

Configs live in `~/.config/apix/configs/` by default. A file named `github.yaml` can be called as `apix github ...`.

```yaml
base_url: https://api.github.com
timeout: 30
default_headers:
  Accept: application/vnd.github+json
  Authorization: Bearer ${GITHUB_TOKEN}
  User-Agent: api-explorer/1.0
endpoints:
  get_repo:
    method: GET
    path: /repos/{owner}/{repo}
    params:
      owner: octocat
      repo: Hello-World
    description: Fetch a repository
```

Run it:

```bash
apix describe github get_repo
apix github get_repo --params '{"owner":"octocat","repo":"Hello-World"}'
```

## Common Commands

| Command | Purpose |
| --- | --- |
| `apix init NAME` | Create `~/.config/apix/configs/NAME.yaml` |
| `apix configs` | Show available config aliases |
| `apix list NAME` | List endpoints in a config |
| `apix describe NAME ENDPOINT` | Show endpoint details without sending a request |
| `apix preview NAME ENDPOINT` | Print the request that would be sent |
| `apix NAME ENDPOINT` | Execute an endpoint |
| `apix PATH.yaml ENDPOINT` | Use an explicit config path |
| `apix --config-dir PATH NAME ENDPOINT` | Resolve aliases from another directory |
| `apix NAME ENDPOINT --output out.json` | Save the response body to a custom file |

Run `apix --help` for all flags and examples.

## Documentation

- [Release guide](docs/RELEASING.md)
- [API config schema](schemas/api-config.schema.json)
- [Collection schema](schemas/collection.schema.json)

## Development

Normal development uses Git, Go 1.26 or newer, Make, and Staticcheck. GoReleaser v2 is only needed for local release validation and snapshot builds.

Common repo tasks:

```bash
make check
make compat
make release-check
make release-snapshot
```

Build the installed command from this checkout:

```bash
go install ./cmd/apix
```

GitHub Releases are created from pushed `v*` tags. Release archives keep the `api-explorer` prefix even though the executable is named `apix`.
