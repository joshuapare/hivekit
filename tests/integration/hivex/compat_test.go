//go:build hivex
// +build hivex

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Test hives for direct comparison
var testHives = []struct {
	name string
	path string
}{
	{"minimal", "../../../testdata/minimal"},
	{"special", "../../../testdata/special"},
	{"rlenvalue_test_hive", "../../../testdata/rlenvalue_test_hive"},
	{"large", "../../../testdata/large"},
	{"windows-xp-system", "../../../testdata/suite/windows-xp-system"},
	{"windows-xp-software", "../../../testdata/suite/windows-xp-software"},
	{"windows-2003-server-system", "../../../testdata/suite/windows-2003-server-system"},
	{"windows-2003-server-software", "../../../testdata/suite/windows-2003-server-software"},
	{"windows-8-enterprise-system", "../../../testdata/suite/windows-8-enterprise-system"},
	{"windows-8-enterprise-software", "../../../testdata/suite/windows-8-enterprise-software"},
	{"windows-2012-system", "../../../testdata/suite/windows-2012-system"},
	{"windows-2012-software", "../../../testdata/suite/windows-2012-software"},
}

// Shared test data directory
var testDataDir = filepath.Join("..", "..", "..", "testdata")

// TestHivexDirectComparison performs direct differential testing between hivex (CGO)
// and gohivex (native Go) implementations on all test hives.
//
// To run: go test -tags=hivex -v ./tests/integration -run TestHivexDirectComparison
//
// Requires:
//   - libhivex installed (brew install hivex on macOS, apt-get install libhivex-dev on Linux)
//   - gabriel-samfira/go-hivex Go bindings (go get github.com/gabriel-samfira/go-hivex)
func TestHivexDirectComparison(t *testing.T) {
	for _, tc := range testHives {
		tc := tc // capture loop variable
		t.Run(tc.name, func(t *testing.T) {
			// Check if test hive exists
			if _, err := os.Stat(tc.path); os.IsNotExist(err) {
				t.Skipf("Test hive not found: %s", tc.path)
			}

			// Open with hivex (CGO bindings)
			t.Logf("Opening %s with hivex (CGO)", tc.path)
			hivexHandle, err := OpenHivex(tc.path)
			if err != nil {
				t.Fatalf("Failed to open hive with hivex: %v", err)
			}
			defer hivexHandle.Close()

			hivexRoot, err := hivexHandle.Root()
			if err != nil {
				t.Fatalf("Failed to get hivex root: %v", err)
			}

			// Open with gohivex (native Go)
			t.Logf("Opening %s with gohivex (native Go)", tc.path)
			gohivexReader, err := reader.Open(tc.path, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive with gohivex: %v", err)
			}
			defer gohivexReader.Close()

			gohivexRoot, err := gohivexReader.Root()
			if err != nil {
				t.Fatalf("Failed to get gohivex root: %v", err)
			}

			// Perform recursive comparison
			t.Logf("Comparing trees recursively...")
			result, err := CompareTreesRecursively(
				hivexHandle, hivexRoot,
				gohivexReader, gohivexRoot,
				"\\",
			)
			if err != nil {
				t.Fatalf("Comparison failed: %v", err)
			}

			// Log summary
			t.Log(result.Summary())

			// Assert no mismatches
			if len(result.Mismatches) > 0 {
				t.Errorf("Found %d mismatches between hivex and gohivex:", len(result.Mismatches))

				// Show first 10 mismatches in detail
				maxShow := 10
				if len(result.Mismatches) < maxShow {
					maxShow = len(result.Mismatches)
				}

				for i := 0; i < maxShow; i++ {
					m := result.Mismatches[i]
					t.Errorf("  [%s] %s: %s", m.Category, m.Path, m.Message)
					t.Errorf("    hivex:   %v", m.HivexValue)
					t.Errorf("    gohivex: %v", m.GohivexValue)
				}

				if len(result.Mismatches) > maxShow {
					t.Errorf("  ... and %d more mismatches", len(result.Mismatches)-maxShow)
				}

				t.FailNow()
			}

			t.Logf("✓ Perfect match: %d nodes compared, %d values compared",
				result.NodesCompared, result.ValuesCompared)
		})
	}
}

