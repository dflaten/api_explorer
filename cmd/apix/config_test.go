package main

import (
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

func TestValidateCollectionRejectsMissingEndpoint(t *testing.T) {
	err := validateCollectionNode(parseYAMLNode(t, "requests:\n  - params:\n      page: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "'endpoint' is a required property") {
		t.Fatalf("unexpected error: %v", err)
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
	if err := writeConfigTemplate(path); err == nil || !strings.Contains(err.Error(), "Refusing to overwrite") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}
