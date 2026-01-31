
import argparse
import json
from api_client import APIClient


def main():
    parser = argparse.ArgumentParser(description='API Testing Tool')
    # First positional argument is the path the config file that should be used, defaulting to config.json
    parser.add_argument('config', nargs='?', default='config.json', help='Path to configuration file')
    parser.add_argument('endpoint', help='Endpoint name to call')
    parser.add_argument('--body', help='Path to JSON body file')
    parser.add_argument('--params', help='Query parameters as JSON string')
    parser.add_argument('--headers', help='Additional headers as JSON string')
    parser.add_argument('--collection', help='Execute collection file')
    parser.add_argument('--verbose', '-v', action='store_true', help='Verbose output')

    args = parser.parse_args()

    client = APIClient(config_path=args.config)

    if args.collection:
        results = client.execute_collection(args.collection)
        print(json.dumps(results, indent=2))
        return

    # Parse optional JSON arguments
    params = json.loads(args.params) if args.params else None
    headers = json.loads(args.headers) if args.headers else None

    # Print request (minus credentials)
    print(f"Making request using this URL/Parameters: ")

    response = client.make_request(
        args.endpoint,
        args.body,
        params,
        headers
    )

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

    # Persist response to response.json
    try:
        with open("response.json", "w") as f:
            if response_json is not None:
                json.dump(response_json, f, indent=2)
            else:
                json.dump({"raw": response_text}, f, indent=2)
            f.write("\n")
    except Exception as e:
        print(f"\nWarning: failed to write response.json: {e}")

    # If token endpoint response contains access_token, persist it back to config
    if isinstance(response_json, dict) and "access_token" in response_json:
        try:
            with open(args.config, 'r') as f:
                config = json.load(f)
            config.setdefault("auth", {})
            config["auth"]["type"] = config.get("auth", {}).get("type", "bearer")
            config["auth"]["token"] = response_json["access_token"]
            with open(args.config, 'w') as f:
                json.dump(config, f, indent=2)
                f.write("\n")
            print("\nUpdated auth token in config.")
        except Exception as e:
            print(f"\nWarning: failed to update config with access_token: {e}")


if __name__ == '__main__':
    main()
