package testutil

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
)

// TestHiveWindows2003 is the path to the Windows 2003 test hive from the repository root.
const TestHiveWindows2003 = "testdata/suite/windows-2003-server-system"

// SetupTestHive copies a test hive to a temporary directory and opens it.
// Returns the opened hive and a cleanup function.
// Calls t.Skip if the test hive is not found.
//
// Example:
//
//	h, cleanup := testutil.SetupTestHive(t)
//	defer cleanup()
func SetupTestHive(t *testing.T) (*hive.Hive, func()) {
	t.Helper()
	return SetupTestHiveFrom(t, TestHiveWindows2003, "test-hive")
}

// SetupTestHiveFrom copies a test hive from the specified path to a temporary directory and opens it.
// The tempName parameter sets the name of the temporary file.
// Returns the opened hive and a cleanup function.
// Calls t.Skip if the source hive is not found.
func SetupTestHiveFrom(t *testing.T, sourceHivePath, tempName string) (*hive.Hive, func()) {
	t.Helper()

	// Resolve path - try both relative to repo root and from current package
	testHivePath := resolveTestPath(t, sourceHivePath)

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), tempName)
	copyHiveFile(t, testHivePath, tempHivePath)

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}

// SetupTestHiveWithAllocator sets up a test hive with a FastAllocator.
// Returns the hive, allocator, and cleanup function.
// The allocator is created with a nil DirtyTracker for read-only use.
//
// Example:
//
//	h, allocator, cleanup := testutil.SetupTestHiveWithAllocator(t)
//	defer cleanup()
func SetupTestHiveWithAllocator(t *testing.T) (*hive.Hive, *alloc.FastAllocator, func()) {
	t.Helper()
	return SetupTestHiveWithAllocatorFrom(t, TestHiveWindows2003, "test-hive")
}

// SetupTestHiveWithAllocatorFrom is like SetupTestHiveWithAllocator but allows specifying the source hive.
func SetupTestHiveWithAllocatorFrom(
	t *testing.T,
	sourceHivePath, tempName string,
) (*hive.Hive, *alloc.FastAllocator, func()) {
	t.Helper()

	// Resolve path
	testHivePath := resolveTestPath(t, sourceHivePath)

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), tempName)
	copyHiveFile(t, testHivePath, tempHivePath)

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	// Create allocator (nil DirtyTracker for read-only use)
	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create allocator: %v", err)
	}

	cleanup := func() {
		h.Close()
	}

	return h, allocator, cleanup
}

// resolveTestPath attempts to find the test hive file by trying multiple path resolutions.
// This handles the fact that tests may be run from different working directories.
func resolveTestPath(t *testing.T, relativePath string) string {
	t.Helper()

	// Try paths in order of likelihood
	candidates := []string{
		relativePath,                     // Direct path (from repo root)
		"../../" + relativePath,          // From package two levels deep (e.g., hive/merge/)
		"../../../" + relativePath,       // From package three levels deep
		"../../../../" + relativePath,    // From package four levels deep
		"../../../../../" + relativePath, // From package five levels deep
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// If not found, skip the test
	t.Skipf("Test hive not found at any candidate path starting from: %s", relativePath)
	return "" // unreachable
}

// copyHiveFile copies a hive file from src to dst.
// Calls t.Fatal if the copy fails.
func copyHiveFile(t *testing.T, src, dst string) {
	t.Helper()

	srcFile, err := os.Open(src)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	defer dstFile.Close()

	if _, copyErr := io.Copy(dstFile, srcFile); copyErr != nil {
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
}
