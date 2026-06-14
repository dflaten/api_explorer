import json
from pathlib import Path

import pytest

from api_client import validate_api_config, validate_collection


SCHEMA_DIR = Path(__file__).resolve().parents[1] / "schemas"


def valid_config():
    return {
        "base_url": "https://api.example.com",
        "timeout": 10,
        "auth": {"type": "bearer", "token": "${API_TOKEN}"},
        "endpoints": {
            "health": {"method": "GET", "path": "/health"},
            "absolute": {"method": "POST", "url": "https://other.example.com"},
        },
    }


@pytest.mark.parametrize(
    ("mutate", "message"),
    [
        (lambda config: config.pop("endpoints"), "'endpoints' is a required property"),
        (lambda config: config.update(timeout=0), "number greater than zero"),
        (
            lambda config: config.update(auth={"type": "bearer"}),
            "'token' is a required property",
        ),
        (
            lambda config: config.update(auth={"type": "digest"}),
            "must be 'bearer' or 'basic'",
        ),
        (
            lambda config: config["endpoints"]["health"].pop("method"),
            "'method' is a required property",
        ),
        (
            lambda config: config["endpoints"]["health"].update(
                url="https://api.example.com/health"
            ),
            "exactly one of 'path' or 'url'",
        ),
        (
            lambda config: config["endpoints"]["health"].update(body_type="xml"),
            "must be 'json' or 'form'",
        ),
        (
            lambda config: config["endpoints"]["health"].update(headers={"X-Count": 2}),
            "header names and values must be strings",
        ),
        (
            lambda config: config["endpoints"]["health"].update(
                params={"filter": {"active": True}}
            ),
            "values must be scalar",
        ),
    ],
)
def test_invalid_api_config_contract(mutate, message):
    config = valid_config()
    mutate(config)

    with pytest.raises(ValueError, match=message):
        validate_api_config(config)


def test_valid_api_config_allows_unknown_metadata():
    config = valid_config()
    config["owner"] = "platform-team"
    config["endpoints"]["health"]["tags"] = ["smoke"]

    validate_api_config(config)


@pytest.mark.parametrize(
    ("collection", "message"),
    [
        ({}, "'requests' is a required property"),
        ({"requests": {}}, "must be an array"),
        ({"requests": [{}]}, "'endpoint' is a required property"),
        ({"requests": [{"endpoint": ""}]}, "must be a non-empty string"),
        (
            {"requests": [{"endpoint": "health", "headers": {"X-Count": 2}}]},
            "header names and values must be strings",
        ),
    ],
)
def test_invalid_collection_contract(collection, message):
    with pytest.raises(ValueError, match=message):
        validate_collection(collection)


def test_checked_in_schemas_are_draft_2020_12_documents():
    for schema_path in SCHEMA_DIR.glob("*.schema.json"):
        schema = json.loads(schema_path.read_text())
        assert schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
        assert schema["type"] == "object"
