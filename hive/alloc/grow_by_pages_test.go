//go:build linux || darwin

package alloc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_GrowByPages_SinglePage verifies GrowByPages(1) creates exactly 4KB HBIN.
func Test_GrowByPages_SinglePage(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Initial state: 4KB
	data := h.Bytes()
	initialSize := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(4096), initialSize, "Initial size should be 4KB")

	// Grow by 1 page (4KB)
	err = fa.GrowByPages(1)
	require.NoError(t, err)

	// Verify: should be 8KB now (4KB base + 4KB new HBIN)
	data = h.Bytes()
	afterSize := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(8192), afterSize, "After GrowByPages(1), size should be 8KB")

	t.Logf("GrowByPages(1): 4KB → 8KB (added exactly 4KB HBIN)")
}

// Test_GrowByPages_MultiplePages verifies GrowByPages(2) creates exactly 8KB HBIN.
func Test_GrowByPages_MultiplePages(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow by 2 pages (8KB)
	err = fa.GrowByPages(2)
	require.NoError(t, err)

	// Verify: should be 12KB now (4KB base + 8KB new HBIN)
	data := h.Bytes()
	afterSize := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(12288), afterSize, "After GrowByPages(2), size should be 12KB")

	t.Logf("GrowByPages(2): 4KB → 12KB (added exactly 8KB HBIN)")
}

// Test_GrowByPages_InvalidInput verifies error handling.
func Test_GrowByPages_InvalidInput(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Test invalid inputs
	err = fa.GrowByPages(0)
	require.Error(t, err, "GrowByPages(0) should return error")

	err = fa.GrowByPages(-1)
	require.Error(t, err, "GrowByPages(-1) should return error")

	t.Logf("GrowByPages() correctly rejects invalid inputs")
}

// Test_GrowByPages_UsableSpace verifies the usable space after growth.
func Test_GrowByPages_UsableSpace(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow by 1 page
	err = fa.GrowByPages(1)
	require.NoError(t, err)

	// Verify we can allocate 4064 bytes (4096 - 32 header)
	// This proves the HBIN header is PART OF the 4KB, not in addition
	ref, _, err := fa.Alloc(4064, ClassNK)
	require.NoError(t, err, "Should be able to allocate 4064 bytes from 4KB HBIN")
	require.NotZero(t, ref, "Allocation should succeed")

	t.Logf("4KB HBIN has 4064 bytes usable (4096 - 32 header) - spec-compliant")
}
