import argparse
import json
import os
import re
import sys
from pathlib import Path

import yaml

from api_client import APIClient


DEFAULT_CONFIG_TEMPLATE = {
    "base_url": "https://api.example.com",
    "timeout": 30,
    "default_headers": {
        "Content-Type": "application/json",
        "User-Agent": "api-explorer/1.0",
    },
    "auth": {
        "type": "bearer",
        "token": "${API_TOKEN}",
    },
    "endpoints": {
        "health": {
            "method": "GET",
            "path": "/health",
            "description": "Basic connectivity check",
        },
        "get_resource": {
            "method": "GET",
            "path": "/resources/{id}",
            "params": {
                "id": "123",
            },
            "description": "Example path parameter substitution",
        },
        "create_resource": {
            "method": "POST",
            "path": "/resources",
            "body": {
                "name": "example",
            },
            "description": "Example JSON body request",
        },
    },
}


SENSITIVE_HEADER_NAMES = {
    "authorization",
    "proxy-authorization",
    "x-api-key",
    "api-key",
}

ENV_TOKEN_PATTERN = re.compile(r"^\$\{([A-Z0-9_]+)\}$")


HELP_EPILOG = """\
Examples:
  Create a new API config:
    api-cli --init-config configs/github.yaml

  See available API configs:
    api-cli --list-configs

  List endpoints for an API alias:
    api-cli github --list

  Describe an endpoint:
    api-cli github --describe get_repo

  Preview a request without sending it:
    api-cli github get_repo --dry-run

  Run a request with JSON overrides:
    api-cli github get_repo --params '{"owner":"octocat"}'

  Send a request body from a file:
    api-cli github create_issue --body issue.json

  Run a request collection:
    api-cli github --collection smoke_tests.yaml

Config workflow:
  1. Put one YAML config per API in configs/
  2. Put secrets in .env
  3. Use the config filename as the CLI alias
"""


def load_env_file(env_path: str = ".env"):
    path = Path(env_path)
    if not path.exists():
        return

    for raw_line in path.read_text().splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue

        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip()
        if not key or key in os.environ:
            continue
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
            value = value[1:-1]
        os.environ[key] = value


def parse_json_argument(value: str, label: str):
    try:
        return json.loads(value)
    except json.JSONDecodeError as exc:
        raise ValueError(f"Invalid JSON for {label}: {exc}") from exc


def write_config_template(config_path: str):
    path = Path(config_path)
    if path.exists():
        raise FileExistsError(f"Refusing to overwrite existing file: {config_path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w") as f:
        yaml.safe_dump(DEFAULT_CONFIG_TEMPLATE, f, sort_keys=False)


def discover_config_files(config_dir: str):
    directory = Path(config_dir)
    if not directory.exists():
        return []
    return sorted(
        path
        for pattern in ("*.yaml", "*.yml")
        for path in directory.glob(pattern)
        if path.is_file()
    )


def resolve_config_path(config_spec: str, config_dir: str):
    candidate = Path(config_spec)
    if candidate.exists():
        return str(candidate)

    alias_candidates = [
        Path(config_dir) / config_spec,
        Path(config_dir) / f"{config_spec}.yaml",
        Path(config_dir) / f"{config_spec}.yml",
    ]
    for alias_path in alias_candidates:
        if alias_path.exists():
            return str(alias_path)

    if candidate.suffix in {".yaml", ".yml"} or "/" in config_spec:
        return config_spec

    raise ValueError(
        f"Unknown config alias '{config_spec}'. Use --list-configs to see available configs."
    )


def resolve_config_and_endpoint(targets, collection_path, config_dir):
    if collection_path:
        if len(targets) > 1:
            raise ValueError(
                "Collection mode accepts at most one positional argument: [config]"
            )
        config_path = (
            resolve_config_path(targets[0], config_dir) if targets else "config.yaml"
        )
        return config_path, None

    if not targets:
        raise ValueError(
            "Provide an endpoint name, or use --list / --describe / --init-config"
        )

    if len(targets) == 1:
        return "config.yaml", targets[0]

    if len(targets) == 2:
        return resolve_config_path(targets[0], config_dir), targets[1]

    raise ValueError(
        "Too many positional arguments. Use [endpoint] or [config endpoint]"
    )


