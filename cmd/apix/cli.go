package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type options struct {
	Command        string
	Targets        []string
	Body           string
	Params         string
	Headers        string
	ConfigDir      string
	RequestPreview bool
	Output         string
	OutputSet      bool
	Verbose        bool
	Help           bool
	Version        bool
}

var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"api-key":             true,
}

var envTokenPattern = regexp.MustCompile(`^\$\{([A-Z0-9_]+)\}$`)

func parseOptions(arguments []string) (options, error) {
	result := options{Output: "response.json"}
	valueOptions := map[string]*string{
		"--body": &result.Body, "--params": &result.Params, "--headers": &result.Headers,
		"--config-dir": &result.ConfigDir, "--output": &result.Output,
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if target, ok := valueOptions[argument]; ok {
			index++
			if index >= len(arguments) {
				return result, fmt.Errorf("argument %s: expected one argument", argument)
			}
			*target = arguments[index]
			if argument == "--output" {
				result.OutputSet = true
			}
			continue
		}
		switch argument {
		case "--help", "-h":
			result.Help = true
		case "--version":
			result.Version = true
		case "--verbose", "-v":
			result.Verbose = true
		default:
			if strings.HasPrefix(argument, "-") {
				return result, fmt.Errorf("unrecognized arguments: %s", argument)
			}
			if result.Command == "" && len(result.Targets) == 0 && isCommand(argument) {
				result.Command = argument
			} else {
				result.Targets = append(result.Targets, argument)
			}
		}
	}
	if result.Command == "" && len(result.Targets) > 0 {
		result.Command = "run"
	}
	return result, nil
}

func isCommand(argument string) bool {
	switch argument {
	case "init", "configs", "list", "describe", "preview", "run", "collection":
		return true
	default:
		return false
	}
}

func run(arguments []string) error {
	options, err := parseOptions(arguments)
	if err != nil {
		return err
	}
	if options.Help {
		printHelp()
		return nil
	}
	if options.Version {
		printVersion()
		return nil
	}
	if err := setDefaultPaths(&options); err != nil {
		return err
	}
	if err := loadEnvFile(defaultEnvFile()); err != nil {
		return err
	}
	switch options.Command {
	case "init":
		if err := rejectRequestOptions(options, "init"); err != nil {
			return err
		}
		if len(options.Targets) != 1 {
			return fmt.Errorf("usage: apix init NAME|PATH")
		}
		path, err := resolveInitConfigPath(options.Targets[0], options.ConfigDir)
		if err != nil {
			return err
		}
		if err := writeConfigTemplate(path); err != nil {
			return err
		}
		fmt.Printf("Created starter config at %s\n", path)
		fmt.Println("Replace placeholders like ${API_TOKEN} with environment variables before calling the API.")
		return nil
	case "configs":
		if err := rejectRequestOptions(options, "configs"); err != nil {
			return err
		}
		if len(options.Targets) != 0 {
			return fmt.Errorf("usage: apix configs")
		}
		return printConfigList(options.ConfigDir)
	case "list":
		if err := rejectRequestOptions(options, "list"); err != nil {
			return err
		}
		configPath, err := resolveConfigTarget(options.Targets, options.ConfigDir)
		if err != nil {
			return err
		}
		client, err := newAPIClient(configPath)
		if err != nil {
			return err
		}
		printEndpointList(client)
		return nil
	case "describe":
		if err := rejectRequestOptions(options, "describe"); err != nil {
			return err
		}
		configPath, endpointName, err := resolveEndpointTargets(options.Targets, options.ConfigDir, "describe")
		if err != nil {
			return err
		}
		client, err := newAPIClient(configPath)
		if err != nil {
			return err
		}
		return printEndpointDetails(client, endpointName)
	case "preview":
		if err := rejectResponseOptions(options, "preview"); err != nil {
			return err
		}
		options.RequestPreview = true
		return executeEndpoint(options, "preview")
	case "run":
		return executeEndpoint(options, "run")
	case "collection":
		if err := rejectRequestOptions(options, "collection"); err != nil {
			return err
		}
		configPath, collectionPath, err := resolveCollectionTargets(options.Targets, options.ConfigDir)
		if err != nil {
			return err
		}
		client, err := newAPIClient(configPath)
		if err != nil {
			return err
		}
		results, err := client.executeCollection(collectionPath)
		if err != nil {
			return err
		}
		return printJSON(results)
	case "":
		return fmt.Errorf("usage: apix COMMAND [arguments]")
	default:
		return fmt.Errorf("unknown command: %s", options.Command)
	}
}

