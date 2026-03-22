package v2_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	v2 "github.com/joshuapare/hivekit/hive/merge/v2"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/stretchr/testify/require"
)

// minimalHivePath is the relative path to the minimal test hive that ships
// with the repository (no need for 'make download-hives').
const minimalHivePath = "testdata/minimal"

// setupMinimalHive copies the minimal test hive to a temp file and opens it.
// Returns the hive, the temp path, and a cleanup function.
func setupMinimalHive(t *testing.T) (*hive.Hive, string, func()) {
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

	return h, dstPath, func() { h.Close() }
}

// resolveTestPath tries multiple relative path prefixes to find a test fixture.
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

// collectKeys performs a recursive walk of the hive from the root NK cell,
// collecting all key paths into a map for structural comparison.
func collectKeys(t *testing.T, h *hive.Hive) map[string]bool {
	t.Helper()
	keys := make(map[string]bool)
	rootOff := h.RootCellOffset()
	rootPayload, err := h.ResolveCellPayload(rootOff)
	require.NoError(t, err, "resolve root cell")
	rootNK, err := hive.ParseNK(rootPayload)
	require.NoError(t, err, "parse root NK")
	collectKeysRecursive(t, h, rootNK, "", keys)
	return keys
}

func collectKeysRecursive(t *testing.T, h *hive.Hive, nk hive.NK, prefix string, keys map[string]bool) {
	t.Helper()
	count := nk.SubkeyCount()
	if count == 0 {
		return
	}
	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		return
	}

	result, err := nk.ResolveSubkeyList(h)
	if err != nil {
		return
	}

	var refs []uint32
	switch result.Kind {
	case hive.ListLH:
		for i := 0; i < result.LH.Count(); i++ {
			refs = append(refs, result.LH.Entry(i).Cell())
		}
	case hive.ListLF:
		for i := 0; i < result.LF.Count(); i++ {
			refs = append(refs, result.LF.Entry(i).Cell())
		}
	default:
		return
	}

	for _, ref := range refs {
		payload, err := h.ResolveCellPayload(ref)
		if err != nil {
			continue
		}
		childNK, err := hive.ParseNK(payload)
		if err != nil {
			continue
		}
		name := string(childNK.Name())
		var path string
		if prefix == "" {
			path = name
		} else {
			path = prefix + "\\" + name
		}
		keys[strings.ToLower(path)] = true
		collectKeysRecursive(t, h, childNK, path, keys)
	}
}

// collectValues performs a recursive walk collecting all value names at each key path.
func collectValues(t *testing.T, h *hive.Hive) map[string][]string {
	t.Helper()
	vals := make(map[string][]string)
	rootOff := h.RootCellOffset()
	rootPayload, err := h.ResolveCellPayload(rootOff)
	require.NoError(t, err, "resolve root cell")
	rootNK, err := hive.ParseNK(rootPayload)
	require.NoError(t, err, "parse root NK")
	collectValuesRecursive(t, h, rootNK, "", vals)
	return vals
}

func collectValuesRecursive(t *testing.T, h *hive.Hive, nk hive.NK, prefix string, vals map[string][]string) {
	t.Helper()

	// Collect values at this key.
	if nk.ValueCount() > 0 && nk.ValueListOffsetRel() != format.InvalidOffset {
		vl, err := nk.ResolveValueList(h)
		if err == nil {
			for i := 0; i < vl.Count(); i++ {
				vkRef, vkErr := vl.VKOffsetAt(i)
				if vkErr != nil {
					continue
				}
				vkPayload, resolveErr := h.ResolveCellPayload(vkRef)
				if resolveErr != nil {
					continue
				}
				vk, parseErr := hive.ParseVK(vkPayload)
				if parseErr != nil {
					continue
				}
				valueName := string(vk.Name())
				vals[strings.ToLower(prefix)] = append(vals[strings.ToLower(prefix)], strings.ToLower(valueName))
			}
		}
	}

	// Recurse into subkeys.
	count := nk.SubkeyCount()
	if count == 0 {
		return
	}
	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		return
	}

	result, err := nk.ResolveSubkeyList(h)
	if err != nil {
		return
	}

	var refs []uint32
	switch result.Kind {
	case hive.ListLH:
		for i := 0; i < result.LH.Count(); i++ {
			refs = append(refs, result.LH.Entry(i).Cell())
		}
	case hive.ListLF:
		for i := 0; i < result.LF.Count(); i++ {
			refs = append(refs, result.LF.Entry(i).Cell())
		}
	default:
		return
	}

	for _, ref := range refs {
		payload, err := h.ResolveCellPayload(ref)
		if err != nil {
			continue
		}
		childNK, err := hive.ParseNK(payload)
		if err != nil {
			continue
		}
		name := string(childNK.Name())
		var path string
		if prefix == "" {
			path = name
		} else {
			path = prefix + "\\" + name
		}
		collectValuesRecursive(t, h, childNK, path, vals)
	}
}