def resolve_config_only(targets, config_dir):
    if not targets:
        return "config.yaml"
    if len(targets) == 1:
        return resolve_config_path(targets[0], config_dir)
    raise ValueError("Too many positional arguments. Use [config] with --list")


def print_config_list(config_dir: str):
    config_files = discover_config_files(config_dir)
    print(f"Config directory: {Path(config_dir)}")
    if not config_files:
        print("No config files found.")
        return

    print("Available configs:")
    for path in config_files:
        print(f"  {path.stem:<20} {path}")


def print_endpoint_list(client: APIClient):
    endpoints = client.list_endpoints()
    if not endpoints:
        print("No endpoints found in config.")
        return

    print(f"Config: {client.config_path}")
    print("Available endpoints:")
    for name, endpoint in endpoints.items():
        method = endpoint.get("method", "?").upper()
        path = endpoint.get("path") or endpoint.get("url", "")
        description = endpoint.get("description")
        line = f"  {name:<20} {method:<6} {path}"
        if description:
            line = f"{line}  - {description}"
        print(line)


def print_endpoint_details(client: APIClient, endpoint_name: str):
    request_definition = client.build_request_definition(endpoint_name)
    endpoint = request_definition["definition"]

    print(f"Endpoint: {endpoint_name}")
    if endpoint.get("description"):
        print(f"Description: {endpoint['description']}")
    print(f"Method: {request_definition['request_kwargs']['method']}")
    print(f"URL: {request_definition['full_url']}")

    if endpoint.get("headers"):
        print("\nEndpoint Headers:")
        print(json.dumps(endpoint["headers"], indent=2))
    if request_definition["request_kwargs"].get("params"):
        print("\nDefault Query Parameters:")
        print(json.dumps(request_definition["request_kwargs"]["params"], indent=2))
    if request_definition["body"] is not None:
        print("\nDefault Body:")
        print(json.dumps(request_definition["body"], indent=2))


def print_request_preview(request_definition):
    request_kwargs = request_definition["request_kwargs"]
    print("Request Preview:")
    print(f"  Method: {request_kwargs['method']}")
    print(f"  URL: {request_definition['full_url']}")
    print(f"  Timeout: {request_kwargs['timeout']}s")
    if request_definition.get("effective_headers"):
        print("  Headers:")
        print(
            json.dumps(
                redact_headers(request_definition["effective_headers"]), indent=2
            )
        )
    if request_kwargs.get("params"):
        print("  Query Params:")
        print(json.dumps(request_kwargs["params"], indent=2))
    if "json" in request_kwargs:
        print("  JSON Body:")
        print(json.dumps(request_kwargs["json"], indent=2))
    if "data" in request_kwargs:
        print("  Form Body:")
        print(json.dumps(request_kwargs["data"], indent=2))


def save_response(output_path: str, response_json, response_text: str):
    with open(output_path, "w") as f:
        if response_json is not None:
            json.dump(response_json, f, indent=2)
        else:
            json.dump({"raw": response_text}, f, indent=2)
        f.write("\n")


def print_response(response, verbose: bool):
    print(f"Status Code: {response.status_code}")
    print(f"Success: {response.ok}")

    if verbose:
        print(f"\nResponse Headers:")
        print(json.dumps(dict(response.headers), indent=2))


def emit_response_body(response_body):
    print(f"\nResponse Body:")
    if isinstance(response_body, (dict, list)):
        print(json.dumps(response_body, indent=2))
        return response_body, None

    if response_body is None:
        print("null")
        return None, None

    print(response_body)
    return None, response_body


def redact_headers(headers):
    redacted = {}
    for key, value in headers.items():
        if key.lower() in SENSITIVE_HEADER_NAMES:
            redacted[key] = "<redacted>"
        else:
            redacted[key] = value
    return redacted


def update_env_value(env_path: str, key: str, value: str):
    path = Path(env_path)
    lines = []
    found = False

    if path.exists():
        lines = path.read_text().splitlines()

    updated_lines = []
    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            updated_lines.append(line)
            continue

        content = stripped[7:].strip() if stripped.startswith("export ") else stripped
        if "=" not in content:
            updated_lines.append(line)
            continue

        existing_key, _ = content.split("=", 1)
        if existing_key.strip() == key:
            updated_lines.append(f"{key}={value}")
            found = True
        else:
            updated_lines.append(line)

    if not found:
        if updated_lines:
            updated_lines.append("")
        updated_lines.append(f"{key}={value}")

    path.write_text("\n".join(updated_lines) + "\n")


