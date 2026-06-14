import base64
import json
import os
import shlex
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs

import pytest


PROJECT_ROOT = Path(__file__).resolve().parents[2]
FIXTURES = Path(__file__).parent / "fixtures"


def cli_command():
    configured = os.getenv("API_EXPLORER_COMMAND")
    if configured:
        command = shlex.split(configured)
        executable = Path(command[0])
        if not executable.is_absolute() and os.sep in command[0]:
            command[0] = str((Path.cwd() / executable).resolve())
        return command
    return [sys.executable, str(PROJECT_ROOT / "api_cli.py")]


def run_cli(tmp_path, *arguments, env=None):
    command_env = os.environ.copy()
    command_env.update(env or {})
    return subprocess.run(
        [*cli_command(), *map(str, arguments)],
        cwd=tmp_path,
        env=command_env,
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )


@pytest.fixture
def test_server():
    received = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            received.append(
                {"method": "GET", "path": self.path, "headers": dict(self.headers)}
            )
            if self.path == "/text":
                self._respond_text("plain response")
            elif self.path == "/empty":
                self.send_response(204)
                self.end_headers()
            elif self.path == "/missing":
                self._respond_json({"error": "not found"}, status=404)
            elif self.path == "/token":
                self._respond_json({"access_token": "new-token"})
            else:
                self._respond_json({"healthy": True})

        def do_POST(self):
            content_length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(content_length)
            content_type = self.headers.get("Content-Type", "")
            parsed_body = (
                parse_qs(body.decode())
                if content_type.startswith("application/x-www-form-urlencoded")
                else json.loads(body)
            )
            received.append(
                {
                    "method": "POST",
                    "path": self.path,
                    "headers": dict(self.headers),
                    "body": parsed_body,
                }
            )
            self._respond_json({"accepted": True})

        def _respond_json(self, body, status=200):
            encoded = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        def _respond_text(self, body):
            encoded = body.encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        def log_message(self, format, *args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", received
    finally:
        server.shutdown()
        thread.join()
        server.server_close()


def compat_env(server_url="http://example.invalid"):
    return {"TEST_SERVER_URL": server_url, "COMPAT_TOKEN": "compat-secret"}


def test_dry_run_resolves_config_and_redacts_secrets(tmp_path):
    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "inspect",
        "--dry-run",
        "--params",
        '{"id":"99","page":2}',
        env=compat_env(),
    )

    assert result.returncode == 0, result.stderr
    assert "Method: POST" in result.stdout
    assert "http://example.invalid/items/99?source=config&page=2" in result.stdout
    assert '"Authorization": "<redacted>"' in result.stdout
    assert "compat-secret" not in result.stdout


def test_request_execution_matches_http_contract(tmp_path, test_server):
    server_url, received = test_server
    output_path = tmp_path / "response.json"

    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "inspect",
        "--output",
        output_path,
        env=compat_env(server_url),
    )

    assert result.returncode == 0, result.stderr
    assert "Status Code: 200" in result.stdout
    assert json.loads(output_path.read_text()) == {"accepted": True}
    assert received == [
        {
            "method": "POST",
            "path": "/items/42?source=config",
            "headers": received[0]["headers"],
            "body": {"name": "example"},
        }
    ]
    assert received[0]["headers"]["Authorization"] == "Bearer compat-secret"
    assert received[0]["headers"]["X-Default"] == "default-value"
    assert received[0]["headers"]["X-Endpoint"] == "endpoint-value"


def test_cli_overrides_headers_and_json_body(tmp_path, test_server):
    server_url, received = test_server
    body_path = tmp_path / "body.json"
    body_path.write_text('{"name":"override","count":2}')

    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "inspect",
        "--body",
        body_path,
        "--headers",
        '{"X-Endpoint":"cli-value","X-CLI":"present"}',
        env=compat_env(server_url),
    )

    assert result.returncode == 0, result.stderr
    assert received[0]["body"] == {"name": "override", "count": 2}
    assert received[0]["headers"]["X-Endpoint"] == "cli-value"
    assert received[0]["headers"]["X-CLI"] == "present"


def test_form_body_is_url_encoded(tmp_path, test_server):
    server_url, received = test_server

    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "form",
        env=compat_env(server_url),
    )

    assert result.returncode == 0, result.stderr
    assert received[0]["body"] == {"name": ["example"], "active": ["true"]}
    assert received[0]["headers"]["Content-Type"].startswith(
        "application/x-www-form-urlencoded"
    )


def test_basic_auth_header(tmp_path, test_server):
    server_url, received = test_server
    result = run_cli(
        tmp_path,
        FIXTURES / "basic-auth.yaml",
        "auth",
        env={"TEST_SERVER_URL": server_url},
    )

    expected = base64.b64encode(b"compat-user:compat-password").decode()
    assert result.returncode == 0, result.stderr
    assert received[0]["headers"]["Authorization"] == f"Basic {expected}"


def test_collection_execution_matches_output_contract(tmp_path, test_server):
    server_url, received = test_server
    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "--collection",
        FIXTURES / "collection.yaml",
        env=compat_env(server_url),
    )

    assert result.returncode == 0, result.stderr
    output = json.loads(result.stdout)
    assert output["health"]["status_code"] == 200
    assert output["health"]["success"] is True
    assert output["health"]["response"] == {"healthy": True}
    assert output["unavailable"]["success"] is False
    assert "not found in config" in output["unavailable"]["error"]
    assert len(received) == 1
    assert received[0]["method"] == "GET"
    assert received[0]["path"] == "/health"


