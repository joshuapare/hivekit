package strategy

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

// setupAppendStrategy creates a test hive and Append strategy for testing.
func setupAppendStrategy(t *testing.T) (*Append, *hive.Hive, func()) {
	testHivePath := "../../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "append-strategy-test-hive")
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

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	// Create dirty tracker (needed by allocator)
	dt := dirty.NewTracker(h)

	// Create allocator with dirty tracker
	allocator, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Build index
	builder := walker.NewIndexBuilder(h, 10000, 10000)
	idx, err := builder.Build(context.Background())
	if err != nil {
		h.Close()
		t.Fatalf("Failed to build index: %v", err)
	}

	// Create Append strategy
	strategy := NewAppend(h, allocator, dt, idx).(*Append)

	cleanup := func() {
		h.Close()
	}

	return strategy, h, cleanup
}

// Test_Append_EnsureKey verifies that EnsureKey creates new keys.
func Test_Append_EnsureKey(t *testing.T) {
	strategy, _, cleanup := setupAppendStrategy(t)
	defer cleanup()

	// Create a unique test key
	path := []string{"_StrategyTest_Append", "TestKey"}

	// Ensure key
	nkRef, keysCreated, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Verify ref is non-zero
	if nkRef == 0 {
		t.Errorf("Expected non-zero nkRef, got 0")
	}

	t.Logf("EnsureKey successful: nkRef=%d, keysCreated=%v", nkRef, keysCreated)
}