def persist_access_token(config_path: str, access_token: str, env_path: str = ".env"):
    with open(config_path, "r") as f:
        config = yaml.safe_load(f) or {}

    token_value = config.get("auth", {}).get("token")
    if not isinstance(token_value, str):
        raise ValueError("Config auth token is missing or not a string")

    match = ENV_TOKEN_PATTERN.match(token_value)
    if not match:
        raise ValueError(
            "Config auth token must use ${ENV_VAR} syntax to persist access_token into .env"
        )

    update_env_value(env_path, match.group(1), access_token)


def main():
    load_env_file()

    parser = argparse.ArgumentParser(
        description="CLI API explorer for YAML-defined API configs.",
        epilog=HELP_EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "targets",
        nargs="*",
        help="Use [endpoint], [config endpoint], or [config_alias endpoint]",
    )
    parser.add_argument("--body", help="Path to JSON body file")
    parser.add_argument("--params", help="Query parameters as JSON string")
    parser.add_argument("--headers", help="Additional headers as JSON string")
    parser.add_argument("--collection", help="Execute collection file")
    parser.add_argument(
        "--config-dir", default="configs", help="Directory used for YAML config aliases"
    )
    parser.add_argument(
        "--list-configs",
        action="store_true",
        help="List config files in the config directory",
    )
    parser.add_argument("--list", action="store_true", help="List configured endpoints")
    parser.add_argument(
        "--describe", metavar="ENDPOINT", help="Show endpoint details without executing"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Build and print the request without sending it",
    )
    parser.add_argument(
        "--init-config",
        metavar="PATH",
        help="Create a starter YAML config file for a new API",
    )
    parser.add_argument(
        "--output", default="response.json", help="Path to save the response body"
    )
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")

    parse_args = getattr(parser, "parse_intermixed_args", parser.parse_args)
    args = parse_args()

    try:
        if args.init_config:
            write_config_template(args.init_config)
            print(f"Created starter config at {args.init_config}")
            print(
                "Replace placeholders like ${API_TOKEN} with environment variables before calling the API."
            )
            return

        if args.list_configs:
            if args.targets:
                raise ValueError("--list-configs does not take positional arguments")
            print_config_list(args.config_dir)
            return

        if args.list:
            config_path = resolve_config_only(args.targets, args.config_dir)
            endpoint = None
        elif args.describe:
            if len(args.targets) > 1:
                raise ValueError(
                    "Use --describe ENDPOINT with an optional [config] positional argument"
                )
            config_path = (
                resolve_config_path(args.targets[0], args.config_dir)
                if args.targets
                else "config.yaml"
            )
            endpoint = None
        else:
            config_path, endpoint = resolve_config_and_endpoint(
                args.targets, args.collection, args.config_dir
            )

        client = APIClient(config_path=config_path)

        if args.list:
            print_endpoint_list(client)
            return

        if args.describe:
            print_endpoint_details(client, args.describe)
            return

        if args.collection:
            results = client.execute_collection(args.collection)
            print(json.dumps(results, indent=2))
            return

        if endpoint is None:
            raise ValueError(
                "Provide an endpoint name, or use --list / --describe / --collection"
            )

        params = parse_json_argument(args.params, "--params") if args.params else None
        headers = (
            parse_json_argument(args.headers, "--headers") if args.headers else None
        )

        request_definition = client.build_request_definition(
            endpoint,
            body_path=args.body,
            params=params,
            headers=headers,
        )

        print_request_preview(request_definition)

        if args.dry_run:
            return

        response = client.session.request(**request_definition["request_kwargs"])

    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc

    print_response(response, args.verbose)
    response_body = client.parse_response_body(response)
    response_json, response_text = emit_response_body(response_body)

    try:
        save_response(args.output, response_json, response_text)
        print(f"\nSaved response body to {args.output}")
    except Exception as e:
        print(f"\nWarning: failed to write {args.output}: {e}")

    # If token endpoint response contains access_token, persist it into .env
    if isinstance(response_json, dict) and "access_token" in response_json:
        try:
            persist_access_token(config_path, response_json["access_token"])
            print("\nUpdated access token in .env.")
        except Exception as e:
            print(f"\nWarning: failed to persist access_token to .env: {e}")


if __name__ == "__main__":
    main()
