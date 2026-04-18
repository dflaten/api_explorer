import json

import pytest
import yaml

from api_client import APIClient


class FakeResponse:
    def __init__(
        self, *, status_code=200, ok=True, json_data=None, text="", headers=None
    ):
        self.status_code = status_code
        self.ok = ok
        self._json_data = json_data
        self.text = text
        self.headers = headers or {}
        if json_data is not None:
            self.content = json.dumps(json_data).encode("utf-8")
        else:
            self.content = text.encode("utf-8")

    def json(self):
        if self._json_data is None:
            raise ValueError("not json")
        return self._json_data


@pytest.fixture
def config_file(tmp_path):
    config_path = tmp_path / "example.yaml"
    config_path.write_text(
        yaml.safe_dump(
            {
                "base_url": "https://api.example.com",
                "timeout": 15,
                "default_headers": {
                    "Content-Type": "application/json",
                },
                "auth": {
                    "type": "bearer",
                    "token": "${API_TOKEN}",
                },
                "endpoints": {
                    "get_user": {
                        "method": "GET",
                        "path": "/users/{id}",
                        "params": {"id": "123", "expand": "teams"},
                        "headers": {"X-Trace": "config"},
                    },
                    "create_user": {
                        "method": "POST",
                        "path": "/users",
                        "body": {"name": "Ada"},
                    },
                },
            },
            sort_keys=False,
        )
    )
    return config_path


def test_build_request_definition_resolves_env_path_params_and_headers(
    config_file, monkeypatch
):
    monkeypatch.setenv("API_TOKEN", "secret-token")
    client = APIClient(str(config_file))

    request = client.build_request_definition(
        "get_user",
        params={"id": "456", "page": 2},
        headers={"X-Trace": "cli"},
    )

    assert (
        request["full_url"] == "https://api.example.com/users/456?expand=teams&page=2"
    )
    assert request["request_kwargs"]["params"] == {"expand": "teams", "page": 2}
    assert request["request_kwargs"]["timeout"] == 15
    assert request["effective_headers"]["Authorization"] == "Bearer secret-token"
    assert request["effective_headers"]["X-Trace"] == "cli"


def test_parse_response_body_handles_json_and_text(config_file):
    client = APIClient(str(config_file))

    assert client.parse_response_body(FakeResponse(json_data={"ok": True})) == {
        "ok": True
    }
    assert client.parse_response_body(FakeResponse(text="plain text")) == "plain text"
    assert client.parse_response_body(FakeResponse(text="")) is None


def test_execute_collection_aggregates_results(config_file, tmp_path, monkeypatch):
    collection_path = tmp_path / "collection.yaml"
    collection_path.write_text(
        yaml.safe_dump(
            {
                "requests": [
                    {"endpoint": "get_user"},
                    {"endpoint": "create_user"},
                ]
            },
            sort_keys=False,
        )
    )

    client = APIClient(str(config_file))
    responses = {
        "get_user": FakeResponse(
            json_data={"id": "123"}, headers={"Content-Type": "application/json"}
        ),
        "create_user": FakeResponse(
            text="created", headers={"Content-Type": "text/plain"}
        ),
    }

    def fake_make_request(endpoint_name, body_path=None, params=None, headers=None):
        return responses[endpoint_name]

    monkeypatch.setattr(client, "make_request", fake_make_request)

    results = client.execute_collection(str(collection_path))

    assert results["get_user"]["success"] is True
    assert results["get_user"]["response"] == {"id": "123"}
    assert results["create_user"]["response"] == "created"
