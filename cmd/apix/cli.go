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

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

type options struct {
	Body           string
	Params         string
	Headers        string
	ConfigDir      string
	RequestPreview bool
	Output         string
	Verbose        bool
}

var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"proxy-authorization": true,
	"set-cookie":          true,
	"x-api-key":           true,
	"api-key":             true,
}

var envTokenPattern = regexp.MustCompile(`^\$\{([A-Z0-9_]+)\}$`)

func run(arguments []string) error {
	command := newRootCommand()
	command.SetArgs(arguments)
	return command.Execute()
}

func newRootCommand() *cobra.Command {
	options := options{Output: "response.json"}
	showVersion := false
	root := &cobra.Command{
		Use:           "apix",
		Short:         "Explore and test HTTP APIs from reusable YAML configs",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `apix explores and tests HTTP APIs from reusable YAML configs.

Configs are YAML files addressed either by alias from ~/.config/apix/configs
or by explicit .yaml/.yml path. Secrets can live in ~/.config/apix/.env and
be referenced with ${ENV_VAR} placeholders.`,
		Example: `  apix init github
  apix configs
  apix list github
  apix describe github get_repo
  apix preview github get_repo --params '{"owner":"octocat","repo":"Hello-World"}'
  apix run github create_issue --body issue.json --headers '{"X-Trace":"cli"}'
  apix collection github smoke.yaml
  apix logs github`,
		RunE: func(command *cobra.Command, args []string) error {
			if showVersion {
				printVersion()
				return nil
			}
			return command.Help()
		},
	}
	root.PersistentFlags().StringVar(&options.ConfigDir, "config-dir", "", "config alias directory (default: ~/.config/apix/configs)")
	root.Flags().BoolVar(&showVersion, "version", false, "show version and build information")

	root.AddCommand(newInitCommand(&options))
	root.AddCommand(newConfigsCommand(&options))
	root.AddCommand(newListCommand(&options))
	root.AddCommand(newDescribeCommand(&options))
	root.AddCommand(newPreviewCommand(&options))
	root.AddCommand(newRunCommand(&options))
	root.AddCommand(newCollectionCommand(&options))
	root.AddCommand(newLogsCommand(&options))
	return root
}

func newInitCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:   "init NAME|PATH",
		Short: "Create a starter YAML API config",
		Long: `Create a starter YAML API config.

NAME creates ~/.config/apix/configs/NAME.yaml by default.
PATH writes to an explicit .yaml/.yml path.

The generated config demonstrates base_url, timeout, default_headers, bearer
auth, endpoints, path params, and JSON bodies.`,
		Example: `  apix init github
  apix --config-dir ./configs init github
  apix init ./github.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := preparePaths(options); err != nil {
				return err
			}
			path, err := resolveInitConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			if err := writeConfigTemplate(path); err != nil {
				return err
			}
			fmt.Printf("Created starter config at %s\n", path)
			fmt.Println("Replace placeholders like ${API_TOKEN} with environment variables before calling the API.")
			return nil
		},
	}
}

func newConfigsCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:   "configs",
		Short: "List available config aliases",
		Long: `List available API config aliases.

Aliases are YAML files in the config directory. By default, github.yaml is
used as "github" in commands such as "apix list github".`,
		Example: `  apix configs
  apix --config-dir ./configs configs`,
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := preparePaths(options); err != nil {
				return err
			}
			return printConfigList(options.ConfigDir)
		},
	}
}

func newListCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:     "list API",
		Aliases: []string{"ls"},
		Short:   "List configured endpoints",
		Long:    "List endpoint names, HTTP methods, paths, and descriptions for one API config.",
		Example: `  apix list github
  apix list ./github.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareRuntime(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			return withAPIClient(configPath, func(client *APIClient) error {
				printEndpointList(client)
				return nil
			})
		},
	}
}

func newDescribeCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:   "describe API ENDPOINT",
		Short: "Describe an endpoint without executing it",
		Long:  "Show one endpoint's resolved method, URL, headers, params, and body without sending a request.",
		Example: `  apix describe github get_repo
  apix describe ./github.yaml get_repo`,
		Args: cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareRuntime(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			return withAPIClient(configPath, func(client *APIClient) error {
				return printEndpointDetails(client, args[1])
			})
		},
	}
}

func newPreviewCommand(options *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "preview API ENDPOINT",
		Short: "Preview a request without sending it",
		Long:  "Print the exact request that would be sent, with sensitive headers redacted.",
		Example: `  apix preview github get_repo --params '{"owner":"octocat","repo":"Hello-World"}'
  apix preview github create_issue --body issue.json --headers '{"X-Trace":"cli"}'`,
		Args: cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareRuntime(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			options.RequestPreview = true
			return executeResolvedEndpoint(*options, configPath, args[1])
		},
	}
	addRequestFlags(command, options)
	return command
}

func newRunCommand(options *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "run API ENDPOINT",
		Short: "Execute an endpoint",
		Long: `Execute one configured endpoint.

The request preview is printed before sending the request. The response body is
saved to response.json unless --output is provided.`,
		Example: `  apix run github get_repo --params '{"owner":"octocat","repo":"Hello-World"}'
  apix run github create_issue --body issue.json --output issue-response.json
  apix run ./github.yaml get_repo -v`,
		Args: cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareRuntime(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			options.RequestPreview = false
			return executeResolvedEndpoint(*options, configPath, args[1])
		},
	}
	addRequestFlags(command, options)
	command.Flags().StringVar(&options.Output, "output", "response.json", "response output path")
	command.Flags().BoolVarP(&options.Verbose, "verbose", "v", false, "print response headers")
	return command
}

func newCollectionCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:   "collection API PATH",
		Short: "Execute a YAML request collection",
		Long: `Execute a YAML request collection against one API config.

Collection file shape:
  requests:
    - endpoint: get_repo
      params:
        owner: octocat
        repo: Hello-World
      headers:
        X-Trace: smoke
      body_file: request.json`,
		Example: `  apix collection github smoke.yaml
  apix collection ./github.yaml ./collections/smoke.yaml`,
		Args: cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareRuntime(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			return withAPIClient(configPath, func(client *APIClient) error {
				results, err := client.executeCollection(args[1])
				if err != nil {
					return err
				}
				return printJSON(results)
			})
		},
	}
}

func newLogsCommand(options *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logs API",
		Short: "Browse request logs",
		Long:  "Browse request logs for one API config. Logs are stored under ~/.config/apix/logs.",
		Example: `  apix logs github
  apix logs ./github.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := preparePaths(options); err != nil {
				return err
			}
			configPath, err := resolveConfigPath(args[0], options.ConfigDir)
			if err != nil {
				return err
			}
			return printRequestLogs(configPath)
		},
	}
}

func addRequestFlags(command *cobra.Command, options *options) {
	command.Flags().StringVar(&options.Body, "body", "", "path to a JSON body file")
	command.Flags().StringVar(&options.Params, "params", "", "query/path parameter overrides as a JSON object")
	command.Flags().StringVar(&options.Headers, "headers", "", "header overrides as a JSON object with string values")
}

func preparePaths(options *options) error {
	return setDefaultPaths(options)
}

func prepareRuntime(options *options) error {
	if err := preparePaths(options); err != nil {
		return err
	}
	return loadEnvFile(defaultEnvFile())
}

func withAPIClient(configPath string, execute func(*APIClient) error) error {
	client, err := newAPIClient(configPath)
	if err != nil {
		return err
	}
	return execute(client)
}

func executeResolvedEndpoint(options options, configPath, endpointName string) error {
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
	response, err := executeWithLog(client, definition)
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

func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

func printVersion() {
	fmt.Printf("apix %s\n", versionString())
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
