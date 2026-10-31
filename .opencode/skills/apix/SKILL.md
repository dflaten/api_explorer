---
name: API Calls With apix
description: Use apix for exploratory or test HTTP API calls instead of ad-hoc curl, wget, Python requests/httpx, or throwaway HTTP scripts. Covers discovering configured endpoints, previewing requests, running calls, and handling credentials.
---

# API Calls With apix

Prefer `apix` over ad-hoc HTTP tools whenever you need to explore an API, reproduce
a request, smoke-test an endpoint, or verify API behavior. Do not write a new HTTP
client or shell script for a call that an `apix` config can express.

If `apix` is not on `PATH`, build it from the `api_explorer` checkout with
`make build` and use `./bin/apix`, or run `go install ./cmd/apix`.

## Before calling

1. Find the API config:
   ```bash
   apix configs
   ```
2. List its endpoints:
   ```bash
   apix list API
   ```
3. Inspect the endpoint before using it:
   ```bash
   apix describe API ENDPOINT
   ```
4. For destructive, expensive, or unfamiliar requests, preview first:
   ```bash
   apix preview API ENDPOINT --params '{"id":"42"}'
   ```

Do not invent endpoint names or parameter names. If an endpoint is missing, add it
to a temporary config as described below, or ask the user where the API is defined.

## Running a request

Use machine-readable mode and avoid writing into the repository:

```bash
apix run API ENDPOINT --json --no-save --color never
```

- `--json` prints one JSON envelope with `request` and `response` objects. Informational
  messages go to stderr, so stdout is safe to pipe to `jq`.
- `--no-save` prevents the default `response.json` from being written to the current
  directory. If you need the exchange later, write it outside the repository with
  `--output /tmp/apix/response.json`.
- `--color never` or `NO_COLOR=1` keeps output free of ANSI escapes.

Overrides are JSON objects:

```bash
apix run API ENDPOINT \
  --params '{"owner":"octocat","repo":"Hello-World"}' \
  --headers '{"X-Trace":"agent"}' \
  --json --no-save --color never
```

Request bodies must be files:

```bash
printf '%s\n' '{"name":"example"}' > /tmp/apix/body.json
apix run API ENDPOINT --body /tmp/apix/body.json --json --no-save --color never
```

Inspect the envelope's `response.status_code`, `response.success`, and
`response.body`. By default, HTTP 4xx and 5xx responses still exit 0. Add `--fail`
when the command's exit code should reflect the HTTP result:

```bash
apix run API ENDPOINT --fail --json --no-save
```

`--fail` exits non-zero for status 400 or higher after printing the response.

## Temporary and unconfigured APIs

For a one-off API, create a config outside the repository and run it by explicit
path. Do not edit `~/.config/apix/configs` or another user's checked-in `configs/`
unless the user asks you to.

```bash
mkdir -p /tmp/apix
cat > /tmp/apix/example.yaml <<'YAML'
base_url: https://api.example.com
timeout: 30
default_headers:
  Accept: application/json
auth:
  type: bearer
  token: ${EXAMPLE_API_TOKEN}
endpoints:
  health:
    method: GET
    path: /health
  get_resource:
    method: GET
    path: /resources/{id}
    params:
      id: "123"
YAML

apix run /tmp/apix/example.yaml health --json --no-save --color never
```

Configs support `base_url`, `timeout`, `default_headers`, `auth` (bearer or
basic), and endpoints with `method`, `path` or `url`, `params`, `headers`,
`body`, `body_type`, and per-endpoint `auth`. Prefer `path` plus `base_url`.

For several calls against one API, use a YAML collection:

```bash
apix collection API /tmp/apix/smoke.yaml
```

## Credentials

- Keep secrets in environment variables or `~/.config/apix/.env`.
- Reference them in YAML as `${ENV_VAR}`. Never put a token directly in a config,
  CLI argument, request body, or committed file.
- Do not print `~/.config/apix/.env`, saved request files, or full headers.
  Sensitive headers are redacted in previews and saved output, but response bodies
  may still contain secrets.
- If a response contains `access_token`, `apix run` writes it back to
  `~/.config/apix/.env` automatically. Avoid calling token endpoints unless the
  user expects that credential update.

## Worktrees and shared credentials

`~/.config/apix` is per-user, not per-repository. All git worktrees and all
agents running as the same OS user share the same config aliases, `.env`
credentials, and logs, which is normally what you want.

- Prefer global aliases for APIs used across worktrees. Explicit config paths are
  resolved from the current worktree and may differ by branch.
- Do not export API tokens in the shell. Exported variables override
  `~/.config/apix/.env`, so the agent and developer can end up using different
  credentials.
- Use distinct environment variable names for different environments. Token
  persistence rewrites the shared `.env`, so reusing a generic name across
  worktrees can overwrite another environment's token.
- Request logs are grouped by config file name; same-named configs from different
  worktrees share a log directory.

## Avoid

- `curl`, `wget`, `httpie`, `requests`, or new scripts for APIs already covered by
  an `apix` config.
- `apix logs`, which opens an interactive viewer.
- Letting `apix run` write `response.json` into a repository; use `--no-save` or
  `--output /tmp/...`.
- Editing shared or user-specific configs without explicit permission.
