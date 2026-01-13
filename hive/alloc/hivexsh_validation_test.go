//go:build linux || darwin

package alloc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/tx"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// Test_Hivexsh_MultipleGrows validates that hivexsh can parse a hive
// that has been grown multiple times. This is the definitive test that
// proves the trailing garbage bug is fixed.
func Test_Hivexsh_MultipleGrows(t *testing.T) {
	// Use a REAL test hive
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test-multi-grow.hiv")

	// Copy the real test hive to temp location
	srcData, err := os.ReadFile(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	if writeErr := os.WriteFile(hivePath, srcData, 0644); writeErr != nil {
		t.Fatalf("Failed to copy test hive: %v", writeErr)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Create REAL dirty tracker
	dt := dirty.NewTracker(h)

	// Create REAL transaction manager
	txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

	// Create allocator
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Perform multiple Grow() operations
	for i := range 5 {
		t.Logf("Grow iteration %d", i+1)

		// Begin transaction
		require.NoError(t, txMgr.Begin(context.Background()))

		// Grow by 8KB
		err = fa.GrowByPages(2) // Add 8KB HBIN
		require.NoError(t, err)

		// Commit transaction
		require.NoError(t, txMgr.Commit(context.Background()))
	}

	// Close hive to ensure all data is flushed
	h.Close()

	// ============================================================
	// CRITICAL TEST: Validate with hivexsh
	// ============================================================

	if !hivexval.IsHivexshAvailable() {
		t.Skip("hivexsh not available, skipping validation")
	}

	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{UseHivexsh: true}))
	defer v.Close()

	v.AssertHivexshValid(t)

	t.Logf("hivexsh successfully parsed the hive after %d Grow() operations!", 5)
	t.Logf("   This confirms the trailing garbage bug is FIXED.")
}