func executeEndpoint(options options, command string) error {
	configPath, endpointName, err := resolveEndpointTargets(options.Targets, options.ConfigDir, command)
	if err != nil {
		return err
	}
	client, err := newAPIClient(configPath)
	if err != nil {
		return err
	}
	parameters := OrderedValues{}
	if options.Params != "" {
		parameters, err = parseOrderedJSON(options.Params)
		if err != nil {
			return fmt.Errorf("invalid JSON for --params: %w", err)
		}
	}
	headers := map[string]string{}
	if options.Headers != "" {
		parsed, parseErr := parseOrderedJSON(options.Headers)
		if parseErr != nil {
			return fmt.Errorf("invalid JSON for --headers: %w", parseErr)
		}
		for _, entry := range parsed.Entries {
			value, ok := entry.Value.(string)
			if !ok {
				return fmt.Errorf("invalid JSON for --headers: header values must be strings")
			}
			headers[entry.Key] = value
		}
	}
	definition, err := client.buildRequest(endpointName, options.Body, parameters, headers)
	if err != nil {
		return err
	}
	printRequestPreview(definition)
	if options.RequestPreview {
		return nil
	}
	response, err := client.execute(definition)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	parsedBody, _, err := parseResponse(response)
	if err != nil {
		return err
	}
	printResponse(response.StatusCode, response.StatusCode >= 200 && response.StatusCode < 400, responseHeaders(response.Header), options.Verbose)
	responseJSON, responseText := emitResponseBody(parsedBody)
	if err := saveResponse(options.Output, responseJSON, responseText); err != nil {
		fmt.Printf("\nWarning: failed to write %s: %s\n", options.Output, err)
	} else {
		fmt.Printf("\nSaved response body to %s\n", options.Output)
	}
	if object, ok := responseJSON.(map[string]any); ok {
		if token, ok := object["access_token"].(string); ok {
			envPath := defaultEnvFile()
			if err := persistAccessToken(configPath, token, envPath); err != nil {
				fmt.Printf("\nWarning: failed to persist access_token to %s: %s\n", envPath, err)
			} else {
				fmt.Printf("\nUpdated access token in %s.\n", envPath)
			}
		}
	}
	return nil
}

func rejectRequestOptions(options options, command string) error {
	if options.Body != "" || options.Params != "" || options.Headers != "" || options.OutputSet || options.Verbose {
		return fmt.Errorf("request options are not valid for %s", command)
	}
	return nil
}

func rejectResponseOptions(options options, command string) error {
	if options.OutputSet || options.Verbose {
		return fmt.Errorf("response options are not valid for %s", command)
	}
	return nil
}

func resolveConfigTarget(targets []string, configDirectory string) (string, error) {
	if len(targets) == 0 {
		return defaultConfigFile(), nil
	}
	if len(targets) == 1 {
		return resolveConfigPath(targets[0], configDirectory)
	}
	return "", fmt.Errorf("usage: apix list [config]")
}

func resolveEndpointTargets(targets []string, configDirectory, command string) (string, string, error) {
	if len(targets) == 1 {
		return defaultConfigFile(), targets[0], nil
	}
	if len(targets) == 2 {
		path, err := resolveConfigPath(targets[0], configDirectory)
		return path, targets[1], err
	}
	return "", "", fmt.Errorf("usage: apix %s [config] endpoint", command)
}

func resolveCollectionTargets(targets []string, configDirectory string) (string, string, error) {
	if len(targets) == 1 {
		return defaultConfigFile(), targets[0], nil
	}
	if len(targets) == 2 {
		path, err := resolveConfigPath(targets[0], configDirectory)
		return path, targets[1], err
	}
	return "", "", fmt.Errorf("usage: apix collection [config] PATH")
}

