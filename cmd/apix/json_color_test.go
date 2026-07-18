package main

import (
	"strings"
	"testing"
)

func TestColorizeJSONHighlightsValues(t *testing.T) {
	input := "{\n  \"name\": \"health\",\n  \"success\": true,\n  \"duration_ms\": 1000,\n  \"empty\": null,\n  \"path\": \"/repos/octocat/Hello-World\"\n}\n"
	output := colorizeJSON(input)
	for _, expected := range []string{
		ansiCyan + "\"name\"" + ansiReset + ": " + ansiGreen + "\"health\"" + ansiReset,
		ansiCyan + "\"success\"" + ansiReset + ": " + ansiMagenta + "true" + ansiReset,
		ansiCyan + "\"duration_ms\"" + ansiReset + ": " + ansiYellow + "1000" + ansiReset,
		ansiCyan + "\"empty\"" + ansiReset + ": " + ansiDim + "null" + ansiReset,
		ansiCyan + "\"path\"" + ansiReset + ": " + ansiGreen + "\"/repos/octocat/Hello-World\"" + ansiReset,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("colorized JSON missing %q:\n%q", expected, output)
		}
	}
}

func TestColorizeJSONPreservesEscapedStrings(t *testing.T) {
	input := "{\n  \"message\": \"quoted \\\"value\\\"\"\n}\n"
	output := colorizeJSON(input)
	expected := ansiCyan + "\"message\"" + ansiReset + ": " + ansiGreen + "\"quoted \\\"value\\\"\"" + ansiReset
	if !strings.Contains(output, expected) {
		t.Fatalf("colorized JSON escaped string incorrectly:\n%q", output)
	}
}

func TestShouldColorTerminalOutputHonorsForcedMode(t *testing.T) {
	previousMode := terminalColorMode
	defer func() { terminalColorMode = previousMode }()

	if err := setTerminalColorMode("always"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NO_COLOR", "1")
	if !shouldColorTerminalOutput() {
		t.Fatal("expected --color always to force color")
	}

	if err := setTerminalColorMode("never"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	if shouldColorTerminalOutput() {
		t.Fatal("expected --color never to disable color")
	}
}

func TestShouldColorTerminalOutputHonorsCLICOLORForce(t *testing.T) {
	previousMode := terminalColorMode
	defer func() { terminalColorMode = previousMode }()

	if err := setTerminalColorMode("auto"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")
	if !shouldColorTerminalOutput() {
		t.Fatal("expected CLICOLOR_FORCE to force color")
	}
}

func TestSetTerminalColorModeRejectsInvalidValue(t *testing.T) {
	if err := setTerminalColorMode("sometimes"); err == nil {
		t.Fatal("expected invalid color mode to fail")
	}
}
