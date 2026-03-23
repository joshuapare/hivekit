package write_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	v2 "github.com/joshuapare/hivekit/hive/merge/v2"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/stretchr/testify/require"
)

// readCellSize reads the cell size header (int32) at the given cell ref.
// Negative = allocated, positive = free.
func readCellSize(t *testing.T, h *hive.Hive, cellRef uint32) int32 {
	t.Helper()
	data := h.Bytes()
	offset := int(format.HeaderSize) + int(cellRef)
	require.Less(t, offset+4, len(data), "cell ref 0x%X beyond hive bounds", cellRef)
	return int32(format.ReadU32(data, offset))
}

// getRootSubkeyListRef returns the subkey list ref from the root NK cell.
func getRootSubkeyListRef(t *testing.T, h *hive.Hive) uint32 {
	t.Helper()
	payload, err := h.ResolveCellPayload(h.RootCellOffset())
	require.NoError(t, err)
	rootNK, err := hive.ParseNK(payload)
	require.NoError(t, err)
	return rootNK.SubkeyListOffsetRel()
}

// getNodeNK resolves a key path and returns the NK.
func getNodeNK(t *testing.T, h *hive.Hive, keyPath []string) hive.NK {
	t.Helper()
	currentRef := h.RootCellOffset()

	for _, segment := range keyPath {
		found := false
		err := walker.WalkSubkeys(h, currentRef, func(childNK hive.NK, ref uint32) error {
			name := childNK.Name()
			if name != nil && string(name) == segment {
				currentRef = ref
				found = true
			}
			return nil
		})
		require.NoError(t, err)
		require.True(t, found, "key segment %q not found", segment)
	}

	payload, err := h.ResolveCellPayload(currentRef)
	require.NoError(t, err)
	nk, err := hive.ParseNK(payload)
	require.NoError(t, err)
	return nk
}

func TestCellFree_SubkeyListFreed(t *testing.T) {
	h := setupMinimalHive(t)
	ctx := context.Background()

	// Create 5 children under the root to establish a subkey list.
	createOps := make([]merge.Op, 5)
	for i := range createOps {
		createOps[i] = merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: []string{fmt.Sprintf("FreeTest%d", i)},
		}
	}
	_, err := v2.Merge(ctx, h, createOps, v2.Options{})
	require.NoError(t, err)

	// Record the current subkey list ref — this is the cell that should be freed.
	oldListRef := getRootSubkeyListRef(t, h)
	require.NotEqual(t, uint32(format.InvalidOffset), oldListRef, "root must have a subkey list")

	// Verify it's allocated (negative size).
	oldSize := readCellSize(t, h, oldListRef)
	require.Less(t, oldSize, int32(0), "old subkey list cell should be allocated (negative size)")

	// Add 3 more children — this forces a subkey list rebuild, which should free the old cell.
	addOps := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"FreeTestNew1"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"FreeTestNew2"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"FreeTestNew3"}},
	}
	_, err = v2.Merge(ctx, h, addOps, v2.Options{})
	require.NoError(t, err)

	// The old subkey list cell should now be freed (positive size).
	newSize := readCellSize(t, h, oldListRef)
	require.Greater(t, newSize, int32(0),
		"old subkey list cell should be freed (positive size), got %d", newSize)

	// The new subkey list ref should be different from the old one.
	newListRef := getRootSubkeyListRef(t, h)
	require.NotEqual(t, oldListRef, newListRef,
		"new subkey list should be at a different offset than the old one")
}

func TestCellFree_ValueListFreed(t *testing.T) {
	h := setupMinimalHive(t)
	ctx := context.Background()

	// Create a key with some values.
	setupOps := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"ValueFreeTest"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ValueFreeTest"},
			ValueName: "Val1",
			ValueType: format.REGSZ,
			Data:      []byte("hello\x00"),
		},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ValueFreeTest"},
			ValueName: "Val2",
			ValueType: format.REGSZ,
			Data:      []byte("world\x00"),
		},
	}
	_, err := v2.Merge(ctx, h, setupOps, v2.Options{})
	require.NoError(t, err)

	// Record old value list ref.
	nk := getNodeNK(t, h, []string{"ValueFreeTest"})
	oldValueListRef := nk.ValueListOffsetRel()
	require.NotEqual(t, uint32(format.InvalidOffset), oldValueListRef)

	oldSize := readCellSize(t, h, oldValueListRef)
	require.Less(t, oldSize, int32(0), "old value list cell should be allocated")

	// Add another value — forces value list rebuild.
	addOps := []merge.Op{
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ValueFreeTest"},
			ValueName: "Val3",
			ValueType: format.REGSZ,
			Data:      []byte("new\x00"),
		},
	}
	_, err = v2.Merge(ctx, h, addOps, v2.Options{})
	require.NoError(t, err)

	// Old value list cell should be freed.
	newSize := readCellSize(t, h, oldValueListRef)
	require.Greater(t, newSize, int32(0),
		"old value list cell should be freed (positive size), got %d", newSize)
}

