import os
import re
from copy import deepcopy
from pathlib import Path
from typing import Dict, Any, Optional
from urllib.parse import urlencode

import requests
import yaml


ENV_PATTERN = re.compile(r"\$\{([A-Z0-9_]+)\}")


class APIClient:
    def __init__(self, config_path: str = "config.yaml"):
        """Initialize API client with configuration file."""
        self.config_path = Path(config_path)
        self.config = self._load_config(config_path)
        self.session = requests.Session()
        self._setup_session()

    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from YAML file."""
        with open(config_path, 'r') as f:
            raw_config = yaml.safe_load(f) or {}
        return self._resolve_env_values(raw_config)

    def _resolve_env_values(self, value: Any) -> Any:
        """Expand ${ENV_VAR} placeholders anywhere in the config."""
        if isinstance(value, dict):
            return {key: self._resolve_env_values(item) for key, item in value.items()}
        if isinstance(value, list):
            return [self._resolve_env_values(item) for item in value]
        if isinstance(value, str):
            return ENV_PATTERN.sub(lambda match: os.getenv(match.group(1), match.group(0)), value)
        return value

    def _setup_session(self):
        """Configure session with default headers and auth."""
        if 'default_headers' in self.config:
            self.session.headers.update(self.config['default_headers'])

        if 'auth' in self.config:
            auth_type = self.config['auth'].get('type')
            if auth_type == 'bearer':
                token = self.config['auth']['token']
                self.session.headers['Authorization'] = f'Bearer {token}'
            elif auth_type == 'basic':
                from requests.auth import HTTPBasicAuth
                self.session.auth = HTTPBasicAuth(
                    self.config['auth']['username'],
                    self.config['auth']['password']
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
        headers: Optional[Dict[str, str]] = None
    ) -> Dict[str, Any]:
        """Build a request definition without sending it."""
        endpoint = deepcopy(self.get_endpoint(endpoint_name))

        request_params = endpoint.get('params', {}).copy()
        if params:
            request_params.update(params)

        if 'url' in endpoint:
            url = endpoint['url']
        else:
            base_url = endpoint.get('base_url', self.config.get('base_url', ''))
            path = endpoint['path']
            if "{" in path and "}" in path:
                path_params = {}
                for key in list(request_params.keys()):
                    token = "{" + key + "}"
                    if token in path:
                        path_params[key] = request_params.pop(key)
                for key, value in path_params.items():
                    path = path.replace("{" + key + "}", str(value))
            url = f"{base_url}{path}"

        body = None
        if body_path:
            with open(body_path, 'r') as f:
                body = json.load(f)
        elif 'body' in endpoint:
            body = endpoint['body']

        request_headers = endpoint.get('headers', {}).copy()
        if headers:
            request_headers.update(headers)
        effective_headers = dict(self.session.headers)
        effective_headers.update(request_headers)

        method = endpoint['method'].upper()
        full_url = f"{url}?{urlencode(request_params)}" if request_params else url

        request_kwargs = {
            "method": method,
            "url": url,
            "params": request_params if request_params else None,
            "headers": request_headers if request_headers else None,
            "timeout": self.config.get('timeout', 30),
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
        headers: Optional[Dict[str, str]] = None
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

    def execute_collection(self, collection_path: str) -> Dict[str, Any]:
        """Execute multiple requests from a YAML collection file."""
        with open(collection_path, 'r') as f:
            collection = yaml.safe_load(f) or {}

        results = {}
        for request in collection['requests']:
            endpoint_name = request['endpoint']
            body_path = request.get('body_file')
            params = request.get('params')
            headers = request.get('headers')

            try:
                response = self.make_request(
                    endpoint_name,
                    body_path,
                    params,
                    headers
                )
                try:
                    response_body = response.json() if response.content else None
                except ValueError:
                    response_body = response.text
                results[endpoint_name] = {
                    'status_code': response.status_code,
                    'success': response.ok,
                    'response': response_body,
                    'headers': dict(response.headers)
                }
            except Exception as e:
                results[endpoint_name] = {
                    'success': False,
                    'error': str(e)
                }

        return results