@pytest.mark.parametrize(
    ("endpoint", "expected_stdout", "expected_file"),
    [
        ("text", "plain response", {"raw": "plain response"}),
        ("empty", "Response Body:\nnull", {"raw": None}),
        ("missing", "Status Code: 404", {"error": "not found"}),
    ],
)
def test_response_types_and_http_status_contract(
    tmp_path, test_server, endpoint, expected_stdout, expected_file
):
    server_url, _ = test_server
    output_path = tmp_path / f"{endpoint}.json"

    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        endpoint,
        "--output",
        output_path,
        env=compat_env(server_url),
    )

    # HTTP error responses are completed requests, so they do not change the CLI exit code.
    assert result.returncode == 0, result.stderr
    assert expected_stdout in result.stdout
    assert json.loads(output_path.read_text()) == expected_file


def test_access_token_updates_env_file(tmp_path, test_server):
    server_url, _ = test_server
    env_path = tmp_path / ".env"
    env_path.write_text("PERSISTED_TOKEN=old-token\nOTHER=value\n")

    result = run_cli(
        tmp_path,
        FIXTURES / "token.yaml",
        "token",
        env={"TEST_SERVER_URL": server_url},
    )

    assert result.returncode == 0, result.stderr
    assert "Updated access token in .env." in result.stdout
    assert env_path.read_text() == "PERSISTED_TOKEN=new-token\nOTHER=value\n"


def test_dotenv_values_are_loaded_without_overriding_shell(tmp_path):
    config_path = tmp_path / "dotenv.yaml"
    config_path.write_text(
        "default_headers:\n"
        "  Authorization: Bearer ${DOTENV_TOKEN}\n"
        "  X-Dotenv-Value: ${DOTENV_TOKEN}\n"
        "endpoints:\n"
        "  health:\n"
        "    method: GET\n"
        "    url: http://example.invalid/health\n"
    )
    (tmp_path / ".env").write_text("DOTENV_TOKEN=file-token\n")

    from_file = run_cli(tmp_path, config_path, "health", "--dry-run")
    from_shell = run_cli(
        tmp_path,
        config_path,
        "health",
        "--dry-run",
        env={"DOTENV_TOKEN": "shell-token"},
    )

    assert from_file.returncode == 0, from_file.stderr
    assert from_shell.returncode == 0, from_shell.stderr
    assert '"X-Dotenv-Value": "file-token"' in from_file.stdout
    assert '"X-Dotenv-Value": "shell-token"' in from_shell.stdout
    assert '"Authorization": "<redacted>"' in from_file.stdout
    assert '"Authorization": "<redacted>"' in from_shell.stdout
    assert from_file.stdout.count("<redacted>") == 1
    assert from_shell.stdout.count("<redacted>") == 1


def test_config_discovery_listing_and_description(tmp_path):
    config_dir = tmp_path / "configs"
    config_dir.mkdir()
    config_path = config_dir / "compat.yaml"
    config_path.write_text((FIXTURES / "api.yaml").read_text())

    listed_configs = run_cli(
        tmp_path, "--config-dir", config_dir, "--list-configs", env=compat_env()
    )
    listed_endpoints = run_cli(
        tmp_path, "--config-dir", config_dir, "compat", "--list", env=compat_env()
    )
    described = run_cli(
        tmp_path,
        "--config-dir",
        config_dir,
        "compat",
        "--describe",
        "health",
        env=compat_env(),
    )

    assert listed_configs.returncode == 0, listed_configs.stderr
    assert "compat" in listed_configs.stdout
    assert listed_endpoints.returncode == 0, listed_endpoints.stderr
    assert "inspect" in listed_endpoints.stdout
    assert "health" in listed_endpoints.stdout
    assert described.returncode == 0, described.stderr
    assert "Endpoint: health" in described.stdout
    assert "Description: Health check endpoint" in described.stdout
    assert "Method: GET" in described.stdout


@pytest.mark.parametrize("option", ["--params", "--headers"])
def test_malformed_json_arguments_exit_one(tmp_path, option):
    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "health",
        option,
        "{invalid",
        env=compat_env(),
    )

    assert result.returncode == 1
    assert f"Invalid JSON for {option}" in result.stderr


def test_missing_path_parameter_exits_one_before_request(tmp_path):
    config_path = tmp_path / "missing-param.yaml"
    config_path.write_text(
        "base_url: http://example.invalid\n"
        "endpoints:\n"
        "  item:\n"
        "    method: GET\n"
        "    path: /items/{id}\n"
    )

    result = run_cli(tmp_path, config_path, "item", "--dry-run")

    assert result.returncode == 1
    assert "Missing path parameter values: id" in result.stderr


def test_invalid_api_config_is_rejected_before_execution(tmp_path):
    config_path = tmp_path / "invalid.yaml"
    config_path.write_text("endpoints:\n  broken:\n    path: /missing-method\n")

    result = run_cli(tmp_path, config_path, "broken")

    assert result.returncode == 1
    assert "Invalid API config" in result.stderr
    assert "'method' is a required property" in result.stderr


def test_invalid_collection_is_rejected(tmp_path):
    collection_path = tmp_path / "invalid-collection.yaml"
    collection_path.write_text("requests:\n  - params:\n      page: 1\n")

    result = run_cli(
        tmp_path,
        FIXTURES / "api.yaml",
        "--collection",
        collection_path,
        env=compat_env(),
    )

    assert result.returncode == 1
    assert "Invalid collection" in result.stderr
    assert "'endpoint' is a required property" in result.stderr
