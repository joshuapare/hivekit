package merge_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/pkg/types"
)

func TestMergeRegTextWithPrefix_SimplePrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext without any prefix
	regText := `Windows Registry Editor Version 5.00

[Microsoft\Windows]
"Version"="10.0"
"Build"=dword:00004e20
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)
	require.Greater(t, applied.ValuesSet, 0)

	// Verify the keys were created under SOFTWARE
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Check SOFTWARE\Microsoft\Windows exists
	nid, err := h.Find("SOFTWARE\\Microsoft\\Windows")
	require.NoError(t, err)
	require.NotEqual(t, 0, nid)

	// Verify values
	version, err := h.GetString("SOFTWARE\\Microsoft\\Windows", "Version")
	require.NoError(t, err)
	require.Equal(t, "10.0", version)

	build, err := h.GetDWORD("SOFTWARE\\Microsoft\\Windows", "Build")
	require.NoError(t, err)
	require.Equal(t, uint32(20000), build)
}

func TestMergeRegTextWithPrefix_NestedPrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with simple paths
	regText := `Windows Registry Editor Version 5.00

[CurrentVersion\SideBySide]
"Enabled"=dword:00000001
`

	// Apply with nested prefix
	applied, err := merge.MergeRegTextWithPrefix(
		context.Background(),
		hivePath,
		regText,
		"SOFTWARE\\Microsoft\\Windows",
		nil,
	)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify the key structure
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Should be at SOFTWARE\Microsoft\Windows\CurrentVersion\SideBySide
	enabled, err := h.GetDWORD("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide", "Enabled")
	require.NoError(t, err)
	require.Equal(t, uint32(1), enabled)
}

func TestMergeRegTextWithPrefix_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with multiple keys
	regText := `Windows Registry Editor Version 5.00

[App1]
"Version"="1.0"

[App2]
"Version"="2.0"

[App3\SubKey]
"Data"="test"
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 2) // At least App1, App2, App3

	// Verify all keys exist
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	app1Version, err := h.GetString("SOFTWARE\\App1", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0", app1Version)

	app2Version, err := h.GetString("SOFTWARE\\App2", "Version")
	require.NoError(t, err)
	require.Equal(t, "2.0", app2Version)

	app3Data, err := h.GetString("SOFTWARE\\App3\\SubKey", "Data")
	require.NoError(t, err)
	require.Equal(t, "test", app3Data)
}

func TestMergeRegTextWithPrefix_ExistingHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with existing data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"SOFTWARE", "Existing"}, "Value", "Old")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Merge new data with SOFTWARE prefix
	regText := `Windows Registry Editor Version 5.00

[New]
"Value"="Fresh"
`

	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify both old and new data exist
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	oldValue, err := h.GetString("SOFTWARE\\Existing", "Value")
	require.NoError(t, err)
	require.Equal(t, "Old", oldValue)

	newValue, err := h.GetString("SOFTWARE\\New", "Value")
	require.NoError(t, err)
	require.Equal(t, "Fresh", newValue)
}

func TestMergeRegTextWithPrefix_EmptyPrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext
	regText := `Windows Registry Editor Version 5.00

[TestKey]
"Value"="Data"
`

	// Apply with empty prefix (no prepending)
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify key is at root level
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value, err := h.GetString("TestKey", "Value")
	require.NoError(t, err)
	require.Equal(t, "Data", value)
}

func TestMergeRegTextWithPrefix_PrefixWithHiveRoot(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext
	regText := `Windows Registry Editor Version 5.00

[Test]
"Value"="Data"
`

	// Apply with prefix that includes hive root (should be stripped)
	applied, err := merge.MergeRegTextWithPrefix(
		context.Background(),
		hivePath,
		regText,
		"HKEY_LOCAL_MACHINE\\SOFTWARE",
		nil,
	)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify key is under SOFTWARE (not HKEY_LOCAL_MACHINE\SOFTWARE)
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value, err := h.GetString("SOFTWARE\\Test", "Value")
	require.NoError(t, err)
	require.Equal(t, "Data", value)
}

