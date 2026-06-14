import json
import os
import re
from pathlib import Path
from typing import Any, Dict, Optional, Protocol
from urllib.parse import urlencode

import requests
import yaml


ENV_PATTERN = re.compile(r"\$\{([A-Z0-9_]+)\}")


def _validation_error(document_label: str, location: str, message: str) -> None:
    raise ValueError(f"Invalid {document_label} at {location}: {message}")


def _validate_headers(value: Any, document_label: str, location: str) -> None:
    if not isinstance(value, dict):
        _validation_error(document_label, location, "must be an object")
    for key, header_value in value.items():
        if not isinstance(key, str) or not isinstance(header_value, str):
            _validation_error(
                document_label, location, "header names and values must be strings"
            )


def _validate_params(value: Any, document_label: str, location: str) -> None:
    if not isinstance(value, dict):
        _validation_error(document_label, location, "must be an object")
    scalar_types = (str, int, float, bool, type(None))
    for key, param_value in value.items():
        if not isinstance(key, str) or not isinstance(param_value, scalar_types):
            _validation_error(
                document_label,
                location,
                "parameter names must be strings and values must be scalar",
            )


def validate_api_config(document: Any) -> None:
    """Enforce the contract in schemas/api-config.schema.json."""
    label = "API config"
    if not isinstance(document, dict):
        _validation_error(label, "<root>", "must be an object")
    if "endpoints" not in document:
        _validation_error(label, "<root>", "'endpoints' is a required property")
    if not isinstance(document["endpoints"], dict):
        _validation_error(label, "endpoints", "must be an object")

    if "base_url" in document and not isinstance(document["base_url"], str):
        _validation_error(label, "base_url", "must be a string")
    if "timeout" in document:
        timeout = document["timeout"]
        if (
            isinstance(timeout, bool)
            or not isinstance(timeout, (int, float))
            or timeout <= 0
        ):
            _validation_error(label, "timeout", "must be a number greater than zero")
    if "default_headers" in document:
        _validate_headers(document["default_headers"], label, "default_headers")

    auth = document.get("auth")
    if auth is not None:
        if not isinstance(auth, dict):
            _validation_error(label, "auth", "must be an object")
        auth_type = auth.get("type")
        if auth_type == "bearer":
            required = ("token",)
        elif auth_type == "basic":
            required = ("username", "password")
        else:
            _validation_error(label, "auth.type", "must be 'bearer' or 'basic'")
        for field in required:
            if field not in auth:
                _validation_error(label, "auth", f"'{field}' is a required property")
            if not isinstance(auth[field], str):
                _validation_error(label, f"auth.{field}", "must be a string")

    for name, endpoint in document["endpoints"].items():
        location = f"endpoints.{name}"
        if not isinstance(name, str) or not isinstance(endpoint, dict):
            _validation_error(
                label, location, "endpoint names and values must be objects"
            )
        if "method" not in endpoint:
            _validation_error(label, location, "'method' is a required property")
        if not isinstance(endpoint["method"], str) or not endpoint["method"]:
            _validation_error(label, f"{location}.method", "must be a non-empty string")
        has_path = "path" in endpoint
        has_url = "url" in endpoint
        if has_path == has_url:
            _validation_error(
                label, location, "must contain exactly one of 'path' or 'url'"
            )
        target_field = "path" if has_path else "url"
        if not isinstance(endpoint[target_field], str):
            _validation_error(label, f"{location}.{target_field}", "must be a string")
        if "base_url" in endpoint and not isinstance(endpoint["base_url"], str):
            _validation_error(label, f"{location}.base_url", "must be a string")
        if "description" in endpoint and not isinstance(endpoint["description"], str):
            _validation_error(label, f"{location}.description", "must be a string")
        if "headers" in endpoint:
            _validate_headers(endpoint["headers"], label, f"{location}.headers")
        if "params" in endpoint:
            _validate_params(endpoint["params"], label, f"{location}.params")
        if endpoint.get("body_type", "json") not in {"json", "form"}:
            _validation_error(
                label, f"{location}.body_type", "must be 'json' or 'form'"
            )


def validate_collection(document: Any) -> None:
    """Enforce the contract in schemas/collection.schema.json."""
    label = "collection"
    if not isinstance(document, dict):
        _validation_error(label, "<root>", "must be an object")
    if "requests" not in document:
        _validation_error(label, "<root>", "'requests' is a required property")
    if not isinstance(document["requests"], list):
        _validation_error(label, "requests", "must be an array")

    for index, request_item in enumerate(document["requests"]):
        location = f"requests.{index}"
        if not isinstance(request_item, dict):
            _validation_error(label, location, "must be an object")
        if "endpoint" not in request_item:
            _validation_error(label, location, "'endpoint' is a required property")
        if (
            not isinstance(request_item["endpoint"], str)
            or not request_item["endpoint"]
        ):
            _validation_error(
                label, f"{location}.endpoint", "must be a non-empty string"
            )
        if "body_file" in request_item and not isinstance(
            request_item["body_file"], str
        ):
            _validation_error(label, f"{location}.body_file", "must be a string")
        if "headers" in request_item:
            _validate_headers(request_item["headers"], label, f"{location}.headers")
        if "params" in request_item:
            _validate_params(request_item["params"], label, f"{location}.params")


class ResponseLike(Protocol):
    content: bytes
    text: str

    def json(self) -> Any: ...


