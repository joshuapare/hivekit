package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testHivePath returns the absolute path to a test hive file
func testHivePath(t *testing.T, name string) string {
	t.Helper()
	// Go up two directories from cmd/hivectl to repo root
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "testdata", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("test file not found: %s", path)
	}
	return path
}

// captureOutput captures stdout while running a function
func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	// Save original stdout
	origStdout := os.Stdout

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	// Redirect stdout to pipe
	os.Stdout = w

	// Run function
	fnErr := fn()

	// Close write end and restore stdout
	w.Close()
	os.Stdout = origStdout

	// Read captured output
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	return buf.String(), fnErr
}

// assertJSON checks that output is valid JSON
func assertJSON(t *testing.T, output string) {
	t.Helper()
	var result interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("invalid JSON output: %v\nOutput: %s", err, output)
	}
}

// assertContains checks that output contains all expected strings
func assertContains(t *testing.T, output string, expected []string) {
	t.Helper()
	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Errorf("output missing expected string %q\nGot: %s", want, output)
		}
	}
}

// assertNotContains checks that output doesn't contain unwanted strings
func assertNotContains(t *testing.T, output string, unwanted []string) {
	t.Helper()
	for _, dont := range unwanted {
		if strings.Contains(output, dont) {
			t.Errorf("output contains unwanted string %q\nGot: %s", dont, output)
		}
	}
}
