package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type commandResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("APIX_TEST_HELPER") != "1" {
		return
	}
	separator := 0
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index + 1
			break
		}
	}
	if err := run(os.Args[separator:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runCLI(t *testing.T, directory string, environment map[string]string, arguments ...string) commandResult {
	t.Helper()
	commandArguments := []string{"-test.run=TestCLIHelperProcess", "--"}
	commandArguments = append(commandArguments, arguments...)
	command := exec.Command(os.Args[0], commandArguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "APIX_TEST_HELPER=1", "HOME="+directory)
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	stdout, stderr := strings.Builder{}, strings.Builder{}
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func compatibilityEnv(serverURL string) map[string]string {
	return map[string]string{"TEST_SERVER_URL": serverURL, "COMPAT_TOKEN": "compat-secret"}
}

type receivedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	JSON    map[string]any
	Form    url.Values
}

func compatibilityServer(t *testing.T) (*httptest.Server, <-chan receivedRequest) {
	t.Helper()
	received := make(chan receivedRequest, 16)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		item := receivedRequest{Method: request.Method, Path: request.URL.RequestURI(), Headers: request.Header.Clone()}
		if request.Method == http.MethodPost {
			if strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				if err := request.ParseForm(); err != nil {
					t.Error(err)
				}
				item.Form = request.PostForm
			} else if err := json.NewDecoder(request.Body).Decode(&item.JSON); err != nil {
				t.Error(err)
			}
		}
		received <- item
		switch request.URL.Path {
		case "/text":
			response.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(response, "plain response")
		case "/empty":
			response.WriteHeader(http.StatusNoContent)
		case "/missing":
			response.WriteHeader(http.StatusNotFound)
			json.NewEncoder(response).Encode(map[string]any{"error": "not found"})
		case "/token":
			json.NewEncoder(response).Encode(map[string]any{"access_token": "new-token"})
		case "/items/42", "/form":
			json.NewEncoder(response).Encode(map[string]any{"accepted": true})
		default:
			json.NewEncoder(response).Encode(map[string]any{"healthy": true})
		}
	}))
	t.Cleanup(server.Close)
	return server, received
}

func TestCLIRequestPreviewRedactsSecrets(t *testing.T) {
	directory := t.TempDir()
	result := runCLI(t, directory, compatibilityEnv("http://example.invalid"), "preview", fixturePath(t, "api.yaml"), "inspect", "--params", `{"id":"99","page":2}`)
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	for _, expected := range []string{"Method: POST", "http://example.invalid/items/99?source=config&page=2", `"Authorization": "<redacted>"`} {
		if !strings.Contains(result.stdout, expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, result.stdout)
		}
	}
	if strings.Contains(result.stdout, "compat-secret") {
		t.Fatal("secret was printed")
	}
}

func TestCLIShorthandAllowsCommandNamedEndpoints(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "api.yaml")
	config := "endpoints:\n  preview:\n    method: GET\n    url: http://example.invalid/preview\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, directory, nil, configPath, "preview", "--output", filepath.Join(directory, "response.json"))
	if result.exitCode != 1 || !strings.Contains(result.stderr, "example.invalid/preview") {
		t.Fatalf("expected shorthand to resolve preview as endpoint, got %#v", result)
	}
}

func TestCLIVersion(t *testing.T) {
	directory := t.TempDir()
	result := runCLI(t, directory, nil, "--version")
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	if result.stdout != "apix 0.4.0 (commit none, built unknown)\n" {
		t.Fatalf("unexpected version output: %q", result.stdout)
	}
}

