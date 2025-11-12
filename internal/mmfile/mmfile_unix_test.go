//go:build unix

package mmfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapReadOnlyUnix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mmap test in short mode")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	want := []byte{0xde, 0xad, 0xbe, 0xef, 0x42}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, cleanup, err := Map(path)
	if err != nil {
		t.Fatalf("MapReadOnly: %v", err)
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Fatalf("cleanup: %v", cleanupErr)
		}
	}()
	if len(data) != len(want) {
		t.Fatalf("len mismatch: got %d want %d", len(data), len(want))
	}
	for i, b := range want {
		if data[i] != b {
			t.Fatalf("byte %d mismatch: got 0x%x want 0x%x", i, data[i], b)
		}
	}
}

func TestMapReadOnlyUnixZeroLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, cleanup, err := Map(path)
	if err != nil {
		t.Fatalf("MapReadOnly: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected zero-length mapping, got %d", len(data))
	}
	if cleanup == nil {
		t.Fatalf("expected cleanup function")
	}
	if cleanupErr := cleanup(); cleanupErr != nil {
		t.Fatalf("cleanup: %v", cleanupErr)
	}
}
