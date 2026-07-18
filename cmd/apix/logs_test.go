package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListRequestLogsReturnsNewestFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, ".config", "apix", "logs", "api")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLogEntry(t, filepath.Join(directory, "old.json"), requestLogEntry{
		Endpoint:  "old",
		Method:    "GET",
		StartedAt: "2026-07-05T10:00:00-05:00",
	})
	writeLogEntry(t, filepath.Join(directory, "new.json"), requestLogEntry{
		Endpoint:  "new",
		Method:    "GET",
		StartedAt: "2026-07-05T11:00:00-05:00",
	})

	logs, err := listRequestLogs(filepath.Join(home, ".config", "apix", "configs", "api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Endpoint != "new" || logs[1].Endpoint != "old" {
		t.Fatalf("expected newest log first, got %#v", logs)
	}
}

func TestRequestLogEntryYAMLUsesHumanReadableShape(t *testing.T) {
	entry := requestLogEntry{
		Name:        "20260705_get_health",
		API:         "example",
		Endpoint:    "health",
		Method:      "GET",
		URL:         "https://api.example.com/health",
		Path:        "/health",
		QueryParams: map[string]any{"token": redactedValue},
		Headers:     map[string]string{"Authorization": redactedValue},
		StatusCode:  intPointer(200),
		Success:     true,
		StartedAt:   "2026-07-05T13:00:00-05:00",
		CompletedAt: "2026-07-05T13:00:01-05:00",
		DurationMS:  1000,
		Operations:  []requestLogEvent{{Name: "request_started", Timestamp: "2026-07-05T13:00:00-05:00"}},
	}
	output, err := requestLogEntryYAML(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"name: 20260705_get_health", "request:", "timing:", "operations:", "Authorization: <redacted>"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("YAML output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "{") || strings.Contains(output, `"name"`) {
		t.Fatalf("expected YAML-shaped output, got:\n%s", output)
	}
}

func TestRequestLogEntryYAMLShowsLegacyParams(t *testing.T) {
	output, err := requestLogEntryYAML(requestLogEntry{
		Name:     "legacy",
		API:      "example",
		Endpoint: "health",
		Method:   "GET",
		URL:      "https://api.example.com/health?page=1",
		Path:     "/health?page=1",
		Params:   map[string]any{"page": float64(1)},
		Success:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "query_params:") || !strings.Contains(output, "page: 1") {
		t.Fatalf("legacy params were not shown as query params:\n%s", output)
	}
}

func TestColorizeYAMLHighlightsLogValues(t *testing.T) {
	input := "name: health\nsuccess: true\nduration_ms: 1000\nAuthorization: <redacted>\nurl: https://api.example.com/health\n"
	output := colorizeYAML(input)
	for _, expected := range []string{
		ansiCyan + "name" + ansiReset + ": " + ansiGreen + "health" + ansiReset,
		ansiCyan + "success" + ansiReset + ": " + ansiMagenta + "true" + ansiReset,
		ansiCyan + "duration_ms" + ansiReset + ": " + ansiYellow + "1000" + ansiReset,
		ansiCyan + "Authorization" + ansiReset + ": " + ansiBoldRed + redactedValue + ansiReset,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("colorized output missing %q:\n%q", expected, output)
		}
	}
}

func TestTerminalLineHelpersKeepBoxWidthStable(t *testing.T) {
	line := padTerminalLine(truncateTerminalLine("abcdef", 4), 4)
	if line != "abc." {
		t.Fatalf("unexpected truncated line %q", line)
	}
	line = padTerminalLine("ab", 4)
	if line != "ab  " {
		t.Fatalf("unexpected padded line %q", line)
	}
	line = truncateTerminalLine("éclair", 4)
	if line != "écl." {
		t.Fatalf("unexpected unicode truncation %q", line)
	}
}

func TestRenderRequestLogViewerUsesBoxDrawingCharacters(t *testing.T) {
	output := captureStdout(t, func() {
		renderRequestLogViewer("example", requestLogSummary{Endpoint: "health"}, []string{"name: health"}, 0, 1, 40, false)
	})
	for _, expected := range []string{"┌", "┐", "│", "└", "┘", "─"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("viewer output missing box character %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "+---") || strings.Contains(output, "| name") {
		t.Fatalf("viewer used ASCII borders:\n%s", output)
	}
}

func intPointer(value int) *int { return &value }

func writeLogEntry(t *testing.T, path string, entry requestLogEntry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = previous }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatal(err)
	}
	return output.String()
}
