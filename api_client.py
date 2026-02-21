import json
import requests
from typing import Dict, Any, Optional
from urllib.parse import urlencode


class APIClient:
    def __init__(self, config_path: str = "config.json"):
        """Initialize API client with configuration file."""
        self.config = self._load_config(config_path)
        self.session = requests.Session()
        self._setup_session()

    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from JSON file."""
        with open(config_path, 'r') as f:
            return json.load(f)

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

    def make_request(
        self,
        endpoint_name: str,
        body_path: Optional[str] = None,
        params: Optional[Dict[str, Any]] = None,
        headers: Optional[Dict[str, str]] = None
    ) -> requests.Response:
        """Execute API request based on endpoint configuration."""

        if endpoint_name not in self.config['endpoints']:
            raise ValueError(f"Endpoint '{endpoint_name}' not found in config")

        endpoint = self.config['endpoints'][endpoint_name]

        # Merge query parameters
        request_params = endpoint.get('params', {}).copy()
        if params:
            request_params.update(params)

        # Build full URL (allow per-endpoint override or absolute URL)
        if 'url' in endpoint:
            url = endpoint['url']
        else:
            base_url = endpoint.get('base_url', self.config.get('base_url', ''))
            path = endpoint['path']
            # Substitute {param} in path using request_params, and remove them from query params
            if "{" in path and "}" in path:
                path_params = {}
                for key in list(request_params.keys()):
                    token = "{" + key + "}"
                    if token in path:
                        path_params[key] = request_params.pop(key)
                for key, value in path_params.items():
                    path = path.replace("{" + key + "}", str(value))
            url = f"{base_url}{path}"

        # Load request body if provided
        body = None
        if body_path:
            with open(body_path, 'r') as f:
                body = json.load(f)
        elif 'body' in endpoint:
            body = endpoint['body']

        # Merge headers
        request_headers = endpoint.get('headers', {}).copy()
        if headers:
            request_headers.update(headers)

        # Make request
        method = endpoint['method'].upper()
        # Print out the URL and parameters for debugging
        if request_params:
            full_url = f"{url}?{urlencode(request_params)}"
        else:
            full_url = url
        print("Making request to URL:", full_url)

        request_kwargs = {
            "method": method,
            "url": url,
            "params": request_params if request_params else None,
            "headers": request_headers if request_headers else None,
            "timeout": self.config.get('timeout', 30),
        }
        print("With parameters:", request_params)

        if body:
            if endpoint.get("body_type") == "form":
                request_kwargs["data"] = body
            else:
                request_kwargs["json"] = body

        response = self.session.request(**request_kwargs)

        return response

    def execute_collection(self, collection_path: str) -> Dict[str, Any]:
        """Execute multiple requests from a collection file."""
        with open(collection_path, 'r') as f:
            collection = json.load(f)

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
                results[endpoint_name] = {
                    'status_code': response.status_code,
                    'success': response.ok,
                    'response': response.json() if response.content else None,
                    'headers': dict(response.headers)
                }
            except Exception as e:
                results[endpoint_name] = {
                    'success': False,
                    'error': str(e)
                }

        return results
