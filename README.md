# API Explorer

A lightweight Python CLI for exploring and testing HTTP APIs without reaching for Postman. Define each API in YAML, preview requests before sending them in JSON-shaped output, and keep API secrets in environment variables instead of hardcoding them.

## Features

- 📁 One YAML config per API, with alias-based selection from `configs/`
- 🔍 JSON-shaped request previews with `--dry-run`
- 🧭 Endpoint discovery with `--list` and `--describe`
- 🔐 Local secret loading from `.env`
- 📦 Batch request execution with YAML collections
- 💾 Response saving to `response.json` or a custom `--output` path
- 🔑 Built-in auth support for bearer and basic auth

## Install

```bash
uv sync
```

## Fast Start

Create a named API config in the default config directory:

```bash
uv run api-cli --init-config configs/github.yaml
```

Create a local `.env` file from the example template:

```bash
cp .env.example .env
```

Edit `.env` with the tokens or API keys referenced by your config.

List available config files:

```bash
uv run api-cli --list-configs
```

Describe an endpoint without sending anything:

```bash
uv run api-cli github --describe health
```

Preview the exact request that would be sent:

```bash
uv run api-cli github health --dry-run
```

Execute a request:

```bash
uv run api-cli github health
```

Use an explicit config path:

```bash
uv run api-cli configs/newsapi.yaml top_headlines
```

## Set Up A New API

1. Create a config file for the API:

```bash
uv run api-cli --init-config configs/my_api.yaml
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
uv run api-cli --list-configs
uv run api-cli my_api --list
```

6. Preview the request before sending it:

```bash
uv run api-cli my_api get_user --dry-run
```

7. Run the endpoint:

```bash
uv run api-cli my_api get_user
```

Tips:

- Start with a simple `health`, `me`, or `list` endpoint before adding write operations.
- Put large request payloads in separate JSON files and pass them with `--body`.
- Use endpoint `description` fields so `--list` output stays readable.
- Keep secrets in `.env`, not in the checked-in YAML config.

Run `uv run api-cli --help` for command patterns and examples.

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
uv run api-cli newsapi --describe top_headlines
uv run api-cli newsapi top_headlines
```

NewsAPI endpoint reference: https://newsapi.org/docs/endpoints/top-headlines

## Development

Install development tools:

```bash
uv sync --group dev
```

Run tests:

```bash
make test
```

Run type checking:

```bash
make typecheck
```

Run formatting:

```bash
make format
```

Run the full local sequence:

```bash
make check
```