func TestMerge_CreateKeys(t *testing.T) {
	// Setup: two independent copies of the same minimal test hive.
	h1, v1Path, cleanup1 := setupMinimalHive(t)
	cleanup1() // Close h1 immediately; v1 MergePlan opens by path.
	_ = h1

	h2, _, cleanup2 := setupMinimalHive(t)
	defer cleanup2()

	ctx := context.Background()

	// Use simple top-level keys (minimal hive may have no existing subkeys).
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"V2TestKey"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"V2TestKey", "SubKey1"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"V2TestKey", "SubKey2"}},
	}

	// Apply via v1 (takes file path, opens internally).
	plan := merge.NewPlan()
	for _, op := range ops {
		plan.AddEnsureKey(op.KeyPath)
	}
	_, err := merge.MergePlan(ctx, v1Path, plan, nil)
	require.NoError(t, err, "v1 merge")

	// Reopen v1 hive to read committed results.
	h1Again, err := hive.Open(v1Path)
	require.NoError(t, err)
	defer h1Again.Close()

	// Apply via v2 (takes opened *hive.Hive).
	result, err := v2.Merge(ctx, h2, ops, v2.Options{})
	require.NoError(t, err, "v2 merge")
	require.Greater(t, result.KeysCreated, 0, "v2 should create keys")

	// Structural comparison: verify the new keys exist in both hives.
	v1Keys := collectKeys(t, h1Again)
	v2Keys := collectKeys(t, h2)

	for _, key := range []string{
		"v2testkey",
		"v2testkey\\subkey1",
		"v2testkey\\subkey2",
	} {
		require.True(t, v1Keys[key], "v1 missing key: %s", key)
		require.True(t, v2Keys[key], "v2 missing key: %s", key)
	}
}

func TestMerge_SetValues(t *testing.T) {
	h, _, cleanup := setupMinimalHive(t)
	defer cleanup()

	ctx := context.Background()

	// Create a key and set values on it.
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"V2ValTest"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"V2ValTest"},
			ValueName: "DWORDVal",
			ValueType: format.REGDWORD,
			Data:      []byte{0x42, 0x00, 0x00, 0x00},
		},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"V2ValTest"},
			ValueName: "StringVal",
			ValueType: format.REGSZ,
			Data:      append([]byte("hello world"), 0x00, 0x00),
		},
	}

	result, err := v2.Merge(ctx, h, ops, v2.Options{})
	require.NoError(t, err, "v2 merge set values")
	require.Equal(t, 1, result.KeysCreated, "should create 1 key")
	require.Equal(t, 2, result.ValuesSet, "should set 2 values")

	// Verify the values exist by walking the hive.
	vals := collectValues(t, h)
	keyPath := strings.ToLower("V2ValTest")
	require.Contains(t, vals, keyPath, "key should have values")
	valueNames := vals[keyPath]
	require.Contains(t, valueNames, "dwordval")
	require.Contains(t, valueNames, "stringval")
}

func TestMerge_EmptyOps(t *testing.T) {
	h, _, cleanup := setupMinimalHive(t)
	defer cleanup()

	ctx := context.Background()

	// Empty ops should succeed without error.
	result, err := v2.Merge(ctx, h, nil, v2.Options{})
	require.NoError(t, err, "empty ops merge")
	require.Equal(t, 0, result.KeysCreated)
	require.Equal(t, 0, result.ValuesSet)
}

func TestMerge_PhaseTiming(t *testing.T) {
	h, _, cleanup := setupMinimalHive(t)
	defer cleanup()

	ctx := context.Background()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"TimingTest"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"TimingTest"},
			ValueName: "Val1",
			ValueType: format.REGDWORD,
			Data:      []byte{0x01, 0x00, 0x00, 0x00},
		},
	}

	result, err := v2.Merge(ctx, h, ops, v2.Options{})
	require.NoError(t, err, "merge for timing")

	// All phase timings should be non-zero for a merge with actual work.
	require.Greater(t, result.PhaseTiming.Parse, time.Duration(0), "Parse timing should be positive")
	require.Greater(t, result.PhaseTiming.Walk, time.Duration(0), "Walk timing should be positive")
	require.Greater(t, result.PhaseTiming.Plan, time.Duration(0), "Plan timing should be positive")
	require.Greater(t, result.PhaseTiming.Write, time.Duration(0), "Write timing should be positive")
	require.Greater(t, result.PhaseTiming.Flush, time.Duration(0), "Flush timing should be positive")
	require.Greater(t, result.PhaseTiming.Total, time.Duration(0), "Total timing should be positive")

	// Total should be >= sum of individual phases.
	sumPhases := result.PhaseTiming.Parse +
		result.PhaseTiming.Walk +
		result.PhaseTiming.Plan +
		result.PhaseTiming.Write +
		result.PhaseTiming.Flush
	require.GreaterOrEqual(t, result.PhaseTiming.Total, sumPhases,
		"Total should be >= sum of phases")
}
