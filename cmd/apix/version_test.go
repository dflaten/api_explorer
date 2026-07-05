package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintVersionUsesBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := version, commit, date
	version, commit, date = "0.5.0", "abc1234", "2026-06-14T12:00:00Z"
	t.Cleanup(func() {
		version, commit, date = originalVersion, originalCommit, originalDate
	})

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	os.Stdout = write
	printVersion()
	os.Stdout = originalStdout
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if actual := strings.TrimSpace(string(output)); actual != "apix 0.5.0 (commit abc1234, built 2026-06-14T12:00:00Z)" {
		t.Fatalf("unexpected version output: %q", actual)
	}
}