func TestMergeRegTextWithPrefix_DeleteOperations(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with existing data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"SOFTWARE", "ToDelete"}, "Value", "Remove")
	require.NoError(t, err)
	err = b.SetString([]string{"SOFTWARE", "ToKeep"}, "Value", "Keep")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with delete operations
	regText := `Windows Registry Editor Version 5.00

[-ToDelete]
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysDeleted, 0)

	// Verify ToDelete is gone and ToKeep remains
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	_, err = h.Find("SOFTWARE\\ToDelete")
	require.Error(t, err, "deleted key should not exist")

	keepValue, err := h.GetString("SOFTWARE\\ToKeep", "Value")
	require.NoError(t, err)
	require.Equal(t, "Keep", keepValue)
}

func TestMergeRegTextWithPrefix_AllValueTypes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with various basic value types
	regText := `Windows Registry Editor Version 5.00

[Types]
"String"="Hello"
"Binary"=hex:01,02,03,04
"DWORD"=dword:12345678
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.ValuesSet, 2)

	// Verify values
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	str, err := h.GetString("SOFTWARE\\Types", "String")
	require.NoError(t, err)
	require.Equal(t, "Hello", str)

	_, binaryData, err := h.GetValue("SOFTWARE\\Types", "Binary")
	require.NoError(t, err)
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, binaryData)

	dw, err := h.GetDWORD("SOFTWARE\\Types", "DWORD")
	require.NoError(t, err)
	require.Equal(t, uint32(0x12345678), dw)
}

func TestMergeRegTextWithPrefix_InvalidRegText(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Invalid regtext (no header)
	regText := `[InvalidKey]
"Value"="Data"
`

	// Should error during parsing
	_, err = merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.Error(t, err, "should error on invalid regtext")
}

func TestMergeRegTextWithPrefix_InvalidHivePath(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[Test]
"Value"="Data"
`

	// Should error on non-existent hive
	_, err := merge.MergeRegTextWithPrefix(context.Background(), "/nonexistent/hive", regText, "SOFTWARE", nil)
	require.Error(t, err, "should error on invalid hive path")
}

// =============================================================================
// PlanFromRegTextWithPrefix Tests
// =============================================================================

func TestPlanFromRegTextWithPrefix_Basic(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[Microsoft\Windows]
"Version"="10.0"
`

	plan, err := merge.PlanFromRegTextWithPrefix(regText, "SOFTWARE")
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Should have ops for creating key path and setting value
	require.Greater(t, len(plan.Ops), 0)

	// Verify the key path is transformed
	// The ops should reference SOFTWARE\Microsoft\Windows
	found := false
	for _, op := range plan.Ops {
		if op.Type == merge.OpSetValue {
			// KeyPath should be ["SOFTWARE", "Microsoft", "Windows"]
			require.Equal(t, []string{"SOFTWARE", "Microsoft", "Windows"}, op.KeyPath)
			require.Equal(t, "Version", op.ValueName)
			found = true
		}
	}
	require.True(t, found, "expected to find OpSetValue with transformed path")
}

func TestPlanFromRegTextWithPrefix_EmptyPrefix(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[TestKey]
"Value"="Data"
`

	// Empty prefix should work like PlanFromRegText
	plan, err := merge.PlanFromRegTextWithPrefix(regText, "")
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Verify path is not prefixed
	for _, op := range plan.Ops {
		if op.Type == merge.OpSetValue {
			require.Equal(t, []string{"TestKey"}, op.KeyPath)
		}
	}
}

func TestPlanFromRegTextWithPrefix_HiveRootStripping(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Test]
"Value"="Data"
`

	// Prefix includes HKEY_LOCAL_MACHINE which should be stripped
	plan, err := merge.PlanFromRegTextWithPrefix(regText, "HKLM\\SOFTWARE")
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Both prefix HKLM and path HKEY_LOCAL_MACHINE should be stripped
	for _, op := range plan.Ops {
		if op.Type == merge.OpSetValue {
			// Should be SOFTWARE\Test (prefix stripped of HKLM, path stripped of HKEY_LOCAL_MACHINE)
			require.Equal(t, []string{"SOFTWARE", "Test"}, op.KeyPath)
		}
	}
}

