
import argparse
import json
from api_client import APIClient


def main():
    parser = argparse.ArgumentParser(description='API Testing Tool')
    parser.add_argument('endpoint', help='Endpoint name to call')
    parser.add_argument('--body', help='Path to JSON body file')
    parser.add_argument('--params', help='Query parameters as JSON string')
    parser.add_argument('--headers', help='Additional headers as JSON string')
    parser.add_argument('--collection', help='Execute collection file')
    parser.add_argument('--verbose', '-v', action='store_true', help='Verbose output')

    args = parser.parse_args()

    client = APIClient()

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
    try:
        print(json.dumps(response.json(), indent=2))
    except:
        print(response.text)


if __name__ == '__main__':
    main()
