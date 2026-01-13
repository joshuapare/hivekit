package dirty_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
)

// =============================================================================
// Context Cancellation Tests for Dirty Package
// =============================================================================

func TestTracker_FlushDataOnly_PreCancelled(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := dirty.NewTracker(h)

	// Add some dirty ranges
	tracker.Add(4096, 100)
	tracker.Add(8192, 200)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// FlushDataOnly should fail with cancelled context
	err := tracker.FlushDataOnly(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestTracker_FlushHeaderAndMeta_PreCancelled(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := dirty.NewTracker(h)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// FlushHeaderAndMeta should fail with cancelled context
	err := tracker.FlushHeaderAndMeta(ctx, dirty.FlushAuto)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestTracker_FlushDataOnly_Success(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := dirty.NewTracker(h)

	// Add some dirty ranges
	tracker.Add(4096, 100)

	// Use valid context
	ctx := context.Background()

	err := tracker.FlushDataOnly(ctx)
	require.NoError(t, err)
}

func TestTracker_FlushHeaderAndMeta_Success(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := dirty.NewTracker(h)

	// Use valid context
	ctx := context.Background()

	err := tracker.FlushHeaderAndMeta(ctx, dirty.FlushAuto)
	require.NoError(t, err)
}

func TestTracker_FlushDataOnly_EmptyWithCancelled(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := dirty.NewTracker(h)

	// No dirty ranges added - should return nil immediately even with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Empty flush should succeed even with cancelled context (early return path)
	err := tracker.FlushDataOnly(ctx)

	// Note: Empty flush returns nil before checking context
	require.NoError(t, err)
}

// --- Helper Functions ---

func setupTestHive(t *testing.T) (*hive.Hive, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "test.hiv")

	// Create minimal hive file (8KB for testing)
	data := make([]byte, 8192)
	copy(data[0:4], []byte("regf"))

	err := os.WriteFile(hivePath, data, 0644)
	require.NoError(t, err)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}