func TestPlanFromRegTextWithPrefix_MultipleOperations(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[Key1]
"Val1"="Data1"

[Key2]
"Val2"=dword:00000001

[-Key3]
`

	plan, err := merge.PlanFromRegTextWithPrefix(regText, "Root")
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Count operation types
	var ensureKeys, setValues, deleteKeys int
	for _, op := range plan.Ops {
		switch op.Type {
		case merge.OpEnsureKey:
			ensureKeys++
		case merge.OpSetValue:
			setValues++
			// All set values should have Root prefix
			require.Equal(t, "Root", op.KeyPath[0])
		case merge.OpDeleteKey:
			deleteKeys++
			require.Equal(t, []string{"Root", "Key3"}, op.KeyPath)
		}
	}

	require.Equal(t, 2, setValues, "expected 2 SetValue ops")
	require.Equal(t, 1, deleteKeys, "expected 1 DeleteKey op")
}

func TestPlanFromRegTextWithPrefix_InvalidRegText(t *testing.T) {
	// No header - should fail
	regText := `[Test]
"Value"="Data"
`

	_, err := merge.PlanFromRegTextWithPrefix(regText, "SOFTWARE")
	require.Error(t, err, "should error on invalid regtext")
}

func TestPlanFromRegTextWithPrefix_DeleteValue(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[TestKey]
"ToDelete"=-
`

	plan, err := merge.PlanFromRegTextWithPrefix(regText, "SOFTWARE")
	require.NoError(t, err)

	// Should have a delete value operation
	found := false
	for _, op := range plan.Ops {
		if op.Type == merge.OpDeleteValue {
			require.Equal(t, []string{"SOFTWARE", "TestKey"}, op.KeyPath)
			require.Equal(t, "ToDelete", op.ValueName)
			found = true
		}
	}
	require.True(t, found, "expected to find OpDeleteValue")
}

// =============================================================================
// AllowMissingHeader Tests (surfaced through merge APIs)
// =============================================================================

func TestMergeRegTextWithPrefix_AllowMissingHeader(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext without header
	regText := `[Test\Key]
"Value"="Data"
`

	// Should fail without AllowMissingHeader
	_, err = merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// Should succeed with AllowMissingHeader
	opts := &merge.Options{
		ParseOptions: types.RegParseOptions{
			AllowMissingHeader: true,
		},
	}
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", opts)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify the key was created
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value, err := h.GetString("SOFTWARE\\Test\\Key", "Value")
	require.NoError(t, err)
	require.Equal(t, "Data", value)
}

func TestMergeRegText_AllowMissingHeader(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext without header
	regText := `[TestKey]
"Value"="Data"
`

	// Should fail without AllowMissingHeader
	_, err = merge.MergeRegText(context.Background(), hivePath, regText, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// Should succeed with AllowMissingHeader
	opts := &merge.Options{
		ParseOptions: types.RegParseOptions{
			AllowMissingHeader: true,
		},
	}
	applied, err := merge.MergeRegText(context.Background(), hivePath, regText, opts)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)
}

func TestSession_ApplyRegTextWithPrefix_AllowMissingHeader(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext without header
	regText := `[Test\Key]
"Value"="Data"
`

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Session with AllowMissingHeader
	opts := merge.Options{
		ParseOptions: types.RegParseOptions{
			AllowMissingHeader: true,
		},
	}
	sess, err := merge.NewSession(context.Background(), h, opts)
	require.NoError(t, err)
	defer sess.Close(context.Background())

	// Should succeed with AllowMissingHeader in session opts
	applied, err := sess.ApplyRegTextWithPrefix(context.Background(), regText, "SOFTWARE")
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)
}

func TestPlanFromRegTextOpts_AllowMissingHeader(t *testing.T) {
	// Regtext without header
	regText := `[TestKey]
"Value"="Data"
`

	// Should fail with default options
	_, err := merge.PlanFromRegText(regText)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// Should succeed with AllowMissingHeader
	plan, err := merge.PlanFromRegTextOpts(regText, types.RegParseOptions{
		AllowMissingHeader: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Ops)
}

func TestPlanFromRegTextWithPrefixOpts_AllowMissingHeader(t *testing.T) {
	// Regtext without header
	regText := `[Test\Key]
"Value"="Data"
`

	// Should fail with default options
	_, err := merge.PlanFromRegTextWithPrefix(regText, "SOFTWARE")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// Should succeed with AllowMissingHeader
	plan, err := merge.PlanFromRegTextWithPrefixOpts(regText, "SOFTWARE", types.RegParseOptions{
		AllowMissingHeader: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Ops)

	// Verify the path includes prefix
	for _, op := range plan.Ops {
		if op.Type == merge.OpSetValue {
			require.Equal(t, "SOFTWARE", op.KeyPath[0])
		}
	}
}
