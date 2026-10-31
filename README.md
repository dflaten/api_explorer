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

`apix` currently supports named API configs, request previews, bearer and basic auth (including per-endpoint overrides), endpoint and CLI parameter merging, YAML request collections, response saving, and access-token persistence back into the local `.env` file.

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
install -m 600 /dev/null ~/.config/apix/.env
printf 'GITHUB_TOKEN=your_github_token_here\n' > ~/.config/apix/.env
```

Edit `~/.config/apix/.env`, then inspect and run the API:

```bash
apix configs
apix list github
apix preview github health
apix run github health
```

## Example Config

Configs live in `~/.config/apix/configs/` by default. A file named `github.yaml` can be called by alias, such as `apix list github`.

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
apix run github get_repo --params '{"owner":"octocat","repo":"Hello-World"}'
```

## Token Endpoints with Client Credentials

An endpoint-level `auth` block overrides the API-wide authentication for that
request. This is useful when normal API calls use a bearer token, while the
token endpoint uses HTTP Basic client credentials:

```yaml
auth:
  type: bearer
  token: ${API_TOKEN}
endpoints:
  token:
    method: POST
    path: /oauth/token
    auth:
      type: basic
      username: ${CLIENT_ID}
      password: ${CLIENT_SECRET}
    body_type: form
    body:
      grant_type: client_credentials
  get_profile:
    method: GET
    path: /me
```

The `token` request sends Basic auth; `get_profile` continues to send the
default bearer token.

## Common Commands

| Command | Purpose |
| --- | --- |
| `apix init NAME` | Create `~/.config/apix/configs/NAME.yaml` |
| `apix configs` | Show available config aliases |
| `apix list API` | List endpoints in a config |
| `apix describe API ENDPOINT` | Show endpoint details without sending a request |
| `apix preview API ENDPOINT` | Print the request that would be sent |
| `apix run API ENDPOINT` | Execute an endpoint |
| `apix run PATH.yaml ENDPOINT` | Use an explicit config path |
| `apix logs API` | Browse request logs for an API with arrow-key selection |
| `apix --config-dir PATH run API ENDPOINT` | Resolve aliases from another directory |
| `apix run API ENDPOINT --output out.json` | Save the request/response exchange to a custom file |

Run `apix --help` for all flags and examples.

## Automation and Agent Use

`apix run` has a machine-readable mode for scripts and coding agents:

```bash
apix run github get_repo --json --no-save
apix run github health --fail --no-save
apix run github create_issue --body issue.json --json --fail --no-save
```

- `--json` prints only a JSON envelope with the request and response. Informational
  messages go to stderr so stdout can be parsed directly.
- `--no-save` skips the default `response.json` output file.
- `--fail` exits non-zero when the HTTP status is 400 or higher.

This repository also ships an OpenCode skill at
[`.opencode/skills/apix/SKILL.md`](./.opencode/skills/apix/SKILL.md) that tells
coding agents to prefer `apix` over ad-hoc HTTP tooling and documents the
recommended workflow.

## Git Worktrees

By default, `apix` keeps configs, credentials, and logs under `~/.config/apix`.
That location is per-user, not per-repository, so every git worktree and every
agent running as the same OS user shares the same aliases and credentials. An
agent can authenticate once and a developer can rerun the same `apix` command
from another worktree without copying secrets.

An agent running with a different `HOME` (for example in a container or sandbox)
will not see this shared state; mount or copy `~/.config/apix` if those agents
need the same credentials.

A few things to keep in mind:

- Put shared secrets in `~/.config/apix/.env`. `apix` creates or rewrites that
  file with mode `0600`. Variables already exported in the shell take precedence
  over the file, so an exported token can make a developer use different
  credentials than the agent did.
- Use global aliases (`apix list github`) for APIs shared across worktrees.
  Explicit paths (`apix run ./configs/github.yaml ...`) are resolved from the
  current worktree, so they can differ by branch.
- Request logs are grouped by config file name. Two worktrees using same-named
  explicit configs share a log directory; use distinct file names or aliases if
  you need separate histories.
- Avoid using the same environment variable name for different environments
  across worktrees. `access_token` persistence rewrites the shared `.env`, so a
  staging token refresh can overwrite a production token stored under the same
  name.

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