func TestCLIExecutesJSONAndFormRequests(t *testing.T) {
	server, received := compatibilityServer(t)
	directory := t.TempDir()
	bodyPath := filepath.Join(directory, "body.json")
	if err := os.WriteFile(bodyPath, []byte(`{"name":"override","count":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, directory, compatibilityEnv(server.URL), fixturePath(t, "api.yaml"), "inspect", "--body", bodyPath, "--headers", `{"X-Endpoint":"cli-value","X-CLI":"present"}`)
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	request := <-received
	if request.Path != "/items/42?source=config" || request.Headers.Get("Authorization") != "Bearer compat-secret" || request.Headers.Get("X-CLI") != "present" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if request.JSON["name"] != "override" || request.JSON["count"] != float64(2) {
		t.Fatalf("unexpected JSON body: %#v", request.JSON)
	}

	result = runCLI(t, directory, compatibilityEnv(server.URL), fixturePath(t, "api.yaml"), "form")
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	request = <-received
	if request.Form.Get("name") != "example" || request.Form.Get("active") != "true" {
		t.Fatalf("unexpected form: %#v", request.Form)
	}
}

func TestCLIBasicAuthAndCollections(t *testing.T) {
	server, received := compatibilityServer(t)
	directory := t.TempDir()
	result := runCLI(t, directory, map[string]string{"TEST_SERVER_URL": server.URL}, fixturePath(t, "basic-auth.yaml"), "auth")
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("compat-user:compat-password"))
	if request := <-received; request.Headers.Get("Authorization") != expectedAuth {
		t.Fatalf("unexpected auth header %q", request.Headers.Get("Authorization"))
	}

	result = runCLI(t, directory, compatibilityEnv(server.URL), "collection", fixturePath(t, "api.yaml"), fixturePath(t, "collection.yaml"))
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	var output []map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
		t.Fatal(err)
	}
	if len(output) != 3 || output[0]["endpoint"] != "health" || output[1]["endpoint"] != "health" || output[2]["endpoint"] != "unavailable" {
		t.Fatalf("collection output did not preserve order and duplicates: %#v", output)
	}
	if output[0]["success"] != true || output[1]["success"] != true || output[2]["success"] != false {
		t.Fatalf("unexpected collection output: %#v", output)
	}
}

func TestCLIResponseFilesAndTokenPersistence(t *testing.T) {
	server, _ := compatibilityServer(t)
	for _, test := range []struct {
		endpoint string
		contains string
		expected map[string]any
	}{
		{"text", "plain response", map[string]any{"raw": "plain response"}},
		{"empty", "Response Body:\nnull", map[string]any{"raw": nil}},
		{"missing", "Status Code: 404", map[string]any{"error": "not found"}},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			directory := t.TempDir()
			outputPath := filepath.Join(directory, "response.json")
			result := runCLI(t, directory, compatibilityEnv(server.URL), fixturePath(t, "api.yaml"), test.endpoint, "--output", outputPath)
			if result.exitCode != 0 || !strings.Contains(result.stdout, test.contains) {
				t.Fatalf("unexpected result: %#v", result)
			}
			data, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]any
			if err := json.Unmarshal(data, &actual); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(actual) != fmt.Sprint(test.expected) {
				t.Fatalf("expected %#v, got %#v", test.expected, actual)
			}
		})
	}

	directory := t.TempDir()
	configHome := filepath.Join(directory, ".config", "apix")
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, ".env"), []byte("PERSISTED_TOKEN=old-token\nOTHER=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, directory, map[string]string{"TEST_SERVER_URL": server.URL}, fixturePath(t, "token.yaml"), "token")
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	data, err := os.ReadFile(filepath.Join(configHome, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PERSISTED_TOKEN=new-token\nOTHER=value\n" {
		t.Fatalf("unexpected env file: %q", data)
	}
}

func TestCLIDotenvDiscoveryAndErrors(t *testing.T) {
	directory := t.TempDir()
	configHome := filepath.Join(directory, ".config", "apix")
	configPath := filepath.Join(directory, "dotenv.yaml")
	config := "default_headers:\n  Authorization: Bearer ${DOTENV_TOKEN}\n  X-Dotenv-Value: ${DOTENV_TOKEN}\nendpoints:\n  health:\n    method: GET\n    url: http://example.invalid/health\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, ".env"), []byte("DOTENV_TOKEN=file-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, directory, nil, "preview", configPath, "health")
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"X-Dotenv-Value": "file-token"`) {
		t.Fatalf("dotenv was not loaded: %#v", result)
	}
	result = runCLI(t, directory, map[string]string{"DOTENV_TOKEN": "shell-token"}, "preview", configPath, "health")
	if !strings.Contains(result.stdout, `"X-Dotenv-Value": "shell-token"`) {
		t.Fatalf("shell value did not win: %s", result.stdout)
	}

	result = runCLI(t, directory, nil, "init", "github")
	if result.exitCode != 0 {
		t.Fatal(result.stderr)
	}
	if _, err := os.Stat(filepath.Join(configHome, "configs", "github.yaml")); err != nil {
		t.Fatal(err)
	}

	defaultConfigDirectory := filepath.Join(configHome, "configs")
	fixture, err := os.ReadFile(fixturePath(t, "api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultConfigDirectory, "compat.yaml"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"configs"}, {"list", "compat"}, {"describe", "compat", "health"}} {
		result = runCLI(t, directory, compatibilityEnv("http://example.invalid"), arguments...)
		if result.exitCode != 0 {
			t.Fatal(result.stderr)
		}
	}

	configDirectory := filepath.Join(directory, "configs")
	if err := os.Mkdir(configDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "compat.yaml"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"--config-dir", configDirectory, "configs"}, {"--config-dir", configDirectory, "list", "compat"}, {"--config-dir", configDirectory, "describe", "compat", "health"}} {
		result = runCLI(t, directory, compatibilityEnv("http://example.invalid"), arguments...)
		if result.exitCode != 0 {
			t.Fatal(result.stderr)
		}
	}

	invalidConfig := filepath.Join(directory, "invalid.yaml")
	if err := os.WriteFile(invalidConfig, []byte("endpoints:\n  broken:\n    path: /missing-method\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		arguments []string
		message   string
	}{
		{[]string{fixturePath(t, "api.yaml"), "health", "--params", "{invalid"}, "invalid JSON for --params"},
		{[]string{fixturePath(t, "api.yaml"), "health", "--params", `{"page":1} garbage`}, "invalid JSON for --params"},
		{[]string{fixturePath(t, "api.yaml"), "health", "--headers", `{"X-Test":"value"} garbage`}, "invalid JSON for --headers"},
		{[]string{invalidConfig, "broken"}, "'method' is a required property"},
	} {
		result = runCLI(t, directory, compatibilityEnv("http://example.invalid"), test.arguments...)
		if result.exitCode != 1 || !strings.Contains(result.stderr, test.message) {
			t.Fatalf("expected %q, got %#v", test.message, result)
		}
	}
}
