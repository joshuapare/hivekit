package tx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/tx"
	"github.com/joshuapare/hivekit/internal/format"
)

// =============================================================================
// Context Cancellation Tests for Transaction Package
// =============================================================================

func TestManager_Begin_PreCancelled(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	tm := tx.NewManager(h, dt, dirty.FlushAuto)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Begin should fail with cancelled context
	err := tm.Begin(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestManager_Commit_PreCancelled(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	tm := tx.NewManager(h, dt, dirty.FlushAuto)

	// Begin with valid context
	err := tm.Begin(context.Background())
	require.NoError(t, err)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Commit should fail with cancelled context
	err = tm.Commit(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)

	// Rollback to clean up
	tm.Rollback()
}

func TestManager_BeginCommit_Success(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	tm := tx.NewManager(h, dt, dirty.FlushAuto)

	ctx := context.Background()

	// Begin should succeed
	err := tm.Begin(ctx)
	require.NoError(t, err)
	require.True(t, tm.InTransaction())

	// Commit should succeed
	err = tm.Commit(ctx)
	require.NoError(t, err)
	require.False(t, tm.InTransaction())
}

func TestManager_DeadlineExceeded(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	tm := tx.NewManager(h, dt, dirty.FlushAuto)

	// Use already-expired deadline
	ctx, cancel := context.WithDeadline(context.Background(),
		// Use a time in the past
		format.FiletimeToTime(0))
	defer cancel()

	// Begin should fail with deadline exceeded
	err := tm.Begin(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded),
		"expected context.DeadlineExceeded, got: %v", err)
}

// --- Helper Functions ---

func setupTestHive(t *testing.T) (*hive.Hive, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "test.hiv")

	// Create minimal hive file with valid REGF header
	data := make([]byte, format.HeaderSize)

	// Write REGF signature
	copy(data[format.REGFSignatureOffset:], format.REGFSignature)

	// Initialize sequences to known values
	format.PutU32(data, format.REGFPrimarySeqOffset, 100)
	format.PutU32(data, format.REGFSecondarySeqOffset, 100)

	err := os.WriteFile(hivePath, data, 0644)
	require.NoError(t, err)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}
