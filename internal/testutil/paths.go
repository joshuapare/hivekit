package testutil

// Test hive paths relative to the repository root.
// These constants should be used instead of hardcoding paths in test files.
const (
	// TestHiveWindows2003 is the primary test hive from Windows 2003 Server
	// This constant is also defined in setup.go for convenience.
	TestHiveWindows2003System = "testdata/suite/windows-2003-server-system"

	// Add other test hive paths here as they are identified and standardized
	// Example:
	// TestHiveWindows10 = "testdata/suite/windows-10-system"
	// TestHiveWindowsXP = "testdata/suite/windows-xp-system".
)
