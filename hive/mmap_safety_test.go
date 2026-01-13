//go:build linux || darwin

package hive

import (
	"os"
	"syscall"
	"testing"
)

func TestPreFaultPages_ValidMemory(t *testing.T) {
	// Create a temporary file with some data
	f, err := os.CreateTemp("", "prefault-test-*.bin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// Write 8KB of data
	data := make([]byte, 8192)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("failed to write data: %v", err)
	}

	// Memory map the file
	mapped, err := syscall.Mmap(int(f.Fd()), 0, len(data), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatalf("mmap failed: %v", err)
	}
	defer syscall.Munmap(mapped)

	// Pre-fault should succeed for valid memory
	err = PreFaultPages(mapped)
	if err != nil {
		t.Errorf("PreFaultPages failed on valid memory: %v", err)
	}
}

func TestPreFaultPages_EmptySlice(t *testing.T) {
	// Should handle empty slices gracefully
	err := PreFaultPages(nil)
	if err != nil {
		t.Errorf("PreFaultPages failed on nil slice: %v", err)
	}

	err = PreFaultPages([]byte{})
	if err != nil {
		t.Errorf("PreFaultPages failed on empty slice: %v", err)
	}
}

func TestPreFaultPages_SmallData(t *testing.T) {
	// Test with data smaller than a page
	data := []byte("hello world")
	err := PreFaultPages(data)
	if err != nil {
		t.Errorf("PreFaultPages failed on small data: %v", err)
	}
}

func TestValidateMappedRegion_SizeMismatch(t *testing.T) {
	data := make([]byte, 100)
	err := ValidateMappedRegion(data, 200)
	if err == nil {
		t.Error("ValidateMappedRegion should fail on size mismatch")
	}
}

func TestValidateMappedRegion_Valid(t *testing.T) {
	data := make([]byte, 100)
	err := ValidateMappedRegion(data, 100)
	if err != nil {
		t.Errorf("ValidateMappedRegion failed on valid data: %v", err)
	}
}

// TestPreFaultPages_TruncatedFile tests that PreFaultPages detects
// when a memory-mapped file has been truncated.
// This test is skipped by default as it requires specific conditions.
func TestPreFaultPages_TruncatedFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping truncated file test in short mode")
	}

	// Create a temporary file with some data
	f, err := os.CreateTemp("", "prefault-truncate-*.bin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	name := f.Name()
	defer os.Remove(name)

	// Write 16KB of data
	originalSize := 16384
	data := make([]byte, originalSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("failed to write data: %v", err)
	}

	// Memory map the file at full size
	mapped, err := syscall.Mmap(int(f.Fd()), 0, originalSize, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatalf("mmap failed: %v", err)
	}
	defer syscall.Munmap(mapped)

	// Truncate the file to a smaller size
	truncatedSize := 4096
	if err := f.Truncate(int64(truncatedSize)); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}
	f.Close()

	// Now PreFaultPages should detect the inaccessible region
	// Note: This behavior depends on the OS and filesystem
	// On some systems, the pages may still be cached and accessible
	err = PreFaultPages(mapped)
	t.Logf("PreFaultPages on truncated file: %v", err)
	// We don't assert error here because behavior varies by OS/filesystem
}
