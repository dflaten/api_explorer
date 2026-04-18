import argparse
import json
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
            raise ValueError("Collection mode accepts at most one positional argument: [config]")
        config_path = resolve_config_path(targets[0], config_dir) if targets else "config.yaml"
        return config_path, None

    if not targets:
        raise ValueError("Provide an endpoint name, or use --list / --describe / --init-config")

    if len(targets) == 1:
        return "config.yaml", targets[0]

    if len(targets) == 2:
        return resolve_config_path(targets[0], config_dir), targets[1]

    raise ValueError("Too many positional arguments. Use [endpoint] or [config endpoint]")


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
        print(json.dumps(redact_headers(request_definition["effective_headers"]), indent=2))
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


def redact_headers(headers):
    redacted = {}
    for key, value in headers.items():
        if key.lower() in SENSITIVE_HEADER_NAMES:
            redacted[key] = "<redacted>"
        else:
            redacted[key] = value
    return redacted


def update_auth_token(config_path: str, access_token: str):
    with open(config_path, 'r') as f:
        config = yaml.safe_load(f) or {}
    config.setdefault("auth", {})
    config["auth"]["type"] = config.get("auth", {}).get("type", "bearer")
    config["auth"]["token"] = access_token

    with open(config_path, 'w') as f:
        yaml.safe_dump(config, f, sort_keys=False)


def main():
    parser = argparse.ArgumentParser(description='API Testing Tool')
    parser.add_argument('targets', nargs='*', help='Use [endpoint], [config endpoint], or [config_alias endpoint]')
    parser.add_argument('--body', help='Path to JSON body file')
    parser.add_argument('--params', help='Query parameters as JSON string')
    parser.add_argument('--headers', help='Additional headers as JSON string')
    parser.add_argument('--collection', help='Execute collection file')
    parser.add_argument('--config-dir', default='configs', help='Directory used for YAML config aliases')
    parser.add_argument('--list-configs', action='store_true', help='List config files in the config directory')
    parser.add_argument('--list', action='store_true', help='List configured endpoints')
    parser.add_argument('--describe', metavar='ENDPOINT', help='Show endpoint details without executing')
    parser.add_argument('--dry-run', action='store_true', help='Build and print the request without sending it')
    parser.add_argument('--init-config', metavar='PATH', help='Create a starter YAML config file for a new API')
    parser.add_argument('--output', default='response.json', help='Path to save the response body')
    parser.add_argument('--verbose', '-v', action='store_true', help='Verbose output')

    parse_args = getattr(parser, "parse_intermixed_args", parser.parse_args)
    args = parse_args()

    try:
        if args.init_config:
            write_config_template(args.init_config)
            print(f"Created starter config at {args.init_config}")
            print("Replace placeholders like ${API_TOKEN} with environment variables before calling the API.")
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
                raise ValueError("Use --describe ENDPOINT with an optional [config] positional argument")
            config_path = resolve_config_path(args.targets[0], args.config_dir) if args.targets else "config.yaml"
            endpoint = None
        else:
            config_path, endpoint = resolve_config_and_endpoint(args.targets, args.collection, args.config_dir)

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

        params = parse_json_argument(args.params, "--params") if args.params else None
        headers = parse_json_argument(args.headers, "--headers") if args.headers else None

        request_definition = client.build_request_definition(
            endpoint,
            body_path=args.body,
            params=params,
            headers=headers,
        )

        print_request_preview(request_definition)

        if args.dry_run:
            return

        response = client.make_request(
            endpoint,
            args.body,
            params,
            headers
        )

    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc

    # Display results
    print(f"Status Code: {response.status_code}")
    print(f"Success: {response.ok}")

    if args.verbose:
        print(f"\nResponse Headers:")
        print(json.dumps(dict(response.headers), indent=2))

    print(f"\nResponse Body:")
    response_text = None
    try:
        response_json = response.json()
        print(json.dumps(response_json, indent=2))
    except Exception:
        response_json = None
        response_text = response.text
        print(response_text)

    try:
        save_response(args.output, response_json, response_text)
        print(f"\nSaved response body to {args.output}")
    except Exception as e:
        print(f"\nWarning: failed to write {args.output}: {e}")

    # If token endpoint response contains access_token, persist it back to config
    if isinstance(response_json, dict) and "access_token" in response_json:
        try:
            update_auth_token(config_path, response_json["access_token"])
            print("\nUpdated auth token in config.")
        except Exception as e:
            print(f"\nWarning: failed to update config with access_token: {e}")


if __name__ == '__main__':
    main()
