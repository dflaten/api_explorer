package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testClient(t *testing.T) *APIClient {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `base_url: https://api.example.com
timeout: 15
default_headers:
  X-Default: present
auth:
  type: bearer
  token: secret
endpoints:
  user:
    method: GET
    path: /users/{id}
    params:
      id: "123"
      expand: teams
    headers:
      X-Trace: config
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	client, err := newAPIClient(path)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestBuildRequestMergesOverrides(t *testing.T) {
	client := testClient(t)
	params := OrderedValues{}
	params.Set("id", "456")
	params.Set("page", float64(2))
	definition, err := client.buildRequest("user", "", params, map[string]string{"X-Trace": "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if definition.FullURL != "https://api.example.com/users/456?expand=teams&page=2" {
		t.Fatalf("unexpected URL %q", definition.FullURL)
	}
	if definition.EffectiveHeaders["Authorization"] != "Bearer secret" {
		t.Fatal("bearer auth was not applied")
	}
	if definition.EffectiveHeaders["X-Trace"] != "cli" {
		t.Fatal("header override was not applied")
	}
}

func TestBuildRequestRejectsMissingPathParameter(t *testing.T) {
	client := testClient(t)
	delete(client.Config.Endpoints["user"].Params.Map(), "id")
	client.Config.Endpoints["user"].Params = OrderedValues{Entries: []ValueEntry{{Key: "expand", Value: "teams"}}}
	_, err := client.buildRequest("user", "", OrderedValues{}, nil)
	if err == nil || err.Error() != "Missing path parameter values: id" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOrderedJSONPreservesInsertionOrder(t *testing.T) {
	values, err := parseOrderedJSON(`{"first":1,"second":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(values.Entries) != 2 || values.Entries[0].Key != "first" || values.Entries[1].Key != "second" {
		t.Fatalf("unexpected entries: %#v", values.Entries)
	}
}