func resolveConfigPath(specification, configDirectory string) (string, error) {
	path, err := expandHome(specification)
	if err != nil {
		return "", err
	}
	if fileExists(path) {
		return path, nil
	}
	candidates := []string{filepath.Join(configDirectory, specification), filepath.Join(configDirectory, specification+".yaml"), filepath.Join(configDirectory, specification+".yml")}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if isExplicitConfigPath(specification) {
		return path, nil
	}
	return "", fmt.Errorf("unknown config alias %q; use 'apix configs' to see available configs", specification)
}

func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }

func setDefaultPaths(options *options) error {
	if options.ConfigDir == "" {
		options.ConfigDir = defaultConfigDir()
		return nil
	}
	path, err := expandHome(options.ConfigDir)
	if err != nil {
		return err
	}
	options.ConfigDir = path
	return nil
}

func defaultConfigHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "apix")
	}
	return filepath.Join(home, ".config", "apix")
}

func defaultConfigDir() string { return filepath.Join(defaultConfigHome(), "configs") }

func defaultConfigFile() string { return filepath.Join(defaultConfigHome(), "config.yaml") }

func defaultEnvFile() string { return filepath.Join(defaultConfigHome(), ".env") }

func resolveInitConfigPath(specification, configDirectory string) (string, error) {
	path, err := expandHome(specification)
	if err != nil {
		return "", err
	}
	if isExplicitConfigPath(specification) {
		return path, nil
	}
	return filepath.Join(configDirectory, specification+".yaml"), nil
}

func isExplicitConfigPath(specification string) bool {
	extension := filepath.Ext(specification)
	return extension == ".yaml" || extension == ".yml" || strings.ContainsRune(specification, os.PathSeparator)
}

func expandHome(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func printConfigList(directory string) error {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		fmt.Printf("Config directory: %s\nNo config files found.\n", directory)
		return nil
	}
	if err != nil {
		return err
	}
	paths := []string{}
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		if !entry.IsDir() && (extension == ".yaml" || extension == ".yml") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	fmt.Printf("Config directory: %s\n", directory)
	if len(paths) == 0 {
		fmt.Println("No config files found.")
		return nil
	}
	fmt.Println("Available configs:")
	for _, path := range paths {
		fmt.Printf("  %-20s %s\n", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), path)
	}
	return nil
}

func printEndpointList(client *APIClient) {
	if len(client.Config.EndpointOrder) == 0 {
		fmt.Println("No endpoints found in config.")
		return
	}
	fmt.Printf("Config: %s\nAvailable endpoints:\n", client.ConfigPath)
	for _, name := range client.Config.EndpointOrder {
		endpoint := client.Config.Endpoints[name]
		target := ""
		if endpoint.Path != nil {
			target = *endpoint.Path
		} else if endpoint.URL != nil {
			target = *endpoint.URL
		}
		line := fmt.Sprintf("  %-20s %-6s %s", name, strings.ToUpper(endpoint.Method), target)
		if endpoint.Description != "" {
			line += "  - " + endpoint.Description
		}
		fmt.Println(line)
	}
}

