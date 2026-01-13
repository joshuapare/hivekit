package strategy

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

// lowerPath converts a path to lowercase for case-insensitive index lookups.
func lowerPath(path []string) []string {
	result := make([]string, len(path))
	for i, p := range path {
		result[i] = strings.ToLower(p)
	}
	return result
}

// setupTestStrategy creates a test hive and strategy for testing.
func setupTestStrategy(t *testing.T) (*InPlace, func()) {
	testHivePath := "../../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "strategy-test-hive")
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

	// Create strategy
	strategy := NewInPlace(h, allocator, dt, idx).(*InPlace)

	cleanup := func() {
		h.Close()
	}

	return strategy, cleanup
}

// Test 1: InPlace - EnsureKey.
func Test_InPlace_EnsureKey(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	// Create a unique test key
	path := []string{"_StrategyTest_InPlace", "TestKey"}

	// Ensure key
	nkRef, keysCreated, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}

	// Verify key was created
	if keysCreated == 0 {
		t.Log("Key already existed (acceptable in test hive)")
	}

	// Verify reference is valid
	if nkRef == 0 {
		t.Error("EnsureKey returned zero reference")
	}

	// Verify key exists in index using WalkPath (must lowercase for case-insensitive lookup)
	nkOff, ok := index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if !ok {
		t.Error("Key not found in index after EnsureKey")
	}
	if nkOff == 0 {
		t.Error("WalkPath returned zero offset")
	}

	// Verify dirty tracker has ranges
	if strategy.dt == nil {
		t.Fatal("DirtyTracker is nil")
	}

	t.Logf("EnsureKey successful: nkRef=%d, keysCreated=%v", nkRef, keysCreated)
}

// Test 2: InPlace - SetValue (small data).
func Test_InPlace_SetValue_Small(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_InPlace", "SmallValue"}
	valueName := "TestValue"
	valueData := []byte("Hello, World!\x00")

	// Set value (REG_SZ)
	err := strategy.SetValue(context.Background(), path, valueName, format.REGSZ, valueData)
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Verify parent key exists in index
	nkOff, ok := index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if !ok {
		t.Fatal("Parent key not found in index after SetValue")
	}

	// Verify value exists in index (must lowercase for case-insensitive lookup)
	vkOff, ok := strategy.idx.GetVK(nkOff, strings.ToLower(valueName))
	if !ok {
		t.Error("Value not found in index after SetValue")
	}
	if vkOff == 0 {
		t.Error("GetVK returned zero offset")
	}

	t.Logf("SetValue successful: path=%v, name=%s, vkOff=%d", path, valueName, vkOff)
}

// Test 3: InPlace - SetValue (large data).
func Test_InPlace_SetValue_Large(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_InPlace", "LargeValue"}
	valueName := "LargeData"
	// Create 50KB of data (larger than 16KB DB threshold)
	valueData := bytes.Repeat([]byte{0xAB}, 50*1024)

	// Set value (REG_BINARY)
	err := strategy.SetValue(context.Background(), path, valueName, format.REGBinary, valueData)
	if err != nil {
		t.Fatalf("SetValue (large) failed: %v", err)
	}

	// Verify parent key and value exist in index
	nkOff, ok := index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if !ok {
		t.Fatal("Key not found in index after SetValue (large)")
	}

	vkOff, ok := strategy.idx.GetVK(nkOff, strings.ToLower(valueName))
	if !ok {
		t.Error("Large value not found in index after SetValue")
	}

	t.Logf(
		"SetValue (large) successful: path=%v, name=%s, size=%d bytes, vkOff=%d",
		path,
		valueName,
		len(valueData),
		vkOff,
	)
}

// Test 4: InPlace - DeleteValue.
func Test_InPlace_DeleteValue(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_InPlace", "DeleteTest"}
	valueName := "ToDelete"

	// First, create a value
	err := strategy.SetValue(context.Background(), path, valueName, format.REGSZ, []byte("delete me\x00"))
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Verify it exists
	nkOff, ok := index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if !ok {
		t.Fatal("Key not found after SetValue")
	}

	vkOff, ok := strategy.idx.GetVK(nkOff, strings.ToLower(valueName))
	if !ok {
		t.Fatal("Value not found before delete")
	}
	if vkOff == 0 {
		t.Fatal("GetVK returned zero offset before delete")
	}

	// Delete the value
	err = strategy.DeleteValue(context.Background(), path, valueName)
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	// Verify it's gone
	_, ok = strategy.idx.GetVK(nkOff, strings.ToLower(valueName))
	if ok {
		t.Error("Value still exists in index after DeleteValue")
	}

	t.Log("DeleteValue successful")
}

// Test 5: InPlace - DeleteKey.
func Test_InPlace_DeleteKey(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_InPlace", "DeleteKeyTest"}

	// First, create a key
	_, keysCreated, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("EnsureKey failed: %v", err)
	}
	if keysCreated == 0 {
		t.Log("Key already existed")
	}

	// Verify it exists
	nkOff, ok := index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if !ok {
		t.Fatal("Key not found in index before delete")
	}
	if nkOff == 0 {
		t.Fatal("WalkPath returned zero offset")
	}

	// Delete the key (non-recursive for now)
	err = strategy.DeleteKey(context.Background(), path, false)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify it's gone from index
	_, ok = index.WalkPath(strategy.idx, strategy.rootRef, lowerPath(path)...)
	if ok {
		t.Error("Key still in index after DeleteKey")
	}

	t.Log("DeleteKey successful")
}

// Test 6: Idempotency - EnsureKey twice.
func Test_InPlace_EnsureKey_Idempotent(t *testing.T) {
	strategy, cleanup := setupTestStrategy(t)
	defer cleanup()

	path := []string{"_StrategyTest_InPlace", "IdempotentKey"}

	// First call
	nkRef1, _, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("First EnsureKey failed: %v", err)
	}

	// Second call (should be idempotent)
	nkRef2, keysCreated2, err := strategy.EnsureKey(context.Background(), path)
	if err != nil {
		t.Fatalf("Second EnsureKey failed: %v", err)
	}

	// Verify idempotency
	if nkRef1 != nkRef2 {
		t.Errorf("EnsureKey not idempotent: first=%d, second=%d", nkRef1, nkRef2)
	}

	if keysCreated2 > 0 {
		t.Error("Second EnsureKey reported keysCreated > 0 (should be 0)")
	}

	t.Logf("EnsureKey idempotency verified: ref=%d", nkRef1)
}
