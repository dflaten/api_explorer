# API Explorer

A lightweight Python CLI for exploring and testing HTTP APIs without reaching for Postman. Define each API in YAML, preview requests before sending them in JSON-shaped output, and keep API secrets in environment variables instead of hardcoding them.

## What Improved

- Easier first-run setup with `--init-config`
- YAML-first one-config-per-API workflow with config aliases from `configs/`
- Endpoint discovery with `--list` and `--describe`
- JSON-shaped request preview with `--dry-run`
- Safer config files with `${ENV_VAR}` expansion
- Default-config flow that now works as documented: `uv run api-cli health`
- Configurable response output path with `--output`

## Install

```bash
uv sync
```

## Fast Start

Create a starter config:

```bash
uv run api-cli --init-config config.yaml
```

Create a named API config in the default config directory:

```bash
uv run api-cli --init-config configs/github.yaml
```

Set your token in the shell instead of the file:

```bash
export API_TOKEN=your_token_here
```

List endpoints in the default config:

```bash
uv run api-cli --list
```

List available config files:

```bash
uv run api-cli --list-configs
```

Describe an endpoint without sending anything:

```bash
uv run api-cli --describe health
```

Preview the exact request that would be sent:

```bash
uv run api-cli health --dry-run
```

Execute a request:

```bash
uv run api-cli health
```

Use a non-default config file:

```bash
uv run api-cli newsapi.yaml top_headlines
```

Target a specific API by config alias:

```bash
uv run api-cli github health
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

4. Export any secrets in your shell:

```bash
export MY_API_KEY=your_key_here
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

## CLI Patterns

Call an endpoint with the default `config.yaml`:

```bash
uv run api-cli get_users
```

Call an endpoint with an explicit config:

```bash
uv run api-cli config.yaml get_users
```

List endpoints from a specific config:

```bash
uv run api-cli my_api.yaml --list
```

List configs from the default config directory:

```bash
uv run api-cli --list-configs
```

Call an endpoint using a config alias:

```bash
uv run api-cli github get_repo
```

Use a custom config directory:

```bash
uv run api-cli --config-dir api_configs --list-configs
```

Describe an endpoint from a specific config:

```bash
uv run api-cli my_api.yaml --describe get_users
```

Override query params and headers:

```bash
uv run api-cli get_users --params '{"page": 2}' --headers '{"X-Debug": "true"}'
```

Send a request body from a file:

```bash
uv run api-cli create_user --body user.json
```

Write the response somewhere else:

```bash
uv run api-cli get_users --output tmp/users.json
```

Run a request collection:

```bash
uv run api-cli config.yaml --collection smoke_tests.yaml
```

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
- Use a YAML file for each API
- Config aliases come from filenames inside `configs/` by default, so `configs/github.yaml` becomes `github`.
- Path parameters such as `/users/{id}` are filled from the endpoint's `params` block or from `--params`.
- Endpoint-level `headers`, `params`, and `body` are merged with CLI overrides.
- If a response JSON object contains `access_token`, the tool updates that config file's bearer token.
- Request and response previews are JSON-shaped in CLI output for readability.

## Real Example

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
export NEWS_API_KEY=REDACTED
uv run api-cli newsapi --describe top_headlines
uv run api-cli newsapi top_headlines
```

NewsAPI endpoint reference: https://newsapi.org/docs/endpoints/top-headlines