// Test_Append_SetValue_Small verifies that SetValue works for small values.
func Test_Append_SetValue_Small(t *testing.T) {
	strategy, _, cleanup := setupAppendStrategy(t)
	defer cleanup()

	// Create parent key
	path := []string{"_StrategyTest_Append", "SmallValue"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Set small value
	data := []byte("Hello, Append Strategy!")
	err = strategy.SetValue(context.Background(), path, "TestValue", format.REGSZ, data)
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	t.Logf("SetValue successful: path=%v, name=TestValue", path)
}

// Test_Append_SetValue_Large verifies that SetValue works for large values.
func Test_Append_SetValue_Large(t *testing.T) {
	strategy, _, cleanup := setupAppendStrategy(t)
	defer cleanup()

	// Create parent key
	path := []string{"_StrategyTest_Append", "LargeValue"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Set large value (50KB)
	data := bytes.Repeat([]byte("X"), 50*1024)
	err = strategy.SetValue(context.Background(), path, "LargeData", format.REGBinary, data)
	if err != nil {
		t.Fatalf("SetValue (large) failed: %v", err)
	}

	t.Logf("SetValue (large) successful: path=%v, name=LargeData, size=%d bytes", path, len(data))
}

// Test_Append_DeleteValue verifies that DeleteValue removes from index but doesn't free cells.
func Test_Append_DeleteValue(t *testing.T) {
	strategy, h, cleanup := setupAppendStrategy(t)
	defer cleanup()

	// Create key and value
	path := []string{"_StrategyTest_Append", "DeleteTest"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	data := []byte("ToBeDeleted")
	err = strategy.SetValue(context.Background(), path, "TempValue", format.REGSZ, data)
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Record hive size before delete
	sizeBefore := len(h.Bytes())

	// Delete value
	err = strategy.DeleteValue(context.Background(), path, "TempValue")
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	// Verify hive did NOT shrink (append-only: orphaned cells remain)
	sizeAfter := len(h.Bytes())
	if sizeAfter < sizeBefore {
		t.Errorf("Hive shrank after delete (should be append-only): before=%d, after=%d", sizeBefore, sizeAfter)
	}

	t.Logf("DeleteValue successful: hive size unchanged (append-only): before=%d, after=%d", sizeBefore, sizeAfter)
}

// Test_Append_DeleteKey verifies that DeleteKey removes from index but doesn't free cells.
func Test_Append_DeleteKey(t *testing.T) {
	strategy, h, cleanup := setupAppendStrategy(t)
	defer cleanup()

	// Create key
	path := []string{"_StrategyTest_Append", "KeyToDelete"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Record hive size before delete
	sizeBefore := len(h.Bytes())

	// Delete key
	err = strategy.DeleteKey(context.Background(), path, false)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify hive did NOT shrink (append-only: orphaned cells remain)
	sizeAfter := len(h.Bytes())
	if sizeAfter < sizeBefore {
		t.Errorf("Hive shrank after delete (should be append-only): before=%d, after=%d", sizeBefore, sizeAfter)
	}

	t.Logf("DeleteKey successful: hive size unchanged (append-only): before=%d, after=%d", sizeBefore, sizeAfter)
}

// Test_Append_EnsureKey_Idempotent verifies that EnsureKey is idempotent.
func Test_Append_EnsureKey_Idempotent(t *testing.T) {
	strategy, _, cleanup := setupAppendStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_Append", "IdempotentKey"}

	// First call: create
	ref1, keysCreated1, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("First EnsureKey failed: %v", err)
	}

	// Second call: should return existing
	ref2, keysCreated2, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("Second EnsureKey failed: %v", err)
	}
	if keysCreated2 > 0 {
		t.Errorf("Expected keysCreated=0 on second call (key already exists)")
	}

	if ref1 != ref2 {
		t.Errorf("EnsureKey not idempotent: ref1=%d, ref2=%d", ref1, ref2)
	}

	t.Logf("EnsureKey idempotency verified: ref=%d, keysCreated1=%v, keysCreated2=%v", ref1, keysCreated1, keysCreated2)
}

// Test_Append_MonotonicGrowth verifies that the hive only grows (never shrinks).
func Test_Append_MonotonicGrowth(t *testing.T) {
	strategy, h, cleanup := setupAppendStrategy(t)
	defer cleanup()

	initialSize := len(h.Bytes())
	t.Logf("Initial hive size: %d bytes", initialSize)

	// Perform multiple operations
	operations := []struct {
		name string
		fn   func() error
	}{
		{"Create Key", func() error {
			_, _, err := strategy.EnsureKey(context.Background(), []string{"MonotonicTest", "Key1"})
			return err
		}},
		{"Set Value", func() error {
			return strategy.SetValue(context.Background(), []string{"MonotonicTest", "Key1"}, "Val1", format.REGSZ, []byte("data"))
		}},
		{"Update Value", func() error {
			return strategy.SetValue(
				context.Background(),
				[]string{"MonotonicTest", "Key1"},
				"Val1",
				format.REGSZ,
				[]byte("updated_data"),
			)
		}},
		{"Delete Value", func() error {
			return strategy.DeleteValue(context.Background(), []string{"MonotonicTest", "Key1"}, "Val1")
		}},
		{"Create Another Key", func() error {
			_, _, err := strategy.EnsureKey(context.Background(), []string{"MonotonicTest", "Key2"})
			return err
		}},
		{"Delete Key", func() error {
			return strategy.DeleteKey(context.Background(), []string{"MonotonicTest", "Key2"}, false)
		}},
	}

	previousSize := initialSize
	for i, op := range operations {
		if err := op.fn(); err != nil {
			t.Fatalf("Operation %d (%s) failed: %v", i+1, op.name, err)
		}

		currentSize := len(h.Bytes())
		if currentSize < previousSize {
			t.Errorf("Hive shrank after operation %d (%s): %d → %d bytes (should be monotonic)",
				i+1, op.name, previousSize, currentSize)
		}

		t.Logf("After %s: %d bytes (Δ+%d)", op.name, currentSize, currentSize-previousSize)
		previousSize = currentSize
	}

	finalSize := len(h.Bytes())
	if finalSize < initialSize {
		t.Errorf("Hive shrank overall: initial=%d, final=%d", initialSize, finalSize)
	}

	t.Logf("Monotonic growth verified: %d → %d bytes (+%d, %+.1f%%)",
		initialSize, finalSize, finalSize-initialSize,
		float64(finalSize-initialSize)/float64(initialSize)*100)
}