func TestCellFree_ReplacedVKAndDataFreed(t *testing.T) {
	h := setupMinimalHive(t)
	ctx := context.Background()

	// Create a key with a value that has external data (> 4 bytes).
	longData := make([]byte, 64)
	for i := range longData {
		longData[i] = byte(i)
	}
	setupOps := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"ReplaceTest"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ReplaceTest"},
			ValueName: "BigVal",
			ValueType: format.REGBinary,
			Data:      longData,
		},
	}
	_, err := v2.Merge(ctx, h, setupOps, v2.Options{})
	require.NoError(t, err)

	// Find the VK cell ref for "BigVal" by reading the value list.
	oldVKRef, oldDataRef := findVKAndDataRef(t, h, []string{"ReplaceTest"}, "BigVal")
	require.NotEqual(t, uint32(format.InvalidOffset), oldVKRef)
	require.NotEqual(t, uint32(format.InvalidOffset), oldDataRef)

	// Both should be allocated.
	require.Less(t, readCellSize(t, h, oldVKRef), int32(0), "old VK should be allocated")
	require.Less(t, readCellSize(t, h, oldDataRef), int32(0), "old data cell should be allocated")

	// Replace the value with different-sized data.
	newData := make([]byte, 128)
	for i := range newData {
		newData[i] = byte(i + 100)
	}
	replaceOps := []merge.Op{
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ReplaceTest"},
			ValueName: "BigVal",
			ValueType: format.REGBinary,
			Data:      newData,
		},
	}
	_, err = v2.Merge(ctx, h, replaceOps, v2.Options{})
	require.NoError(t, err)

	// Old VK and data cells should be freed.
	require.Greater(t, readCellSize(t, h, oldVKRef), int32(0),
		"old VK cell should be freed")
	require.Greater(t, readCellSize(t, h, oldDataRef), int32(0),
		"old data cell should be freed")
}

func TestCellFree_HiveGrowthBounded(t *testing.T) {
	h := setupMinimalHive(t)
	ctx := context.Background()

	// Create initial children.
	ops := make([]merge.Op, 10)
	for i := range ops {
		ops[i] = merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: []string{fmt.Sprintf("GrowthTest%d", i)},
		}
	}
	_, err := v2.Merge(ctx, h, ops, v2.Options{})
	require.NoError(t, err)

	sizeBefore := h.Size()

	// Add 5 more children — triggers subkey list rebuild.
	addOps := make([]merge.Op, 5)
	for i := range addOps {
		addOps[i] = merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: []string{fmt.Sprintf("GrowthTestMore%d", i)},
		}
	}
	_, err = v2.Merge(ctx, h, addOps, v2.Options{})
	require.NoError(t, err)

	sizeAfter := h.Size()
	growth := sizeAfter - sizeBefore

	// Each NK is ~100 bytes, so 5 keys ≈ 500 bytes plus a new subkey list.
	// With 4K HBIN alignment, growth should be ≤ 8192.
	require.LessOrEqual(t, growth, int64(8192),
		"hive growth should be bounded; got %d bytes", growth)
}

// findVKAndDataRef locates a VK cell by name and returns its ref and data cell ref.
func findVKAndDataRef(t *testing.T, h *hive.Hive, keyPath []string, valueName string) (vkRef, dataRef uint32) {
	t.Helper()
	nk := getNodeNK(t, h, keyPath)
	valueListRef := nk.ValueListOffsetRel()
	valueCount := nk.ValueCount()
	require.NotEqual(t, uint32(format.InvalidOffset), valueListRef)

	payload, err := h.ResolveCellPayload(valueListRef)
	require.NoError(t, err)

	for i := uint32(0); i < valueCount; i++ {
		ref := format.ReadU32(payload, int(i)*format.DWORDSize)
		vkPayload, err := h.ResolveCellPayload(ref)
		require.NoError(t, err)
		vk, err := hive.ParseVK(vkPayload)
		require.NoError(t, err)

		nameBytes := vk.Name()
		if string(nameBytes) == valueName {
			vkRef = ref
			if !vk.IsSmallData() && vk.DataLen() > 0 {
				dataRef = vk.DataOffsetRel()
			}
			return vkRef, dataRef
		}
	}
	t.Fatalf("value %q not found under key %v", valueName, keyPath)
	return 0, 0
}
