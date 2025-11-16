package link_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/link"
)

func TestLinkSubtree_BasicLinking(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive with existing data
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()

	err = pb.SetString([]string{"SOFTWARE", "Existing"}, "Value", "Parent")
	require.NoError(t, err)
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with data to link
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"ChildApp", "Config"}, "Version", "1.0")
	require.NoError(t, err)
	err = cb.SetDWORD([]string{"ChildApp", "Config"}, "Enabled", 1)
	require.NoError(t, err)
	err = cb.Commit()
	require.NoError(t, err)

	// Link child under SOFTWARE\Apps
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE\\Apps",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Greater(t, stats.KeysCreated, 0)
	require.Greater(t, stats.ValuesSet, 0)

	// Verify the structure
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	// Check that child data exists under mount path
	version, err := h.GetString("SOFTWARE\\Apps\\ChildApp\\Config", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0", version)

	enabled, err := h.GetDWORD("SOFTWARE\\Apps\\ChildApp\\Config", "Enabled")
	require.NoError(t, err)
	require.Equal(t, uint32(1), enabled)

	// Check that parent data is still there
	parentValue, err := h.GetString("SOFTWARE\\Existing", "Value")
	require.NoError(t, err)
	require.Equal(t, "Parent", parentValue)
}

func TestLinkSubtree_WithFlatten(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with SYSTEM as first key
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"SYSTEM", "Setup"}, "Version", "10.0")
	require.NoError(t, err)
	err = cb.Commit()
	require.NoError(t, err)

	// Link with flatten - mounting under SYSTEM with flatten=true
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SYSTEM",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: true,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.True(t, stats.FlattenApplied)

	// Verify structure is SYSTEM\Setup, not SYSTEM\SYSTEM\Setup
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	version, err := h.GetString("SYSTEM\\Setup", "Version")
	require.NoError(t, err)
	require.Equal(t, "10.0", version)

	// Verify SYSTEM\SYSTEM\Setup does NOT exist
	_, err = h.Find("SYSTEM\\SYSTEM\\Setup")
	require.Error(t, err, "flattened path should not exist")
}

func TestLinkSubtree_ImportRootValues(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with root values
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	// Set values on root key
	err = cb.SetString([]string{}, "RootValue", "FromChild")
	require.NoError(t, err)
	err = cb.SetDWORD([]string{}, "RootNumber", 42)
	require.NoError(t, err)

	// Also add a subkey
	err = cb.SetString([]string{"SubKey"}, "Data", "Test")
	require.NoError(t, err)

	err = cb.Commit()
	require.NoError(t, err)

	// Link with ImportRootValues=true
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE\\Component",
		ImportRootValues:             true,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Greater(t, stats.ValuesSet, 0)

	// Verify root values were copied to mount path
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	rootValue, err := h.GetString("SOFTWARE\\Component", "RootValue")
	require.NoError(t, err)
	require.Equal(t, "FromChild", rootValue)

	rootNumber, err := h.GetDWORD("SOFTWARE\\Component", "RootNumber")
	require.NoError(t, err)
	require.Equal(t, uint32(42), rootNumber)

	// Verify subkey is also there
	subData, err := h.GetString("SOFTWARE\\Component\\SubKey", "Data")
	require.NoError(t, err)
	require.Equal(t, "Test", subData)
}

func TestLinkSubtree_ConflictOverwrite(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive with existing value
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()

	err = pb.SetString([]string{"SOFTWARE", "App"}, "Version", "1.0")
	require.NoError(t, err)
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with conflicting value
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"App"}, "Version", "2.0")
	require.NoError(t, err)
	err = cb.Commit()
	require.NoError(t, err)

	// Link with ConflictOverwrite
	_, err = link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)

	// Verify the value was overwritten
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	version, err := h.GetString("SOFTWARE\\App", "Version")
	require.NoError(t, err)
	require.Equal(t, "2.0", version, "should be overwritten by child value")
}

func TestLinkSubtree_ConflictSkip(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive with existing value
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()

	err = pb.SetString([]string{"SOFTWARE", "App"}, "Version", "1.0")
	require.NoError(t, err)
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with conflicting value
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"App"}, "Version", "2.0")
	require.NoError(t, err)
	err = cb.Commit()
	require.NoError(t, err)

	// Link with ConflictSkip
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictSkip,
	})
	require.NoError(t, err)
	require.Greater(t, stats.Conflicts, 0, "should have detected conflicts")

	// Verify the original value was kept
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	version, err := h.GetString("SOFTWARE\\App", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0", version, "should keep original value")
}

