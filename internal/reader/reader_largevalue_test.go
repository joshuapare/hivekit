// +build hivex

package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestLargeValue_BindingsVsReader compares large value data from bindings vs reader
func TestLargeValue_BindingsVsReader(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		keyPath   []string
		valueName string
		expected  int // expected size in bytes
	}{
		{
			name:     "BootPlan",
			hivePath: "../../testdata/suite/windows-xp-system",
			keyPath:  []string{"ControlSet001", "Services", "RdyBoost", "Parameters"},
			valueName: "BootPlan",
			expected: 25544,
		},
		{
			name:     "ProductPolicy",
			hivePath: "../../testdata/suite/windows-xp-system",
			keyPath:  []string{"ControlSet001", "Control", "ProductOptions"},
			valueName: "ProductPolicy",
			expected: 22224,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.hivePath); os.IsNotExist(err) {
				t.Skipf("Hive not found: %s", tt.hivePath)
			}

			// Open with hivex bindings
			hx, err := bindings.Open(tt.hivePath, 0)
			if err != nil {
				t.Fatalf("Failed to open with hivex: %v", err)
			}
			defer hx.Close()

			// Open with our reader
			data, err := os.ReadFile(tt.hivePath)
			if err != nil {
				t.Fatalf("Failed to read hive: %v", err)
			}
			r, err := OpenBytes(data, types.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open with reader: %v", err)
			}
			defer r.Close()

			// Navigate to the key using bindings
			hxRoot := hx.Root()
			hxNode := hxRoot
			for _, name := range tt.keyPath {
				hxNode = hx.NodeGetChild(hxNode, name)
				if hxNode == 0 {
					t.Fatalf("Failed to navigate to %s with bindings", name)
				}
			}

			// Navigate to the key using reader
			ourRoot, _ := r.Root()
			ourNode := ourRoot
			for _, name := range tt.keyPath {
				ourNode, err = r.Lookup(ourNode, name)
				if err != nil {
					t.Fatalf("Failed to navigate to %s with reader: %v", name, err)
				}
			}

			// Get value from bindings
			hxVal := hx.NodeGetValue(hxNode, tt.valueName)
			if hxVal == 0 {
				t.Fatalf("Value %q not found in hivex", tt.valueName)
			}

			hxData, hxType, err := hx.ValueValue(hxVal)
			if err != nil {
				t.Fatalf("Failed to get value data from hivex: %v", err)
			}

			// Get value from reader
			ourValues, err := r.Values(ourNode)
			if err != nil {
				t.Fatalf("Failed to get values from reader: %v", err)
			}

			var ourValID types.ValueID
			for _, val := range ourValues {
				meta, _ := r.StatValue(val)
				if meta.Name == tt.valueName {
					ourValID = val
					break
				}
			}

			if ourValID == 0 {
				t.Fatalf("Value %q not found in reader", tt.valueName)
			}

			ourMeta, err := r.StatValue(ourValID)
			if err != nil {
				t.Fatalf("Failed to get value meta from reader: %v", err)
			}

			ourData, err := r.ValueBytes(ourValID, types.ReadOptions{CopyData: true})
			if err != nil {
				t.Fatalf("Failed to get value data from reader: %v", err)
			}

			// Also load the hivexget extracted data for comparison
			hivexgetData, err := os.ReadFile("/tmp/bootplan_hivexget.bin")
			if err != nil && tt.name == "BootPlan" {
				t.Logf("Warning: Could not read hivexget data: %v", err)
			}

			// Compare sizes
			t.Logf("\n=== SIZE COMPARISON ===")
			t.Logf("Expected:    %d bytes", tt.expected)
			t.Logf("Hivex bindings: %d bytes (type=%s)", len(hxData), hxType.String())
			t.Logf("Reader:      %d bytes (type=%s)", len(ourData), ourMeta.Type.String())
			if hivexgetData != nil && tt.name == "BootPlan" {
				t.Logf("Hivexget:    %d bytes", len(hivexgetData))
			}

			if len(hxData) != tt.expected {
				t.Errorf("Hivex bindings returned wrong size: got %d, want %d", len(hxData), tt.expected)
			}

			if len(ourData) != tt.expected {
				t.Errorf("Reader returned wrong size: got %d, want %d", len(ourData), tt.expected)
			}

			// Compare first and last bytes
			t.Logf("\n=== CONTENT COMPARISON ===")
			t.Logf("Hivex first 32 bytes: % x", hxData[:min(32, len(hxData))])
			t.Logf("Reader first 32 bytes: % x", ourData[:min(32, len(ourData))])
			t.Logf("Hivex last 32 bytes:  % x", hxData[max(0, len(hxData)-32):])
			t.Logf("Reader last 32 bytes:  % x", ourData[max(0, len(ourData)-32):])

			if hivexgetData != nil && tt.name == "BootPlan" {
				t.Logf("Hivexget first 32 bytes: % x", hivexgetData[:min(32, len(hivexgetData))])
				t.Logf("Hivexget last 32 bytes:  % x", hivexgetData[max(0, len(hivexgetData)-32):])
			}

			// Find first difference
			firstDiff := -1
			for i := 0; i < min(len(hxData), len(ourData)); i++ {
				if hxData[i] != ourData[i] {
					firstDiff = i
					break
				}
			}

			if firstDiff >= 0 {
				t.Logf("\n=== FIRST DIFFERENCE at byte %d ===", firstDiff)
				t.Logf("Position: %d = %d * 4096 + %d (block boundary analysis)", firstDiff, firstDiff/4096, firstDiff%4096)

				start := max(0, firstDiff-16)
				end := min(len(hxData), firstDiff+16)
				t.Logf("Hivex  [%d:%d]: % x", start, end, hxData[start:end])
				t.Logf("Reader [%d:%d]: % x", start, end, ourData[start:end])
				if hivexgetData != nil && tt.name == "BootPlan" {
					t.Logf("Hivexget [%d:%d]: % x", start, end, hivexgetData[start:end])
				}

				// Check if it's a 4-byte alignment issue
				if firstDiff > 4 {
					// See if reader data matches hivex data shifted by 4 bytes
					shifted := true
					for i := 0; i < min(100, len(hxData)-firstDiff-4); i++ {
						if ourData[firstDiff+i] != hxData[firstDiff+i+4] {
							shifted = false
							break
						}
					}
					if shifted {
						t.Logf("*** Reader data appears to be shifted by +4 bytes starting at %d ***", firstDiff)
						t.Logf("This suggests 4 extra bytes were inserted before this position")
					}
				}

				t.Errorf("Data mismatch at byte %d", firstDiff)
			} else if len(hxData) == len(ourData) {
				t.Logf("\n✓ Data matches perfectly!")
			}
		})
	}
}