// TestHivexDirectComparison_SingleNode tests comparison of a single node (non-recursive)
// This is useful for debugging specific node comparison issues.
func TestHivexDirectComparison_SingleNode(t *testing.T) {
	hivePath := "../../testdata/minimal"

	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", hivePath)
	}

	// Open with both implementations
	hivexHandle, err := OpenHivex(hivePath)
	if err != nil {
		t.Fatalf("Failed to open hive with hivex: %v", err)
	}
	defer hivexHandle.Close()

	gohivexReader, err := reader.Open(hivePath, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open hive with gohivex: %v", err)
	}
	defer gohivexReader.Close()

	// Get root nodes
	hivexRoot, err := hivexHandle.Root()
	if err != nil {
		t.Fatalf("Failed to get hivex root: %v", err)
	}

	gohivexRoot, err := gohivexReader.Root()
	if err != nil {
		t.Fatalf("Failed to get gohivex root: %v", err)
	}

	// Compare just root node metadata (not recursive)
	hivexName, err := hivexHandle.NodeName(hivexRoot)
	if err != nil {
		t.Fatalf("hivex NodeName failed: %v", err)
	}

	gohivexMeta, err := gohivexReader.StatKey(gohivexRoot)
	if err != nil {
		t.Fatalf("gohivex StatKey failed: %v", err)
	}

	t.Logf("Root node comparison:")
	t.Logf("  hivex name:   %q", hivexName)
	t.Logf("  gohivex name: %q", gohivexMeta.Name)

	if hivexName != gohivexMeta.Name {
		t.Errorf("Root node names differ: hivex=%q, gohivex=%q", hivexName, gohivexMeta.Name)
	}

	// Compare child counts
	hivexChildCount, err := hivexHandle.NodeNrChildren(hivexRoot)
	if err != nil {
		t.Fatalf("hivex NodeNrChildren failed: %v", err)
	}

	if int64(gohivexMeta.SubkeyN) != hivexChildCount {
		t.Errorf("Child count differs: hivex=%d, gohivex=%d", hivexChildCount, gohivexMeta.SubkeyN)
	}

	// Compare value counts
	hivexValueCount, err := hivexHandle.NodeNrValues(hivexRoot)
	if err != nil {
		t.Fatalf("hivex NodeNrValues failed: %v", err)
	}

	if int64(gohivexMeta.ValueN) != hivexValueCount {
		t.Errorf("Value count differs: hivex=%d, gohivex=%d", hivexValueCount, gohivexMeta.ValueN)
	}

	t.Logf("✓ Root node metadata matches")
}

// TestHivexWrapper_BasicOperations tests the hivex wrapper functions independently
func TestHivexWrapper_BasicOperations(t *testing.T) {
	hivePath := "../../testdata/minimal"

	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", hivePath)
	}

	// Test OpenHivex
	t.Run("OpenHivex", func(t *testing.T) {
		handle, err := OpenHivex(hivePath)
		if err != nil {
			t.Fatalf("OpenHivex failed: %v", err)
		}
		defer handle.Close()

		if handle.handle == nil {
			t.Error("Expected non-nil handle")
		}
	})

	// Test Root
	t.Run("Root", func(t *testing.T) {
		handle, err := OpenHivex(hivePath)
		if err != nil {
			t.Fatalf("OpenHivex failed: %v", err)
		}
		defer handle.Close()

		root, err := handle.Root()
		if err != nil {
			t.Fatalf("Root failed: %v", err)
		}

		if root == 0 {
			t.Error("Expected non-zero root node ID")
		}
	})

	// Test NodeName
	t.Run("NodeName", func(t *testing.T) {
		handle, err := OpenHivex(hivePath)
		if err != nil {
			t.Fatalf("OpenHivex failed: %v", err)
		}
		defer handle.Close()

		root, err := handle.Root()
		if err != nil {
			t.Fatalf("Root failed: %v", err)
		}

		name, err := handle.NodeName(root)
		if err != nil {
			t.Fatalf("NodeName failed: %v", err)
		}

		t.Logf("Root node name: %q", name)
	})

	// Test NodeChildren
	t.Run("NodeChildren", func(t *testing.T) {
		handle, err := OpenHivex(hivePath)
		if err != nil {
			t.Fatalf("OpenHivex failed: %v", err)
		}
		defer handle.Close()

		root, err := handle.Root()
		if err != nil {
			t.Fatalf("Root failed: %v", err)
		}

		children, err := handle.NodeChildren(root)
		if err != nil {
			t.Fatalf("NodeChildren failed: %v", err)
		}

		t.Logf("Root has %d children", len(children))
	})

	// Test NodeValues
	t.Run("NodeValues", func(t *testing.T) {
		handle, err := OpenHivex(hivePath)
		if err != nil {
			t.Fatalf("OpenHivex failed: %v", err)
		}
		defer handle.Close()

		root, err := handle.Root()
		if err != nil {
			t.Fatalf("Root failed: %v", err)
		}

		values, err := handle.NodeValues(root)
		if err != nil {
			t.Fatalf("NodeValues failed: %v", err)
		}

		t.Logf("Root has %d values", len(values))

		// If there are values, test value operations
		if len(values) > 0 {
			valID := values[0]

			name, err := handle.ValueKey(valID)
			if err != nil {
				t.Errorf("ValueKey failed: %v", err)
			} else {
				t.Logf("First value name: %q", name)
			}

			valType, err := handle.ValueType(valID)
			if err != nil {
				t.Errorf("ValueType failed: %v", err)
			} else {
				t.Logf("First value type: %s", HivexValueTypeName(valType))
			}

			_, data, err := handle.ValueValue(valID)
			if err != nil {
				t.Errorf("ValueValue failed: %v", err)
			} else {
				t.Logf("First value data: %d bytes", len(data))
			}
		}
	})
}
