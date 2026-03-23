package write_test

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	v2 "github.com/joshuapare/hivekit/hive/merge/v2"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/stretchr/testify/require"
)

const minimalHivePath = "testdata/minimal"

func resolveTestPath(t *testing.T, relativePath string) string {
	t.Helper()
	candidates := []string{
		relativePath,
		"../../" + relativePath,
		"../../../" + relativePath,
		"../../../../" + relativePath,
		"../../../../../" + relativePath,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skipf("Test fixture %q not found", relativePath)
	return ""
}

func setupMinimalHive(t *testing.T) *hive.Hive {
	t.Helper()
	srcPath := resolveTestPath(t, minimalHivePath)
	dstPath := filepath.Join(t.TempDir(), "test-hive")

	src, err := os.Open(srcPath)
	if err != nil {
		t.Skipf("Minimal test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	require.NoError(t, err)
	defer dst.Close()

	_, err = io.Copy(dst, src)
	require.NoError(t, err)

	h, err := hive.Open(dstPath)
	require.NoError(t, err)
	t.Cleanup(func() { h.Close() })
	return h
}

// readSKRefcount reads the reference count from the SK cell at the given cell ref.
func readSKRefcount(t *testing.T, h *hive.Hive, skCellRef uint32) uint32 {
	t.Helper()
	payload, err := h.ResolveCellPayload(skCellRef)
	require.NoError(t, err, "resolve SK cell payload")
	require.GreaterOrEqual(t, len(payload), format.SKReferenceCountOffset+4, "SK cell too small")
	return binary.LittleEndian.Uint32(payload[format.SKReferenceCountOffset : format.SKReferenceCountOffset+4])
}

// getRootSKRef returns the SK cell ref from the root NK cell.
func getRootSKRef(t *testing.T, h *hive.Hive) uint32 {
	t.Helper()
	rootPayload, err := h.ResolveCellPayload(h.RootCellOffset())
	require.NoError(t, err)
	rootNK, err := hive.ParseNK(rootPayload)
	require.NoError(t, err)
	skRef := rootNK.SecurityOffsetRel()
	require.NotEqual(t, format.InvalidOffset, skRef, "root NK must have an SK cell")
	return skRef
}

func TestSKRefcount_NewKeysIncrementParentSK(t *testing.T) {
	h := setupMinimalHive(t)
	ctx := context.Background()

	// Read the root's SK cell refcount before merge.
	skRef := getRootSKRef(t, h)
	refcountBefore := readSKRefcount(t, h, skRef)

	// Create 5 new child keys under the root — all inherit root's SK cell.
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"SKTest1"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"SKTest2"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"SKTest3"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"SKTest4"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"SKTest5"}},
	}

	result, err := v2.Merge(ctx, h, ops, v2.Options{})
	require.NoError(t, err)
	require.Equal(t, 5, result.KeysCreated)

	// Read the SK refcount after merge — should have increased by 5.
	refcountAfter := readSKRefcount(t, h, skRef)
	require.Equal(t, refcountBefore+5, refcountAfter,
		"SK refcount should increase by the number of new keys inheriting it (before=%d, after=%d)",
		refcountBefore, refcountAfter)
}