func TestLinkSubtree_ConflictError(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive with existing value
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()

	err = pb.SetString([]string{"SOFTWARE", "App"}, "Version", "1.0")
	require.NoError(t, err)
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with conflicting value
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"App"}, "Version", "2.0")
	require.NoError(t, err)
	err = cb.Commit()
	require.NoError(t, err)

	// Link with ConflictError - should fail
	_, err = link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictError,
	})
	require.Error(t, err, "should error on conflict")
	require.Contains(t, err.Error(), "conflict", "error should mention conflict")
}

func TestLinkSubtreeComponents(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "component.hive")

	// Create parent hive
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	// Create component hive with root values and subkeys
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	// Root values for component metadata
	err = cb.SetString([]string{}, "ComponentID", "12345")
	require.NoError(t, err)
	err = cb.SetString([]string{}, "Version", "1.0.0")
	require.NoError(t, err)

	// Component subkeys
	err = cb.SetString([]string{"Files", "System32"}, "kernel32.dll", "c:\\windows\\system32\\kernel32.dll")
	require.NoError(t, err)

	err = cb.Commit()
	require.NoError(t, err)

	// Link using convenience wrapper
	stats, err := link.LinkSubtreeComponents(
		parentPath,
		childPath,
		"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide\\Components",
	)
	require.NoError(t, err)
	require.Greater(t, stats.ValuesSet, 0)

	// Verify structure
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	// Root values should be at mount point
	componentID, err := h.GetString("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide\\Components", "ComponentID")
	require.NoError(t, err)
	require.Equal(t, "12345", componentID)

	version, err := h.GetString("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide\\Components", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", version)

	// Subkeys should be under mount point
	kernel32, err := h.GetString("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide\\Components\\Files\\System32", "kernel32.dll")
	require.NoError(t, err)
	require.Equal(t, "c:\\windows\\system32\\kernel32.dll", kernel32)
}

func TestLinkSubtree_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	// Create child hive with multiple nested keys
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()

	err = cb.SetString([]string{"Level1", "Level2", "Level3"}, "Deep", "Value")
	require.NoError(t, err)
	err = cb.SetString([]string{"Another", "Path"}, "Data", "Test")
	require.NoError(t, err)

	err = cb.Commit()
	require.NoError(t, err)

	// Link under SOFTWARE
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Greater(t, stats.KeysCreated, 3, "should create multiple keys")

	// Verify all paths exist
	h, err := hive.Open(parentPath)
	require.NoError(t, err)
	defer h.Close()

	deep, err := h.GetString("SOFTWARE\\Level1\\Level2\\Level3", "Deep")
	require.NoError(t, err)
	require.Equal(t, "Value", deep)

	data, err := h.GetString("SOFTWARE\\Another\\Path", "Data")
	require.NoError(t, err)
	require.Equal(t, "Test", data)
}

func TestLinkSubtree_EmptyChild(t *testing.T) {
	dir := t.TempDir()
	parentPath := filepath.Join(dir, "parent.hive")
	childPath := filepath.Join(dir, "child.hive")

	// Create parent hive
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	// Create empty child hive
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()
	err = cb.Commit()
	require.NoError(t, err)

	// Link empty child - should succeed with no operations
	stats, err := link.LinkSubtree(parentPath, childPath, link.LinkOptions{
		MountPath:                    "SOFTWARE\\Empty",
		ImportRootValues:             false,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             link.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Equal(t, 0, stats.KeysCreated, "empty hive should create no keys")
	require.Equal(t, 0, stats.ValuesSet, "empty hive should set no values")
}

func TestLinkSubtree_InvalidPaths(t *testing.T) {
	dir := t.TempDir()
	childPath := filepath.Join(dir, "child.hive")

	// Create child hive
	cb, err := builder.New(childPath, nil)
	require.NoError(t, err)
	defer cb.Close()
	err = cb.Commit()
	require.NoError(t, err)

	// Link with non-existent parent hive
	_, err = link.LinkSubtree("/nonexistent/parent.hive", childPath, link.LinkOptions{
		MountPath:        "SOFTWARE",
		ConflictStrategy: link.ConflictOverwrite,
	})
	require.Error(t, err, "should error on non-existent parent")

	// Link with non-existent child hive
	parentPath := filepath.Join(dir, "parent.hive")
	pb, err := builder.New(parentPath, nil)
	require.NoError(t, err)
	defer pb.Close()
	err = pb.Commit()
	require.NoError(t, err)

	_, err = link.LinkSubtree(parentPath, "/nonexistent/child.hive", link.LinkOptions{
		MountPath:        "SOFTWARE",
		ConflictStrategy: link.ConflictOverwrite,
	})
	require.Error(t, err, "should error on non-existent child")
}
