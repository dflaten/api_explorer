package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func parseYAMLNode(t *testing.T, value string) *yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(value), &node); err != nil {
		t.Fatal(err)
	}
	return &node
}

func TestValidateConfigRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		message string
	}{
		{"missing endpoints", "base_url: https://example.com\n", "'endpoints' is a required property"},
		{"invalid timeout", "timeout: 0\nendpoints: {}\n", "number greater than zero"},
		{"missing bearer token", "auth:\n  type: bearer\nendpoints: {}\n", "'token' is a required property"},
		{"unsupported auth", "auth:\n  type: digest\nendpoints: {}\n", "must be 'bearer' or 'basic'"},
		{"missing method", "endpoints:\n  broken:\n    path: /x\n", "'method' is a required property"},
		{"both targets", "endpoints:\n  broken:\n    method: GET\n    path: /x\n    url: https://example.com/x\n", "exactly one of 'path' or 'url'"},
		{"invalid headers", "endpoints:\n  broken:\n    method: GET\n    path: /x\n    headers:\n      X-Count: 2\n", "header names and values must be strings"},
		{"nested params", "endpoints:\n  broken:\n    method: GET\n    path: /x\n    params:\n      filter:\n        active: true\n", "values must be scalar"},
		{"invalid body type", "endpoints:\n  broken:\n    method: POST\n    path: /x\n    body_type: xml\n", "must be 'json' or 'form'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateConfigNode(parseYAMLNode(t, test.config))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func TestLoadConfigPreservesOrderAndExpandsEnvironment(t *testing.T) {
	t.Setenv("API_ROOT", "https://example.com")
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "base_url: ${API_ROOT}\nendpoints:\n  first:\n    method: GET\n    path: /first\n  second:\n    method: GET\n    path: /second\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.BaseURL != "https://example.com" {
		t.Fatalf("unexpected base URL %q", config.BaseURL)
	}
	if strings.Join(config.EndpointOrder, ",") != "first,second" {
		t.Fatalf("unexpected endpoint order %v", config.EndpointOrder)
	}
}

func TestValidateCollectionRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name       string
		collection string
		message    string
	}{
		{"missing requests", "owner: platform\n", "'requests' is a required property"},
		{"requests is not an array", "requests: {}\n", "must be an array"},
		{"request is not an object", "requests:\n  - invalid\n", "must be an object"},
		{"missing endpoint", "requests:\n  - params:\n      page: 1\n", "'endpoint' is a required property"},
		{"empty endpoint", "requests:\n  - endpoint: ''\n", "must be a non-empty string"},
		{"invalid body file", "requests:\n  - endpoint: health\n    body_file: 2\n", "must be a string"},
		{"invalid headers", "requests:\n  - endpoint: health\n    headers:\n      X-Count: 2\n", "header names and values must be strings"},
		{"nested params", "requests:\n  - endpoint: health\n    params:\n      filter:\n        active: true\n", "values must be scalar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCollectionNode(parseYAMLNode(t, test.collection))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func TestCheckedInSchemasMatchValidationContracts(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		requiredProperty string
	}{
		{"API config", "api-config.schema.json", "endpoints"},
		{"collection", "collection.schema.json", "requests"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "schemas", test.path))
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["type"] != "object" {
				t.Fatalf("schema metadata is invalid: %#v", schema)
			}
			required, ok := schema["required"].([]any)
			if !ok || len(required) != 1 || required[0] != test.requiredProperty {
				t.Fatalf("unexpected required properties: %#v", schema["required"])
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema properties are invalid: %#v", schema["properties"])
			}
			if _, ok := properties[test.requiredProperty]; !ok {
				t.Fatalf("required property %q has no schema", test.requiredProperty)
			}
		})
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "api-config.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions struct {
			Params struct {
				AdditionalProperties struct {
					Type []string `json:"type"`
				} `json:"additionalProperties"`
			} `json:"params"`
			Endpoint struct {
				Properties struct {
					BodyType struct {
						Enum []string `json:"enum"`
					} `json:"body_type"`
				} `json:"properties"`
			} `json:"endpoint"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if strings.Join(schema.Definitions.Params.AdditionalProperties.Type, ",") != "string,number,boolean,null" {
		t.Fatalf("unexpected parameter scalar types: %#v", schema.Definitions.Params.AdditionalProperties.Type)
	}
	if strings.Join(schema.Definitions.Endpoint.Properties.BodyType.Enum, ",") != "json,form" {
		t.Fatalf("unexpected body types: %#v", schema.Definitions.Endpoint.Properties.BodyType.Enum)
	}
}

func TestWriteConfigTemplateCreatesValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs", "example.yaml")
	if err := writeConfigTemplate(path); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Endpoints["health"]; !ok {
		t.Fatal("generated config does not contain health endpoint")
	}
	if err := writeConfigTemplate(path); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestDefaultConfigPathsUseHomeConfigDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	expectedHome := filepath.Join(home, ".config", "apix")
	if defaultConfigHome() != expectedHome {
		t.Fatalf("unexpected config home: %s", defaultConfigHome())
	}
	if defaultConfigDir() != filepath.Join(expectedHome, "configs") {
		t.Fatalf("unexpected config dir: %s", defaultConfigDir())
	}
	if defaultEnvFile() != filepath.Join(expectedHome, ".env") {
		t.Fatalf("unexpected env file: %s", defaultEnvFile())
	}
}

func TestResolveInitConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDirectory := filepath.Join(home, ".config", "apix", "configs")

	path, err := resolveInitConfigPath("github", configDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(configDirectory, "github.yaml") {
		t.Fatalf("unexpected alias path: %s", path)
	}

	path, err = resolveInitConfigPath("~/custom.yaml", configDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(home, "custom.yaml") {
		t.Fatalf("unexpected explicit path: %s", path)
	}
}
