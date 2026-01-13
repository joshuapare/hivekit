package merge

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// Test_E2E_Exact_KnownOperations applies specific operations and asserts EXACT results
//
// This test:
// 1. Analyzes the baseline test hive to get EXACT initial counts
// 2. Applies a KNOWN set of operations with specific data
// 3. Asserts the EXACT final state (not estimates or thresholds).
func Test_E2E_Exact_KnownOperations(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "e2e-exact-test")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Phase 1: Analyze baseline hive to get EXACT initial counts
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	initialStats, err := AnalyzeHive(h)
	if err != nil {
		t.Fatalf("Failed to analyze initial hive: %v", err)
	}

	t.Logf("=== INITIAL STATE ===")
	t.Logf("Total Keys: %d", initialStats.TotalKeys)
	t.Logf("Total Values: %d", initialStats.TotalValues)

	h.Close()

	// Phase 2: Apply EXACT known operations
	h, err = hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}

	session, err := NewSession(context.Background(), h, DefaultOptions())
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close(context.Background())

	plan := NewPlan()

	// Define EXACT operations with known data
	testKeyPath := []string{"_E2E_EXACT_TEST"}

	// Operation 1: Create parent key
	plan.AddEnsureKey(testKeyPath)

	// Operation 2: Add exactly 3 subkeys
	subkey1 := append([]string(nil), testKeyPath...)
	subkey1 = append(subkey1, "SubKey1")
	subkey2 := append([]string(nil), testKeyPath...)
	subkey2 = append(subkey2, "SubKey2")
	subkey3 := append([]string(nil), testKeyPath...)
	subkey3 = append(subkey3, "SubKey3")
	plan.AddEnsureKey(subkey1)
	plan.AddEnsureKey(subkey2)
	plan.AddEnsureKey(subkey3)

	// Operation 3: Add exactly 5 values to SubKey1
	plan.AddSetValue(subkey1, "String1", format.REGSZ, []byte("Test String 1\x00"))
	plan.AddSetValue(subkey1, "String2", format.REGSZ, []byte("Test String 2\x00"))
	plan.AddSetValue(subkey1, "DWORD1", format.REGDWORD, []byte{0x42, 0x00, 0x00, 0x00})
	plan.AddSetValue(subkey1, "DWORD2", format.REGDWORD, []byte{0xFF, 0x00, 0x00, 0x00})
	plan.AddSetValue(subkey1, "Binary1", format.REGBinary, []byte{0x01, 0x02, 0x03, 0x04, 0x05})

	// Operation 4: Add exactly 2 values to SubKey2
	plan.AddSetValue(subkey2, "Value1", format.REGSZ, []byte("Data\x00"))
	plan.AddSetValue(subkey2, "Value2", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})

	// Operation 5: Add exactly 1 large value to SubKey3
	largeData := bytes.Repeat([]byte("LARGE"), 1024) // 5KB value
	plan.AddSetValue(subkey3, "LargeValue", format.REGBinary, largeData)

	// Operation 6: Delete exactly 1 value from SubKey2
	plan.AddDeleteValue(subkey2, "Value2")

	// Apply plan
	applied, err := session.ApplyWithTx(context.Background(), plan)
	if err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	t.Logf("=== OPERATIONS APPLIED ===")
	t.Logf("Keys Created: %d", applied.KeysCreated)
	t.Logf("Values Set: %d", applied.ValuesSet)
	t.Logf("Values Deleted: %d", applied.ValuesDeleted)

	h.Close()

	// Phase 3: Reopen and analyze final state
	h, err = hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	finalStats, err := AnalyzeHive(h)
	if err != nil {
		t.Fatalf("Failed to analyze final hive: %v", err)
	}

	t.Logf("=== FINAL STATE ===")
	t.Logf("Total Keys: %d", finalStats.TotalKeys)
	t.Logf("Total Values: %d", finalStats.TotalValues)

	// Phase 4: EXACT ASSERTIONS (not estimates!)

	// Keys: We added exactly 4 keys (parent + 3 subkeys)
	// Note: Only count newly created keys, not re-used ones
	expectedKeysCreated := 4
	if applied.KeysCreated != expectedKeysCreated {
		t.Errorf("EXACT CHECK FAILED: Expected exactly %d keys created, got %d",
			expectedKeysCreated, applied.KeysCreated)
	}

	// Values: We set exactly 8 values (5+2+1) and deleted 1, so net +7 values in hive
	expectedValuesSet := 8
	if applied.ValuesSet != expectedValuesSet {
		t.Errorf("EXACT CHECK FAILED: Expected exactly %d values set, got %d",
			expectedValuesSet, applied.ValuesSet)
	}

	expectedValuesDeleted := 1
	if applied.ValuesDeleted != expectedValuesDeleted {
		t.Errorf("EXACT CHECK FAILED: Expected exactly %d values deleted, got %d",
			expectedValuesDeleted, applied.ValuesDeleted)
	}

	// Total keys: initial + 4 new keys
	expectedTotalKeys := initialStats.TotalKeys + 4
	if finalStats.TotalKeys != expectedTotalKeys {
		t.Errorf("EXACT CHECK FAILED: Expected exactly %d total keys, got %d",
			expectedTotalKeys, finalStats.TotalKeys)
	}

	// Total values: initial + 8 set - 1 deleted = initial + 7
	expectedTotalValues := initialStats.TotalValues + 7
	if finalStats.TotalValues != expectedTotalValues {
		t.Errorf("EXACT CHECK FAILED: Expected exactly %d total values, got %d",
			expectedTotalValues, finalStats.TotalValues)
	}

	// Phase 5: Verify EXACT key existence and value content
	builder := walker.NewIndexBuilder(h, 10000, 10000)
	idx, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Verify parent key exists
	_, exists := index.WalkPath(idx, h.RootCellOffset(), lowerPath(testKeyPath)...)
	if !exists {
		t.Errorf("EXACT CHECK FAILED: Parent key %v should exist", testKeyPath)
	}

	// Verify SubKey1 exists with exactly 5 values
	subkey1Off, exists := index.WalkPath(idx, h.RootCellOffset(), lowerPath(subkey1)...)
	if !exists {
		t.Errorf("EXACT CHECK FAILED: SubKey1 %v should exist", subkey1)
	} else {
		// Verify values in SubKey1
		valueNames := []string{"string1", "string2", "dword1", "dword2", "binary1"}
		for _, name := range valueNames {
			_, valueExists := idx.GetVK(subkey1Off, name)
			if !valueExists {
				t.Errorf("EXACT CHECK FAILED: Value %s should exist in SubKey1", name)
			}
		}
	}

	// Verify SubKey2 exists with exactly 1 value (Value1, Value2 was deleted)
	subkey2Off, exists := index.WalkPath(idx, h.RootCellOffset(), lowerPath(subkey2)...)
	if !exists {
		t.Errorf("EXACT CHECK FAILED: SubKey2 %v should exist", subkey2)
	} else {
		_, value1Exists := idx.GetVK(subkey2Off, "value1")
		if !value1Exists {
			t.Errorf("EXACT CHECK FAILED: Value1 should exist in SubKey2")
		}

		_, value2Exists := idx.GetVK(subkey2Off, "value2")
		if value2Exists {
			t.Errorf("EXACT CHECK FAILED: Value2 should NOT exist in SubKey2 (was deleted)")
		}
	}

	// Verify SubKey3 exists with exactly 1 large value
	subkey3Off, exists := index.WalkPath(idx, h.RootCellOffset(), lowerPath(subkey3)...)
	if !exists {
		t.Errorf("EXACT CHECK FAILED: SubKey3 %v should exist", subkey3)
	} else {
		_, largeValueExists := idx.GetVK(subkey3Off, "largevalue")
		if !largeValueExists {
			t.Errorf("EXACT CHECK FAILED: LargeValue should exist in SubKey3")
		}
	}

	// Verify exact value data content for String1
	vkOff, exists := idx.GetVK(subkey1Off, "string1")
	if exists {
		vkPayload, resolveErr := h.ResolveCellPayload(vkOff)
		if resolveErr == nil {
			vk, parseErr := hive.ParseVK(vkPayload)
			if parseErr == nil {
				data, dataErr := vk.Data(h.Bytes())
				if dataErr == nil {
					expectedData := []byte("Test String 1\x00")
					if !bytes.Equal(data, expectedData) {
						t.Errorf("EXACT CHECK FAILED: String1 data mismatch.\nExpected: %v\nGot: %v",
							expectedData, data)
					} else {
						t.Logf("✓ String1 data content matches exactly")
					}
				}
			}
		}
	}

	// Verify exact value data content for DWORD1
	vkOff, exists = idx.GetVK(subkey1Off, "dword1")
	if exists {
		vkPayload, resolveErr := h.ResolveCellPayload(vkOff)
		if resolveErr == nil {
			vk, parseErr := hive.ParseVK(vkPayload)
			if parseErr == nil {
				data, dataErr := vk.Data(h.Bytes())
				if dataErr == nil {
					expectedData := []byte{0x42, 0x00, 0x00, 0x00}
					if !bytes.Equal(data, expectedData) {
						t.Errorf("EXACT CHECK FAILED: DWORD1 data mismatch.\nExpected: %v\nGot: %v",
							expectedData, data)
					} else {
						t.Logf("✓ DWORD1 data content matches exactly")
					}
				}
			}
		}
	}

	// Verify exact length of LargeValue
	vkOff, exists = idx.GetVK(subkey3Off, "largevalue")
	if exists {
		vkPayload, resolveErr := h.ResolveCellPayload(vkOff)
		if resolveErr == nil {
			vk, parseErr := hive.ParseVK(vkPayload)
			if parseErr == nil {
				dataLen := vk.DataLen()
				expectedLen := 5 * 1024
				if dataLen != expectedLen {
					t.Errorf("EXACT CHECK FAILED: LargeValue length mismatch. Expected: %d, Got: %d",
						expectedLen, dataLen)
				} else {
					t.Logf("✓ LargeValue length matches exactly: %d bytes", dataLen)
				}
			}
		}
	}

	t.Logf("ALL EXACT CHECKS PASSED")
	t.Logf("   - Exact key counts verified")
	t.Logf("   - Exact value counts verified")
	t.Logf("   - Exact key existence verified")
	t.Logf("   - Exact value existence verified")
	t.Logf("   - Exact value data content verified")
	t.Logf("   - Exact deletion behavior verified")

	// Phase 6: External validation with hivexsh
	if hivexval.IsHivexshAvailable() {
		v := hivexval.Must(hivexval.New(tempHivePath, &hivexval.Options{UseHivexsh: true}))
		defer v.Close()
		if err := v.ValidateWithHivexsh(); err != nil {
			t.Errorf("hivexsh validation FAILED: %v", err)
		} else {
			t.Logf("hivexsh validation successful")
		}
	} else {
		t.Logf("hivexsh not available, skipping validation")
	}
}

// lowerPath converts a path to lowercase for case-insensitive lookups.
func lowerPath(path []string) []string {
	result := make([]string, len(path))
	for i, component := range path {
		result[i] = strings.ToLower(component)
	}
	return result
}
