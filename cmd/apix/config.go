package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if err := validateConfigNode(&root); err != nil {
		return nil, err
	}
	resolveEnvNode(&root)

	var config Config
	if err := root.Decode(&config); err != nil {
		return nil, err
	}
	config.EndpointOrder = mappingKeys(mappingValue(documentValue(&root), "endpoints"))
	if config.Timeout == 0 {
		config.Timeout = 30
	}
	return &config, nil
}

func loadCollection(path string) (*Collection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if err := validateCollectionNode(&root); err != nil {
		return nil, err
	}
	resolveEnvNode(&root)
	var collection Collection
	if err := root.Decode(&collection); err != nil {
		return nil, err
	}
	return &collection, nil
}

func resolveEnvNode(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		node.Value = envPattern.ReplaceAllStringFunc(node.Value, func(match string) string {
			parts := envPattern.FindStringSubmatch(match)
			if value, ok := os.LookupEnv(parts[1]); ok {
				return value
			}
			return match
		})
	}
	for _, child := range node.Content {
		resolveEnvNode(child)
	}
}

func validateConfigNode(root *yaml.Node) error {
	document := documentValue(root)
	if document == nil || document.Kind != yaml.MappingNode {
		return invalid("API config", "<root>", "must be an object")
	}
	endpoints := mappingValue(document, "endpoints")
	if endpoints == nil {
		return invalid("API config", "<root>", "'endpoints' is a required property")
	}
	if endpoints.Kind != yaml.MappingNode {
		return invalid("API config", "endpoints", "must be an object")
	}
	if timeout := mappingValue(document, "timeout"); timeout != nil {
		var value float64
		if timeout.Decode(&value) != nil || value <= 0 || timeout.Tag == "!!bool" {
			return invalid("API config", "timeout", "must be a number greater than zero")
		}
	}
	if baseURL := mappingValue(document, "base_url"); baseURL != nil && baseURL.Tag != "!!str" {
		return invalid("API config", "base_url", "must be a string")
	}
	if headers := mappingValue(document, "default_headers"); headers != nil {
		if err := validateHeadersNode(headers, "API config", "default_headers"); err != nil {
			return err
		}
	}
	if auth := mappingValue(document, "auth"); auth != nil {
		if auth.Kind != yaml.MappingNode {
			return invalid("API config", "auth", "must be an object")
		}
		authType := mappingValue(auth, "type")
		if authType == nil || (authType.Value != "bearer" && authType.Value != "basic") {
			return invalid("API config", "auth.type", "must be 'bearer' or 'basic'")
		}
		required := []string{"token"}
		if authType.Value == "basic" {
			required = []string{"username", "password"}
		}
		for _, field := range required {
			value := mappingValue(auth, field)
			if value == nil {
				return invalid("API config", "auth", fmt.Sprintf("'%s' is a required property", field))
			}
			if value.Tag != "!!str" {
				return invalid("API config", "auth."+field, "must be a string")
			}
		}
	}

	for index := 0; index < len(endpoints.Content); index += 2 {
		nameNode := endpoints.Content[index]
		name := nameNode.Value
		endpoint := endpoints.Content[index+1]
		location := "endpoints." + name
		if nameNode.Tag != "!!str" || endpoint.Kind != yaml.MappingNode {
			return invalid("API config", location, "endpoint names and values must be objects")
		}
		method := mappingValue(endpoint, "method")
		if method == nil {
			return invalid("API config", location, "'method' is a required property")
		}
		if method.Tag != "!!str" || method.Value == "" {
			return invalid("API config", location+".method", "must be a non-empty string")
		}
		path, absoluteURL := mappingValue(endpoint, "path"), mappingValue(endpoint, "url")
		if (path == nil) == (absoluteURL == nil) {
			return invalid("API config", location, "must contain exactly one of 'path' or 'url'")
		}
		target, field := path, "path"
		if target == nil {
			target, field = absoluteURL, "url"
		}
		if target.Tag != "!!str" {
			return invalid("API config", location+"."+field, "must be a string")
		}
		if baseURL := mappingValue(endpoint, "base_url"); baseURL != nil && baseURL.Tag != "!!str" {
			return invalid("API config", location+".base_url", "must be a string")
		}
		if description := mappingValue(endpoint, "description"); description != nil && description.Tag != "!!str" {
			return invalid("API config", location+".description", "must be a string")
		}
		if headers := mappingValue(endpoint, "headers"); headers != nil {
			if err := validateHeadersNode(headers, "API config", location+".headers"); err != nil {
				return err
			}
		}
		if params := mappingValue(endpoint, "params"); params != nil {
			if err := validateParamsNode(params, "API config", location+".params"); err != nil {
				return err
			}
		}
		if bodyType := mappingValue(endpoint, "body_type"); bodyType != nil && bodyType.Value != "json" && bodyType.Value != "form" {
			return invalid("API config", location+".body_type", "must be 'json' or 'form'")
		}
	}
	return nil
}

func validateCollectionNode(root *yaml.Node) error {
	document := documentValue(root)
	if document == nil || document.Kind != yaml.MappingNode {
		return invalid("collection", "<root>", "must be an object")
	}
	requests := mappingValue(document, "requests")
	if requests == nil {
		return invalid("collection", "<root>", "'requests' is a required property")
	}
	if requests.Kind != yaml.SequenceNode {
		return invalid("collection", "requests", "must be an array")
	}
	for index, request := range requests.Content {
		location := fmt.Sprintf("requests.%d", index)
		if request.Kind != yaml.MappingNode {
			return invalid("collection", location, "must be an object")
		}
		endpoint := mappingValue(request, "endpoint")
		if endpoint == nil {
			return invalid("collection", location, "'endpoint' is a required property")
		}
		if endpoint.Tag != "!!str" || endpoint.Value == "" {
			return invalid("collection", location+".endpoint", "must be a non-empty string")
		}
		if bodyFile := mappingValue(request, "body_file"); bodyFile != nil && bodyFile.Tag != "!!str" {
			return invalid("collection", location+".body_file", "must be a string")
		}
		if headers := mappingValue(request, "headers"); headers != nil {
			if err := validateHeadersNode(headers, "collection", location+".headers"); err != nil {
				return err
			}
		}
		if params := mappingValue(request, "params"); params != nil {
			if err := validateParamsNode(params, "collection", location+".params"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHeadersNode(node *yaml.Node, label, location string) error {
	if node.Kind != yaml.MappingNode {
		return invalid(label, location, "must be an object")
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Tag != "!!str" || node.Content[index+1].Tag != "!!str" {
			return invalid(label, location, "header names and values must be strings")
		}
	}
	return nil
}

func validateParamsNode(node *yaml.Node, label, location string) error {
	if node.Kind != yaml.MappingNode {
		return invalid(label, location, "must be an object")
	}
	for index := 0; index < len(node.Content); index += 2 {
		value := node.Content[index+1]
		if node.Content[index].Tag != "!!str" || value.Kind != yaml.ScalarNode {
			return invalid(label, location, "parameter names must be strings and values must be scalar")
		}
	}
	return nil
}

func invalid(label, location, message string) error {
	return fmt.Errorf("invalid %s at %s: %s", label, location, message)
}

func documentValue(root *yaml.Node) *yaml.Node {
	if root != nil && root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		keys = append(keys, node.Content[index].Value)
	}
	return keys
}

func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if len(value) >= 2 && value[0] == value[len(value)-1] && (value[0] == '\'' || value[0] == '"') {
			value = value[1 : len(value)-1]
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
