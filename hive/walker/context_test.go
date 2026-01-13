package walker_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/walker"
)

// =============================================================================
// Context Cancellation Tests for Walker Package
// =============================================================================

func TestIndexBuilder_Build_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ib := walker.NewIndexBuilder(h, 1000, 1000)
	_, err = ib.Build(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestIndexBuilder_Build_DeadlineExceeded(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Use already-expired deadline
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	ib := walker.NewIndexBuilder(h, 1000, 1000)
	_, err = ib.Build(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded),
		"expected context.DeadlineExceeded, got: %v", err)
}

func TestIndexBuilder_Build_Success(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	ctx := context.Background()

	ib := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := ib.Build(ctx)

	require.NoError(t, err)
	require.NotNil(t, idx)
}

func TestIndexBuilder_Build_WithTimeout_Success(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Generous timeout - should succeed for small hive
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ib := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := ib.Build(ctx)

	require.NoError(t, err)
	require.NotNil(t, idx)
}

func TestIndexBuilder_Build_LargerHive_Cancellation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createLargerTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Channel to signal completion
	done := make(chan error, 1)

	go func() {
		ib := walker.NewIndexBuilder(h, 10000, 10000)
		_, err := ib.Build(ctx)
		done <- err
	}()

	// Cancel after a short delay
	time.Sleep(100 * time.Microsecond)
	cancel()

	// Wait for completion with timeout
	select {
	case err := <-done:
		// Either completed or cancelled - both are acceptable
		if err != nil {
			require.True(t, errors.Is(err, context.Canceled),
				"expected context.Canceled or nil, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Build did not respect cancellation within 5 seconds")
	}
}

// --- Helper Functions ---

func createTestHive(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Software"}, "Version", "1.0")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}

func createLargerTestHive(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create more keys to make the hive larger
	for i := 0; i < 100; i++ {
		path := []string{"Software", "App" + string(rune('A'+i%26)), "Key" + string(rune('0'+i%10))}
		err = b.SetString(path, "Value", "Data")
		require.NoError(t, err)
	}

	err = b.Commit()
	require.NoError(t, err)
}

// =============================================================================
// WalkSubkeysCtx Tests
// =============================================================================

func TestWalkSubkeysCtx_Success(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithSubkeys(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	ctx := context.Background()
	var subkeys []string

	err = walker.WalkSubkeysCtx(ctx, h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		subkeys = append(subkeys, string(nk.Name()))
		return nil
	})

	require.NoError(t, err)
	require.Len(t, subkeys, 3, "expected 3 subkeys: A, B, C")
}

func TestWalkSubkeysCtx_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithSubkeys(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = walker.WalkSubkeysCtx(ctx, h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		t.Error("callback should not be called with cancelled context")
		return nil
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got: %v", err)
}

func TestWalkSubkeysCtx_EarlyStop(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithSubkeys(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	ctx := context.Background()
	var count int

	err = walker.WalkSubkeysCtx(ctx, h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		count++
		if count >= 2 {
			return walker.ErrStopWalk
		}
		return nil
	})

	require.NoError(t, err, "ErrStopWalk should result in nil error")
	require.Equal(t, 2, count, "should have processed exactly 2 subkeys before stopping")
}

func TestWalkSubkeysCtx_CallbackError(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithSubkeys(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	ctx := context.Background()
	expectedErr := errors.New("test callback error")

	err = walker.WalkSubkeysCtx(ctx, h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		return expectedErr
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, expectedErr), "expected callback error, got: %v", err)
}

// =============================================================================
// WalkValuesCtx Tests
// =============================================================================

func TestWalkValuesCtx_Success(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithSubkeyValues(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	ctx := context.Background()

	// First, walk to find the "TestKey" subkey offset
	var testKeyOffset uint32
	err = walker.WalkSubkeysCtx(ctx, h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		if string(nk.Name()) == "TestKey" {
			testKeyOffset = ref
			return walker.ErrStopWalk
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, testKeyOffset, "TestKey not found")

	// Now walk values on that subkey
	var values []string
	err = walker.WalkValuesCtx(ctx, h, testKeyOffset, func(vk hive.VK, ref uint32) error {
		values = append(values, string(vk.Name()))
		return nil
	})

	require.NoError(t, err)
	require.Len(t, values, 2, "expected 2 values: Value1, Value2")
}

func TestWalkValuesCtx_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithValues(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	testKeyOffset := getTestKeyOffset(t, h)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = walker.WalkValuesCtx(ctx, h, testKeyOffset, func(vk hive.VK, ref uint32) error {
		t.Error("callback should not be called with cancelled context")
		return nil
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got: %v", err)
}

func TestWalkValuesCtx_EarlyStop(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithMultipleValues(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	testKeyOffset := getTestKeyOffset(t, h)

	ctx := context.Background()
	var count int

	err = walker.WalkValuesCtx(ctx, h, testKeyOffset, func(vk hive.VK, ref uint32) error {
		count++
		if count >= 2 {
			return walker.ErrStopWalk
		}
		return nil
	})

	require.NoError(t, err, "ErrStopWalk should result in nil error")
	require.Equal(t, 2, count, "should have processed exactly 2 values before stopping")
}

func TestWalkValuesCtx_CallbackError(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createHiveWithValues(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	testKeyOffset := getTestKeyOffset(t, h)

	ctx := context.Background()
	expectedErr := errors.New("test callback error")

	err = walker.WalkValuesCtx(ctx, h, testKeyOffset, func(vk hive.VK, ref uint32) error {
		return expectedErr
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, expectedErr), "expected callback error, got: %v", err)
}

// --- Additional Helper Functions ---

func createHiveWithSubkeys(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create multiple subkeys under root
	err = b.SetString([]string{"A"}, "Val", "data")
	require.NoError(t, err)
	err = b.SetString([]string{"B"}, "Val", "data")
	require.NoError(t, err)
	err = b.SetString([]string{"C"}, "Val", "data")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}

func createHiveWithValues(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create values on a subkey (we'll walk the subkey's values)
	err = b.SetString([]string{"TestKey"}, "Value1", "data1")
	require.NoError(t, err)
	err = b.SetString([]string{"TestKey"}, "Value2", "data2")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}

func createHiveWithSubkeyValues(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create values on a subkey
	err = b.SetString([]string{"TestKey"}, "Value1", "data1")
	require.NoError(t, err)
	err = b.SetString([]string{"TestKey"}, "Value2", "data2")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}

func createHiveWithMultipleValues(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create multiple values on a subkey
	err = b.SetString([]string{"TestKey"}, "Val1", "data1")
	require.NoError(t, err)
	err = b.SetString([]string{"TestKey"}, "Val2", "data2")
	require.NoError(t, err)
	err = b.SetString([]string{"TestKey"}, "Val3", "data3")
	require.NoError(t, err)
	err = b.SetString([]string{"TestKey"}, "Val4", "data4")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}

// getTestKeyOffset finds the offset of "TestKey" subkey under root.
func getTestKeyOffset(t *testing.T, h *hive.Hive) uint32 {
	t.Helper()
	var testKeyOffset uint32
	err := walker.WalkSubkeysCtx(context.Background(), h, h.RootCellOffset(), func(nk hive.NK, ref uint32) error {
		if string(nk.Name()) == "TestKey" {
			testKeyOffset = ref
			return walker.ErrStopWalk
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, testKeyOffset, "TestKey not found")
	return testKeyOffset
}
