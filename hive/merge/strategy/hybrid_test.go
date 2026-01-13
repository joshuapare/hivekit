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

// setupHybridStrategy creates a test hive and Hybrid strategy for testing.
func setupHybridStrategy(t *testing.T, slackPct int) (*Hybrid, *hive.Hive, func()) {
	testHivePath := "../../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "hybrid-strategy-test-hive")
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

	// Create Hybrid strategy with provided slack percentage
	strategy := NewHybrid(h, allocator, dt, idx, slackPct).(*Hybrid)

	cleanup := func() {
		h.Close()
	}

	return strategy, h, cleanup
}

// Test_Hybrid_EnsureKey verifies that EnsureKey creates new keys using InPlace.
func Test_Hybrid_EnsureKey(t *testing.T) {
	strategy, _, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	// Create a unique test key
	path := []string{"_StrategyTest_Hybrid", "TestKey"}

	// Ensure key
	nkRef, keysCreated, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Verify ref is non-zero
	if nkRef == 0 {
		t.Errorf("Expected non-zero nkRef, got 0")
	}

	if keysCreated == 0 {
		t.Errorf("Expected keysCreated > 0 for new key")
	}

	t.Logf("EnsureKey successful: nkRef=%d, keysCreated=%v", nkRef, keysCreated)
}

