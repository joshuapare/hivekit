package link_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/link"
)

// =============================================================================
// Context Cancellation Tests for Link Package
// =============================================================================

func TestLinkSubtree_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	createTestHive(t, parentPath)
	createTestHive(t, childPath)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// LinkSubtree should fail with cancelled context
	_, err := link.LinkSubtree(ctx, parentPath, childPath, link.LinkOptions{
		MountPath:        "Mounted",
		ImportRootValues: true,
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestLinkSubtreeComponents_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	createTestHive(t, parentPath)
	createTestHive(t, childPath)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// LinkSubtreeComponents should fail with cancelled context
	_, err := link.LinkSubtreeComponents(ctx, parentPath, childPath, "Mounted")

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestLinkSubtree_Success(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	createTestHive(t, parentPath)
	createChildHive(t, childPath)

	// Use valid context
	ctx := context.Background()

	stats, err := link.LinkSubtree(ctx, parentPath, childPath, link.LinkOptions{
		MountPath:        "Mounted",
		ImportRootValues: true,
		ConflictStrategy: link.ConflictOverwrite,
	})

	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.KeysCreated, 0)
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

func createChildHive(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create some structure to link
	err = b.SetString([]string{"ChildKey1"}, "Name", "Value1")
	require.NoError(t, err)

	err = b.SetString([]string{"ChildKey2"}, "Name", "Value2")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}
