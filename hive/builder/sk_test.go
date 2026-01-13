package builder

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

func TestSKCellCreation(t *testing.T) {
	path := "/tmp/test-sk-cells.hive"
	os.Remove(path)

	opts := DefaultOptions()
	opts.Strategy = StrategyInPlace

	b, err := New(path, opts)
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}
	defer os.Remove(path)
	defer b.Close()

	// Create 100 keys - they should all share the same security descriptor
	for range 100 {
		keyPath := []string{"Test", "Key", "SubKey"}
		if err := b.EnsureKey(keyPath); err != nil {
			t.Fatalf("Failed to create key: %v", err)
		}
	}

	if err := b.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Open the hive and check for SK cells
	h, err := hive.Open(path)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Scan for SK cells
	data := h.Bytes()
	skCount := 0
	for i := 0x1000; i < len(data)-2; i++ {
		if data[i] == 's' && data[i+1] == 'k' {
			skCount++
		}
	}

	t.Logf("Found %d SK cells", skCount)
	if skCount == 0 {
		t.Error("Expected to find SK cells, found 0")
	}
}
