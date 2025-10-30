package edit_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestLargeValueDB tests writing and reading large values that require DB records (>4KB)
func TestLargeValueDB(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
	}{
		{"5KB value (just over threshold)", 5 * 1024},
		{"16KB value (1 DB block)", 16 * 1024},
		{"20KB value (2 DB blocks)", 20 * 1024},
		{"50KB value (multiple DB blocks)", 50 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Load minimal hive as base
			data, err := os.ReadFile("../../testdata/minimal")
			if err != nil {
				t.Fatalf("Failed to load minimal hive: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			// Create editor and transaction
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			// Create test key
			err = tx.CreateKey("TestKey", hive.CreateKeyOptions{})
			if err != nil {
				t.Fatalf("Failed to create key: %v", err)
			}

			// Generate test data with a pattern (so we can verify it later)
			testData := make([]byte, tc.dataSize)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Write the large value
			err = tx.SetValue("TestKey", "LargeValue", hive.REG_BINARY, testData)
			if err != nil {
				t.Fatalf("Failed to set large value: %v", err)
			}

			// Commit the changes
			w := &writer.MemWriter{}
			err = tx.Commit(w, hive.WriteOptions{})
			if err != nil {
				t.Fatalf("Failed to commit: %v", err)
			}

			t.Logf("Hive size after writing %d byte value: %d bytes", tc.dataSize, len(w.Buf))

			// Reopen the hive and read the value back
			r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to reopen hive: %v", err)
			}
			defer r2.Close()

			// Navigate to the key
			rootID, _ := r2.Root()
			testKeyID, err := r2.Lookup(rootID, "TestKey")
			if err != nil {
				t.Fatalf("Failed to find TestKey: %v", err)
			}

			// Get the values
			valueIDs, err := r2.Values(testKeyID)
			if err != nil {
				t.Fatalf("Failed to get values: %v", err)
			}

			if len(valueIDs) != 1 {
				t.Fatalf("Expected 1 value, got %d", len(valueIDs))
			}

			// Read the value data
			readData, err := r2.ValueBytes(valueIDs[0], hive.ReadOptions{CopyData: true})
			if err != nil {
				t.Fatalf("Failed to read value data: %v", err)
			}

			// Verify the data
			if len(readData) != len(testData) {
				t.Errorf("Data length mismatch: expected %d bytes, got %d bytes", len(testData), len(readData))
			}

			if !bytes.Equal(readData, testData) {
				// Find first difference
				for i := 0; i < min(len(testData), len(readData)); i++ {
					if testData[i] != readData[i] {
						t.Errorf("Data mismatch at byte %d: expected 0x%02x, got 0x%02x", i, testData[i], readData[i])
						break
					}
				}
			}

			t.Logf("✓ Successfully wrote and read %d byte value", tc.dataSize)
		})
	}
}

// TestLargeValueDBPreservation tests that existing DB records are preserved during edit
func TestLargeValueDBPreservation(t *testing.T) {
	// Load minimal hive as base
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to load minimal hive: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer r.Close()

	// Create editor and transaction
	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Create test key
	err = tx.CreateKey("TestKey", hive.CreateKeyOptions{})
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	// Create original data (25KB)
	originalData := make([]byte, 25*1024)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	// Set large value
	err = tx.SetValue("TestKey", "LargeValue", hive.REG_BINARY, originalData)
	if err != nil {
		t.Fatalf("Failed to set large value: %v", err)
	}

	// Also set a small value
	err = tx.SetValue("TestKey", "SmallValue", hive.REG_SZ, []byte("t\x00e\x00s\x00t\x00\x00\x00"))
	if err != nil {
		t.Fatalf("Failed to set small value: %v", err)
	}

	// Commit first transaction
	w := &writer.MemWriter{}
	err = tx.Commit(w, hive.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Reopen and modify the small value (should preserve the large value's DB record)
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer r2.Close()

	ed2 := edit.NewEditor(r2)
	tx2 := ed2.Begin()

	// Modify only the small value
	err = tx2.SetValue("TestKey", "SmallValue", hive.REG_SZ, []byte("m\x00o\x00d\x00i\x00f\x00i\x00e\x00d\x00\x00\x00"))
	if err != nil {
		t.Fatalf("Failed to modify small value: %v", err)
	}

	// Commit second transaction
	w2 := &writer.MemWriter{}
	err = tx2.Commit(w2, hive.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to commit modification: %v", err)
	}

	// Verify the large value is still intact
	r3, err := reader.OpenBytes(w2.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to reopen hive for verification: %v", err)
	}
	defer r3.Close()

	// Navigate to the key
	rootID, _ := r3.Root()
	testKeyID, err := r3.Lookup(rootID, "TestKey")
	if err != nil {
		t.Fatalf("Failed to find TestKey: %v", err)
	}

	// Get the values
	valueIDs, err := r3.Values(testKeyID)
	if err != nil {
		t.Fatalf("Failed to get values: %v", err)
	}

	if len(valueIDs) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(valueIDs))
	}

	// Find and read the large value
	for _, vid := range valueIDs {
		meta, err := r3.StatValue(vid)
		if err != nil {
			continue
		}

		if meta.Name == "LargeValue" {
			readData, err := r3.ValueBytes(vid, hive.ReadOptions{CopyData: true})
			if err != nil {
				t.Fatalf("Failed to read large value: %v", err)
			}

			if !bytes.Equal(readData, originalData) {
				t.Errorf("Large value was corrupted during edit operation")
				t.Errorf("Expected %d bytes, got %d bytes", len(originalData), len(readData))
			} else {
				t.Logf("✓ Large value preserved correctly after small value modification")
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
