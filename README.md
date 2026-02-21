# API Testing Tool

A lightweight Python-based API testing tool to be used as an alternative to Postman. Execute HTTP requests using configuration files and JSON bodies without writing code.

## Features

- 🚀 Configuration-based endpoint management
- 🔐 Built-in authentication support (Bearer, Basic)
- 📦 Batch request execution with collections
- 🎯 Custom headers and query parameters
- 📝 JSON request bodies from files
- ⚡ Session management for connection pooling
- 🔍 Verbose output mode

## Quick Start

Create `config.json` in your project directory:

```json
{
  "base_url": "https://api.example.com",
  "timeout": 30,
  "default_headers": {
    "Content-Type": "application/json",
    "User-Agent": "API-Client/1.0"
  },
  "auth": {
    "type": "bearer",
    "token": "your_token_here"
  },
  "endpoints": {
    "get_users": {
      "method": "GET",
      "path": "/users"
    },
    "create_user": {
      "method": "POST",
      "path": "/users",
      "headers": {
        "X-Custom-Header": "value"
      }
    },
    "update_user": {
      "method": "PUT",
      "path": "/users/{id}",
      "params": {
        "notify": "true"
      }
    },
    "delete_user": {
      "method": "DELETE",
      "path": "/users/{id}"
    }
  }
}
```

Config using an api key would look like:

```json
{
  "base_url": "https://api.my_api.converge/v0",
  "timeout": 30,
  "default_headers": {
    "Content-Type": "application/json",
    "X-API-KEY": "MYKEYHERE"
  },
  "endpoints": {
    "health": {
      "method": "GET",
      "path": "/path/subpath"
    }
  }
}
```

Install dependencies with `uv`:

```bash
uv sync
```

Run a request:

```bash
uv run api-cli get_users
```

## Real Example

Using `newsapi.org`, I created a file named `newsapi_config.json` with these contents:

```json
{
  "base_url": "https://newsapi.org/v2/",
  "timeout": 30,
  "default_headers": {
    "Content-Type": "application/json",
    "X-API-KEY": "REDACTED"
  },
  "endpoints": {
    "top_headlines": {
      "method": "GET",
      "path": "top-headlines",
      "params": {
        "country": "us"
      }
    }
  }
```

Run a request:
```bash
uv run api-cli newsapi_config.json top_headlines
```

See the [documentation here](https://newsapi.org/docs/endpoints/top-headlines) for more details on the endpoint.