func printEndpointDetails(client *APIClient, name string) error {
	definition, err := client.buildRequest(name, "", OrderedValues{}, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Endpoint: %s\n", name)
	if definition.Definition.Description != "" {
		fmt.Printf("Description: %s\n", definition.Definition.Description)
	}
	fmt.Printf("Method: %s\nURL: %s\n", definition.Method, definition.FullURL)
	if len(definition.Definition.Headers) > 0 {
		fmt.Println("\nEndpoint Headers:")
		printJSON(definition.Definition.Headers)
	}
	if definition.Params.Len() > 0 {
		fmt.Println("\nDefault Query Parameters:")
		printJSON(definition.Params.Map())
	}
	if definition.Body != nil {
		fmt.Println("\nDefault Body:")
		printJSON(definition.Body)
	}
	return nil
}

func printRequestPreview(definition *RequestDefinition) {
	fmt.Printf("Request Preview:\n  Method: %s\n  URL: %s\n  Timeout: %gs\n", definition.Method, definition.FullURL, definition.Timeout)
	if len(definition.EffectiveHeaders) > 0 {
		fmt.Println("  Headers:")
		printJSON(redactHeaders(definition.EffectiveHeaders))
	}
	if definition.Params.Len() > 0 {
		fmt.Println("  Query Params:")
		printJSON(definition.Params.Map())
	}
	if definition.Body != nil {
		if definition.BodyType == "form" {
			fmt.Println("  Form Body:")
		} else {
			fmt.Println("  JSON Body:")
		}
		printJSON(definition.Body)
	}
}

func redactHeaders(headers map[string]string) map[string]string {
	result := cloneHeaders(headers)
	for key := range result {
		if sensitiveHeaders[strings.ToLower(key)] {
			result[key] = "<redacted>"
		}
	}
	return result
}

func printResponse(statusCode int, success bool, headers map[string]string, verbose bool) {
	fmt.Printf("Status Code: %d\nSuccess: %t\n", statusCode, success)
	if verbose {
		fmt.Println("\nResponse Headers:")
		printJSON(headers)
	}
}

func emitResponseBody(body any) (any, *string) {
	fmt.Println("\nResponse Body:")
	if body == nil {
		fmt.Println("null")
		return nil, nil
	}
	switch body.(type) {
	case map[string]any, []any:
		printJSON(body)
		return body, nil
	default:
		text := fmt.Sprint(body)
		fmt.Println(text)
		return nil, &text
	}
}

func printJSON(value any) error {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	fmt.Print(output.String())
	return nil
}

func saveResponse(path string, responseJSON any, responseText *string) error {
	value := responseJSON
	if responseJSON == nil {
		var raw any
		if responseText != nil {
			raw = *responseText
		}
		value = map[string]any{"raw": raw}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func persistAccessToken(configPath, token, envPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var raw struct {
		Auth struct {
			Token any `yaml:"token"`
		} `yaml:"auth"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	tokenValue, ok := raw.Auth.Token.(string)
	if !ok {
		return fmt.Errorf("Config auth token is missing or not a string")
	}
	match := envTokenPattern.FindStringSubmatch(tokenValue)
	if match == nil {
		return fmt.Errorf("Config auth token must use ${ENV_VAR} syntax to persist access_token into the env file")
	}
	return updateEnvValue(envPath, match[1], token)
}

func updateEnvValue(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(data) == 0 {
		lines = nil
	}
	found := false
	for index, line := range lines {
		content := strings.TrimSpace(line)
		if strings.HasPrefix(content, "export ") {
			content = strings.TrimSpace(strings.TrimPrefix(content, "export "))
		}
		existingKey, _, hasValue := strings.Cut(content, "=")
		if hasValue && strings.TrimSpace(existingKey) == key {
			lines[index] = key + "=" + value
			found = true
		}
	}
	if !found {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, key+"="+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func writeConfigTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing file: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0o644)
}

func printHelp() {
	fmt.Print(`usage: apix [global options] COMMAND [arguments]
       apix [global options] [config] endpoint [request options]

CLI API explorer for YAML-defined API configs.

Commands:
  init NAME|PATH               Create a starter YAML config
  configs                      List available config aliases
  list [config]                List configured endpoints
  describe [config] endpoint   Describe an endpoint without executing it
  preview [config] endpoint    Preview a request without sending it
  run [config] endpoint        Execute an endpoint
  collection [config] PATH     Execute a YAML request collection

Request options:
  --body PATH             Path to a JSON body file
  --params JSON           Query parameter overrides
  --headers JSON          Header overrides

Response options:
  --output PATH           Response output path (default: response.json)
  --verbose, -v           Print response headers

Global options:
  --config-dir PATH       Config alias directory (default: ~/.config/apix/configs)
  --version               Show version and build information
  --help, -h              Show this help

Examples:
  apix init github
  apix configs
  apix list github
  apix describe github get_repo
  apix preview github get_repo
  apix run github get_repo --params '{"owner":"octocat"}'
  apix github get_repo --params '{"owner":"octocat"}'
`)
}

func printVersion() {
	fmt.Printf("apix %s (commit %s, built %s)\n", version, commit, date)
}

const defaultConfigTemplate = `base_url: https://api.example.com
timeout: 30
default_headers:
  Content-Type: application/json
  User-Agent: api-explorer/1.0
auth:
  type: bearer
  token: ${API_TOKEN}
endpoints:
  health:
    method: GET
    path: /health
    description: Basic connectivity check
  get_resource:
    method: GET
    path: /resources/{id}
    params:
      id: "123"
    description: Example path parameter substitution
  create_resource:
    method: POST
    path: /resources
    body:
      name: example
    description: Example JSON body request
`