class APIClient:
    def __init__(self, config_path: str = "config.yaml"):
        """Initialize API client with configuration file."""
        self.config_path = Path(config_path)
        self.config = self._load_config(config_path)
        self.session = requests.Session()
        self._setup_session()

    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from YAML file."""
        with open(config_path, "r") as f:
            raw_config = yaml.safe_load(f) or {}
        validate_api_config(raw_config)
        return self._resolve_env_values(raw_config)

    def _resolve_env_values(self, value: Any) -> Any:
        """Expand ${ENV_VAR} placeholders anywhere in the config."""
        if isinstance(value, dict):
            return {key: self._resolve_env_values(item) for key, item in value.items()}
        if isinstance(value, list):
            return [self._resolve_env_values(item) for item in value]
        if isinstance(value, str):
            return ENV_PATTERN.sub(
                lambda match: os.getenv(match.group(1), match.group(0)), value
            )
        return value

    def _setup_session(self):
        """Configure session with default headers and auth."""
        if "default_headers" in self.config:
            self.session.headers.update(self.config["default_headers"])

        if "auth" in self.config:
            auth_type = self.config["auth"].get("type")
            if auth_type == "bearer":
                token = self.config["auth"]["token"]
                self.session.headers["Authorization"] = f"Bearer {token}"
            elif auth_type == "basic":
                from requests.auth import HTTPBasicAuth

                self.session.auth = HTTPBasicAuth(
                    self.config["auth"]["username"], self.config["auth"]["password"]
                )

    def list_endpoints(self) -> Dict[str, Dict[str, Any]]:
        """Return endpoint definitions keyed by name."""
        return self.config.get("endpoints", {})

    def get_endpoint(self, endpoint_name: str) -> Dict[str, Any]:
        """Get a single endpoint definition."""
        if endpoint_name not in self.config.get("endpoints", {}):
            raise ValueError(f"Endpoint '{endpoint_name}' not found in config")
        return self.config["endpoints"][endpoint_name]

    def build_request_definition(
        self,
        endpoint_name: str,
        body_path: Optional[str] = None,
        params: Optional[Dict[str, Any]] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Build a request definition without sending it."""
        endpoint = self.get_endpoint(endpoint_name)

        request_params = endpoint.get("params", {}).copy()
        if params:
            request_params.update(params)

        if "url" in endpoint:
            url = endpoint["url"]
        else:
            base_url = endpoint.get("base_url", self.config.get("base_url", ""))
            path = endpoint["path"]
            if "{" in path and "}" in path:
                path_params = {}
                for key in list(request_params.keys()):
                    token = "{" + key + "}"
                    if token in path:
                        path_params[key] = request_params.pop(key)
                for key, value in path_params.items():
                    path = path.replace("{" + key + "}", str(value))
                unresolved = re.findall(r"\{([^{}]+)\}", path)
                if unresolved:
                    names = ", ".join(unresolved)
                    raise ValueError(f"Missing path parameter values: {names}")
            url = f"{base_url}{path}"

        body = None
        if body_path:
            with open(body_path, "r") as f:
                body = json.load(f)
        elif "body" in endpoint:
            body = endpoint["body"]

        request_headers = endpoint.get("headers", {}).copy()
        if headers:
            request_headers.update(headers)
        effective_headers = dict(self.session.headers)
        effective_headers.update(request_headers)

        method = endpoint["method"].upper()
        full_url = f"{url}?{urlencode(request_params)}" if request_params else url

        request_kwargs = {
            "method": method,
            "url": url,
            "params": request_params if request_params else None,
            "headers": request_headers if request_headers else None,
            "timeout": self.config.get("timeout", 30),
        }

        if body is not None:
            if endpoint.get("body_type") == "form":
                request_kwargs["data"] = body
            else:
                request_kwargs["json"] = body

        return {
            "endpoint": endpoint_name,
            "definition": endpoint,
            "full_url": full_url,
            "request_kwargs": request_kwargs,
            "effective_headers": effective_headers,
            "body": body,
        }

    def make_request(
        self,
        endpoint_name: str,
        body_path: Optional[str] = None,
        params: Optional[Dict[str, Any]] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> requests.Response:
        """Execute API request based on endpoint configuration."""
        request_definition = self.build_request_definition(
            endpoint_name,
            body_path=body_path,
            params=params,
            headers=headers,
        )
        response = self.session.request(**request_definition["request_kwargs"])
        return response

    def parse_response_body(self, response: ResponseLike) -> Any:
        """Return JSON when possible, otherwise raw text."""
        if not response.content:
            return None
        try:
            return response.json()
        except ValueError:
            return response.text

    def execute_collection(self, collection_path: str) -> Dict[str, Any]:
        """Execute multiple requests from a YAML collection file."""
        with open(collection_path, "r") as f:
            collection = yaml.safe_load(f) or {}
        validate_collection(collection)

        results = {}
        for request_item in collection.get("requests", []):
            endpoint_name = request_item["endpoint"]
            body_path = request_item.get("body_file")
            params = request_item.get("params")
            headers = request_item.get("headers")

            try:
                response = self.make_request(endpoint_name, body_path, params, headers)
                results[endpoint_name] = {
                    "status_code": response.status_code,
                    "success": response.ok,
                    "response": self.parse_response_body(response),
                    "headers": dict(response.headers),
                }
            except Exception as e:
                results[endpoint_name] = {"success": False, "error": str(e)}

        return results