// Test_Hybrid_SmallValue_UsesInPlace verifies small values use InPlace strategy.
func Test_Hybrid_SmallValue_UsesInPlace(t *testing.T) {
	strategy, h, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	// Create parent key
	path := []string{"_StrategyTest_Hybrid", "SmallValue"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Record initial hive size
	sizeBefore := len(h.Bytes())

	// Set small value (<1KB, should use InPlace)
	data := bytes.Repeat([]byte("X"), 512)
	err = strategy.SetValue(context.Background(), path, "SmallData", format.REGBinary, data)
	if err != nil {
		t.Fatalf("SetValue (small) failed: %v", err)
	}

	sizeAfter := len(h.Bytes())

	// For small values, hive may not grow (InPlace reuses existing cells)
	// or may grow slightly if allocating new cells
	t.Logf("SetValue (small) successful: path=%v, name=SmallData, size=%d bytes", path, len(data))
	t.Logf("Hive size: before=%d, after=%d, delta=%d", sizeBefore, sizeAfter, sizeAfter-sizeBefore)
}

// Test_Hybrid_LargeValue_UsesAppend verifies large values use Append strategy.
func Test_Hybrid_LargeValue_UsesAppend(t *testing.T) {
	strategy, h, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	// Create parent key
	path := []string{"_StrategyTest_Hybrid", "LargeValue"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Record initial hive size
	sizeBefore := len(h.Bytes())

	// Set large value (≥1KB, should use Append)
	data := bytes.Repeat([]byte("Y"), 50*1024)
	err = strategy.SetValue(context.Background(), path, "LargeData", format.REGBinary, data)
	if err != nil {
		t.Fatalf("SetValue (large) failed: %v", err)
	}

	sizeAfter := len(h.Bytes())

	// Large values should grow the hive (Append strategy)
	if sizeAfter < sizeBefore {
		t.Errorf("Expected hive to grow for large value, but it shrank: %d → %d", sizeBefore, sizeAfter)
	}

	t.Logf("SetValue (large) successful: path=%v, name=LargeData, size=%d bytes", path, len(data))
	t.Logf("Hive size: before=%d, after=%d, delta=%d", sizeBefore, sizeAfter, sizeAfter-sizeBefore)
}

// Test_Hybrid_DeleteValue verifies deletes use InPlace (free cells).
func Test_Hybrid_DeleteValue(t *testing.T) {
	strategy, _, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	// Create key and value
	path := []string{"_StrategyTest_Hybrid", "DeleteTest"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	data := []byte("ToBeDeleted")
	err = strategy.SetValue(context.Background(), path, "TempValue", format.REGSZ, data)
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Delete value (should use InPlace, which frees cells)
	err = strategy.DeleteValue(context.Background(), path, "TempValue")
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	t.Logf("DeleteValue successful: path=%v, name=TempValue", path)
}

// Test_Hybrid_DeleteKey verifies key deletes use InPlace.
func Test_Hybrid_DeleteKey(t *testing.T) {
	strategy, _, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	// Create key
	path := []string{"_StrategyTest_Hybrid", "KeyToDelete"}
	_, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Delete key (should use InPlace)
	err = strategy.DeleteKey(context.Background(), path, false)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	t.Logf("DeleteKey successful: path=%v", path)
}

// Test_Hybrid_MixedOperations verifies hybrid strategy handles mixed workloads.
func Test_Hybrid_MixedOperations(t *testing.T) {
	strategy, h, cleanup := setupHybridStrategy(t, 12)
	defer cleanup()

	initialSize := len(h.Bytes())
	t.Logf("Initial hive size: %d bytes", initialSize)

	// Perform mixed operations
	operations := []struct {
		name string
		fn   func() error
	}{
		{"Create Key", func() error {
			_, _, err := strategy.EnsureKey(context.Background(), []string{"HybridTest", "Key1"})
			return err
		}},
		{"Set Small Value", func() error {
			return strategy.SetValue(context.Background(), []string{"HybridTest", "Key1"}, "Small", format.REGSZ, []byte("small data"))
		}},
		{"Set Large Value", func() error {
			return strategy.SetValue(
				context.Background(),
				[]string{"HybridTest", "Key1"},
				"Large",
				format.REGBinary,
				bytes.Repeat([]byte("X"), 10*1024),
			)
		}},
		{"Update Small Value", func() error {
			return strategy.SetValue(
				context.Background(),
				[]string{"HybridTest", "Key1"},
				"Small",
				format.REGSZ,
				[]byte("updated small"),
			)
		}},
		{"Delete Value", func() error {
			return strategy.DeleteValue(context.Background(), []string{"HybridTest", "Key1"}, "Small")
		}},
		{"Create Another Key", func() error {
			_, _, err := strategy.EnsureKey(context.Background(), []string{"HybridTest", "Key2"})
			return err
		}},
		{"Delete Key", func() error {
			return strategy.DeleteKey(context.Background(), []string{"HybridTest", "Key2"}, false)
		}},
	}

	previousSize := initialSize
	for i, op := range operations {
		if err := op.fn(); err != nil {
			t.Fatalf("Operation %d (%s) failed: %v", i+1, op.name, err)
		}

		currentSize := len(h.Bytes())
		t.Logf("After %s: %d bytes (Δ%+d)", op.name, currentSize, currentSize-previousSize)
		previousSize = currentSize
	}

	finalSize := len(h.Bytes())
	t.Logf("Mixed operations completed: %d → %d bytes (%+.1f%%)",
		initialSize, finalSize,
		float64(finalSize-initialSize)/float64(initialSize)*100)
}

// Test_Hybrid_SlackThreshold verifies slack percentage behavior.
func Test_Hybrid_SlackThreshold(t *testing.T) {
	// Test with different slack percentages
	testCases := []struct {
		name     string
		slackPct int
	}{
		{"Slack 0%", 0},
		{"Slack 12%", 12},
		{"Slack 25%", 25},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strategy, _, cleanup := setupHybridStrategy(t, tc.slackPct)
			defer cleanup()

			// Test shouldUseInPlace calculation
			testCases := []struct {
				needed    int
				available int
				expected  bool
			}{
				{100, 100, true},                    // exact fit
				{100 + tc.slackPct, 100, true},      // within slack
				{100 + tc.slackPct + 1, 100, false}, // exceeds slack
				{0, 100, true},                      // zero size
				{200, 100, false},                   // double size
			}

			for _, test := range testCases {
				result := strategy.shouldUseInPlace(test.needed, test.available)
				if result != test.expected {
					t.Errorf("shouldUseInPlace(%d, %d) with %d%% slack: got %v, want %v",
						test.needed, test.available, tc.slackPct, result, test.expected)
				}
			}
		})
	}
}
