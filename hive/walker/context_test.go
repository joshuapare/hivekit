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
