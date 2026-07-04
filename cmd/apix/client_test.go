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
	client.Config.Endpoints["user"].Params = OrderedValues{Entries: []ValueEntry{{Key: "expand", Value: "teams"}}}
	_, err := client.buildRequest("user", "", OrderedValues{}, nil)
	if err == nil || err.Error() != "missing path parameter values: id" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildRequestEscapesPathParameters(t *testing.T) {
	client := testClient(t)
	params := OrderedValues{}
	params.Set("id", "a/b c")

	definition, err := client.buildRequest("user", "", params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition.FullURL != "https://api.example.com/users/a%2Fb%20c?expand=teams" {
		t.Fatalf("unexpected URL %q", definition.FullURL)
	}
}

func TestScalarStringUsesHTTPFriendlyBooleansAndNull(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{true, "true"},
		{false, "false"},
		{nil, "null"},
	}
	for _, test := range tests {
		if got := scalarString(test.value); got != test.want {
			t.Fatalf("scalarString(%#v) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestBuildRequestPreservesExistingQueryParameters(t *testing.T) {
	client := testClient(t)
	requestURL := "https://api.example.com/users?fixed=1#results"
	client.Config.Endpoints["absolute"] = &Endpoint{
		Method: "GET",
		URL:    &requestURL,
		Params: OrderedValues{Entries: []ValueEntry{{Key: "page", Value: float64(2)}}},
	}

	definition, err := client.buildRequest("absolute", "", OrderedValues{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition.FullURL != "https://api.example.com/users?fixed=1&page=2#results" {
		t.Fatalf("unexpected URL %q", definition.FullURL)
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

func TestParseOrderedJSONRejectsTrailingInput(t *testing.T) {
	for _, value := range []string{`{"page":1} garbage`, `{"page":1} {`, `{"page":1} 2`} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseOrderedJSON(value); err == nil {
				t.Fatalf("expected trailing input in %q to fail", value)
			}
		})
	}
}
