# API Explorer

A lightweight Go CLI for exploring and testing HTTP APIs. Define each API in YAML, preview requests before sending them in JSON-shaped output, and keep API secrets in environment variables instead of hardcoding them.

## Features

- 📁 One YAML config per API, with alias-based selection from `configs/`
- 🔍 JSON-shaped request previews with `--request-preview`
- 🧭 Endpoint discovery with `--list` and `--describe`
- 🔐 Local secret loading from `.env`
- 📦 Batch request execution with YAML collections
- 💾 Response saving to `response.json` or a custom `--output` path
- 🔑 Built-in auth support for bearer and basic auth

## Development Setup

### Requirements

| Tool | Required version | Used for |
| --- | --- | --- |
| Git | Current supported release | Cloning, branches, tags, and publishing release tags |
| Go | 1.26 or newer | Building, installing, formatting, vetting, and testing |
| Make | Any modern GNU Make | Running the repository's standard development commands |
| GoReleaser | v2 | Validating and creating release artifacts locally |

GoReleaser is only required for `make release-check` and `make release-snapshot`. GitHub Actions installs it automatically when publishing a tagged release.

Ubuntu/Debian example for Git and Make:

```bash
sudo apt update
sudo apt install git make
```

macOS provides Git and Make through the Xcode command-line tools:

```bash
xcode-select --install
```

Install Go using the instructions at [go.dev/doc/install](https://go.dev/doc/install), then install GoReleaser v2:

```bash
go install github.com/goreleaser/goreleaser/v2@latest
```

Both `apix` and `goreleaser` are installed into `$(go env GOPATH)/bin`, which is usually `~/go/bin`. Add that directory to your shell's `PATH` once.

Fish:

```fish
fish_add_path (go env GOPATH)/bin
```

Bash:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

Zsh:

```zsh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Verify the development tools:

```bash
git --version
go version
make --version
goreleaser --version
```

### Install From Source

Clone the repository, then run the following command from the repository root:

```bash
go install ./cmd/apix
```

Verify the installation:

```bash
apix --help
```

After changing the source, run `go install ./cmd/apix` again to update the command installed in your shell. `make build` is also available when you only need a repository-local binary at `bin/apix`.

The GitHub CLI (`gh`) is optional. It can be used to inspect workflow runs and releases, but it is not required to build or publish because pushing a version tag triggers the release workflow.

## Fast Start

Create a named API config in the default config directory:

```bash
apix --init-config configs/github.yaml
```

Create a local `.env` file from the example template:

```bash
cp .env.example .env
```

Edit `.env` with the tokens or API keys referenced by your config.

List available config files:

```bash
apix --list-configs
```

Describe an endpoint without sending anything:

```bash
apix github --describe health
```

Preview the exact request that would be sent:

```bash
apix github health --request-preview
```

Execute a request:

```bash
apix github health
```

Use an explicit config path:

```bash
apix configs/newsapi.yaml top_headlines
```

## Set Up A New API

1. Create a config file for the API:

```bash
apix --init-config configs/my_api.yaml
```

2. Update the top-level settings:

- Set `base_url` to the API root URL
- Add any shared auth or required headers under `default_headers`
- If the API uses bearer auth, prefer an environment variable such as `${MY_API_TOKEN}`

Example:

```yaml
base_url: https://api.example.com/v1
timeout: 30
default_headers:
  Content-Type: application/json
  X-API-KEY: ${MY_API_KEY}
```

3. Add one or two starter endpoints under `endpoints`:

```yaml
endpoints:
  health:
    method: GET
    path: /health
    description: Basic connectivity check
  get_user:
    method: GET
    path: /users/{id}
    params:
      id: "123"
```

4. Add secrets to `.env`:

```dotenv
MY_API_KEY=your_key_here
```

5. Confirm the config is discoverable:

```bash
apix --list-configs
apix my_api --list
```

6. Preview the request before sending it:

```bash
apix my_api get_user --request-preview
```

7. Run the endpoint:

```bash
apix my_api get_user
```

Tips:

- Start with a simple `health`, `me`, or `list` endpoint before adding write operations.
- Put large request payloads in separate JSON files and pass them with `--body`.
- Use endpoint `description` fields so `--list` output stays readable.
- Keep secrets in `.env`, not in the checked-in YAML config.

Run `apix --help` for command patterns and examples.

## Config Format

Each config file represents one API and is written in YAML. Keep separate files such as:

- `configs/github.yaml`
- `configs/stripe.yaml`
- `configs/slack.yaml`

Example config:

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
  list_user_repos:
    method: GET
    path: /users/{username}/repos
    params:
      username: octocat
```

Notes:

- API configs and collections are validated according to the JSON Schemas in `schemas/` before execution.
- Unknown fields are allowed for forward-compatible metadata, but known fields and required values must match the schema.
- `${API_TOKEN}` style placeholders are expanded from environment variables when the config loads.
- `.env` is loaded automatically when the CLI starts, and existing shell variables win if both are set.
- Use a YAML file for each API
- Config aliases come from filenames inside `configs/` by default, so `configs/github.yaml` becomes `github`.
- Path parameters such as `/users/{id}` are filled from the endpoint's `params` block or from `--params`.
- Endpoint-level `headers`, `params`, and `body` are merged with CLI overrides.
- If a response JSON object contains `access_token`, the tool updates the referenced `.env` variable when `auth.token` uses `${ENV_VAR}` syntax.
- Multiple APIs can all return `access_token`; keep them separate by using different env vars such as `${GITHUB_TOKEN}` and `${SLACK_TOKEN}` in each config.
- Request and response previews are JSON-shaped in CLI output for readability.

## Real Example

Create the local `.env` file and add your NewsAPI key:

```bash
cp .env.example .env
```

```dotenv
NEWS_API_KEY=your_newsapi_key_here
```

Then add the API config:

```yaml
base_url: https://newsapi.org/v2/
timeout: 30
default_headers:
  Content-Type: application/json
  X-API-KEY: ${NEWS_API_KEY}
endpoints:
  top_headlines:
    method: GET
    path: top-headlines
    params:
      country: us
    description: Top US headlines
```

```bash
apix newsapi --describe top_headlines
apix newsapi top_headlines
```

NewsAPI endpoint reference: https://newsapi.org/docs/endpoints/top-headlines

## Development Commands

Run tests:

```bash
make test
```

Run Go static analysis:

```bash
make vet
```

Run formatting:

```bash
make format
```

Run the full local sequence:

```bash
make check
```

Run only the black-box compatibility suite:

```bash
make compat
```

The native Go compatibility suite checks request previews, HTTP requests, collections, output files, secret redaction, and schema failures.

Validate the GoReleaser configuration:

```bash
make release-check
```

Build all release archives locally without publishing:

```bash
make release-snapshot
```

Release snapshots are written to the ignored `dist/` directory.

## Releases

See [docs/RELEASING.md](docs/RELEASING.md) for the repository-specific plan to publish prebuilt Linux and macOS binaries through GitHub Releases.
