package e2e

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

// MergeTestCase defines a complete E2E merge test.
type MergeTestCase struct {
	Name           string
	BaseHive       string // Path relative to testdata/suite
	Operations     func(*merge.Plan)
	ExpectedStats  merge.Applied
	VerifyKeys     []KeyVerification
	VerifyValues   []ValueVerification
	VerifyNotExist []string // Key paths that should NOT exist
}

// KeyVerification specifies a key that should exist after merge.
type KeyVerification struct {
	Path []string
}

// ValueVerification specifies a value that should exist after merge.
type ValueVerification struct {
	KeyPath   []string
	ValueName string
	// We don't verify data content in E2E (that's unit test territory)
	// Just that the value exists
}

// Test_Merge_E2E runs table-driven E2E merge tests.
func Test_Merge_E2E(t *testing.T) {
	testCases := []MergeTestCase{
		{
			Name:     "AddNewKey",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Add a new top-level key that doesn't exist
				p.AddEnsureKey([]string{"E2ETestKey"})
				p.AddSetValue(
					[]string{"E2ETestKey"},
					"TestValue",
					format.REGSZ,
					[]byte("TestData\x00"),
				)
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   1,
				KeysDeleted:   0,
				ValuesSet:     1,
				ValuesDeleted: 0,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"E2ETestKey"}},
			},
			VerifyValues: []ValueVerification{
				{KeyPath: []string{"E2ETestKey"}, ValueName: "TestValue"},
			},
		},
		{
			Name:     "AddNestedKeys",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Add nested keys under existing ControlSet001
				p.AddEnsureKey([]string{"ControlSet001", "E2ETest", "Nested", "Deep"})
				p.AddSetValue(
					[]string{"ControlSet001", "E2ETest"},
					"Level1",
					format.REGDWORD,
					[]byte{0x01, 0x00, 0x00, 0x00},
				)
				p.AddSetValue(
					[]string{"ControlSet001", "E2ETest", "Nested"},
					"Level2",
					format.REGDWORD,
					[]byte{0x02, 0x00, 0x00, 0x00},
				)
				p.AddSetValue(
					[]string{"ControlSet001", "E2ETest", "Nested", "Deep"},
					"Level3",
					format.REGDWORD,
					[]byte{0x03, 0x00, 0x00, 0x00},
				)
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   3, // E2ETest, Nested, Deep (ControlSet001 already exists)
				KeysDeleted:   0,
				ValuesSet:     3,
				ValuesDeleted: 0,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"ControlSet001", "E2ETest"}},
				{Path: []string{"ControlSet001", "E2ETest", "Nested"}},
				{Path: []string{"ControlSet001", "E2ETest", "Nested", "Deep"}},
			},
			VerifyValues: []ValueVerification{
				{KeyPath: []string{"ControlSet001", "E2ETest"}, ValueName: "Level1"},
				{KeyPath: []string{"ControlSet001", "E2ETest", "Nested"}, ValueName: "Level2"},
				{
					KeyPath:   []string{"ControlSet001", "E2ETest", "Nested", "Deep"},
					ValueName: "Level3",
				},
			},
		},
		{
			Name:     "UpdateExistingValue",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// ControlSet001\Control\CurrentUser exists - update it
				p.AddSetValue(
					[]string{"ControlSet001", "Control"},
					"CurrentUser",
					format.REGSZ,
					[]byte("UPDATED\x00"),
				)
				p.AddSetValue(
					[]string{"ControlSet001", "Control"},
					"NewValue",
					format.REGSZ,
					[]byte("NEW\x00"),
				)
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   0,
				KeysDeleted:   0,
				ValuesSet:     2,
				ValuesDeleted: 0,
			},
			VerifyValues: []ValueVerification{
				{KeyPath: []string{"ControlSet001", "Control"}, ValueName: "CurrentUser"},
				{KeyPath: []string{"ControlSet001", "Control"}, ValueName: "NewValue"},
			},
		},
		{
			Name:     "DeleteKey",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Create then delete
				p.AddEnsureKey([]string{"TempKey"})
				p.AddSetValue([]string{"TempKey"}, "TempValue", format.REGSZ, []byte("temp\x00"))
				p.AddDeleteKey([]string{"TempKey"})
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   1,
				KeysDeleted:   1,
				ValuesSet:     1,
				ValuesDeleted: 0, // Deleted via key deletion, not counted separately
			},
			VerifyNotExist: []string{"TempKey"},
		},
		{
			Name:     "DeleteValue",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Add then delete value
				p.AddEnsureKey([]string{"DeleteValueTest"})
				p.AddSetValue([]string{"DeleteValueTest"}, "Value1", format.REGSZ, []byte("v1\x00"))
				p.AddSetValue([]string{"DeleteValueTest"}, "Value2", format.REGSZ, []byte("v2\x00"))
				p.AddDeleteValue([]string{"DeleteValueTest"}, "Value1")
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   1,
				KeysDeleted:   0,
				ValuesSet:     2,
				ValuesDeleted: 1,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"DeleteValueTest"}},
			},
			VerifyValues: []ValueVerification{
				{KeyPath: []string{"DeleteValueTest"}, ValueName: "Value2"},
			},
		},
		{
			Name:     "LargeValueStorage",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Add large values (>16KB) to test DB format
				p.AddEnsureKey([]string{"LargeValueTest"})

				// 20KB value
				data20k := make([]byte, 20*1024)
				for i := range data20k {
					data20k[i] = byte(i % 256)
				}
				p.AddSetValue([]string{"LargeValueTest"}, "LargeValue20K", format.REGBinary, data20k)

				// 50KB value
				data50k := make([]byte, 50*1024)
				for i := range data50k {
					data50k[i] = byte((i + 1) % 256)
				}
				p.AddSetValue([]string{"LargeValueTest"}, "LargeValue50K", format.REGBinary, data50k)
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   1,
				KeysDeleted:   0,
				ValuesSet:     2,
				ValuesDeleted: 0,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"LargeValueTest"}},
			},
			VerifyValues: []ValueVerification{
				{KeyPath: []string{"LargeValueTest"}, ValueName: "LargeValue20K"},
				{KeyPath: []string{"LargeValueTest"}, ValueName: "LargeValue50K"},
			},
		},
		{
			Name:     "LargeValueDeletion",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Create and delete large value (tests our bug fix!)
				p.AddEnsureKey([]string{"LargeDelTest"})

				data := make([]byte, 30*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				p.AddSetValue([]string{"LargeDelTest"}, "LargeValue", format.REGBinary, data)
				p.AddDeleteValue([]string{"LargeDelTest"}, "LargeValue")
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   1,
				KeysDeleted:   0,
				ValuesSet:     1,
				ValuesDeleted: 1,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"LargeDelTest"}},
			},
			// LargeValue should NOT exist
		},
		{
			Name:     "ComplexMixedOperations",
			BaseHive: "windows-2003-server-system",
			Operations: func(p *merge.Plan) {
				// Complex realistic scenario
				p.AddEnsureKey([]string{"ControlSet001", "Services", "E2ETestService"})
				p.AddSetValue(
					[]string{"ControlSet001", "Services", "E2ETestService"},
					"DisplayName",
					format.REGSZ,
					[]byte("E2E Test Service\x00"),
				)
				p.AddSetValue(
					[]string{"ControlSet001", "Services", "E2ETestService"},
					"Start",
					format.REGDWORD,
					[]byte{0x02, 0x00, 0x00, 0x00},
				)

				// Add Parameters subkey
				p.AddEnsureKey(
					[]string{"ControlSet001", "Services", "E2ETestService", "Parameters"},
				)
				p.AddSetValue(
					[]string{"ControlSet001", "Services", "E2ETestService", "Parameters"},
					"ServiceDll",
					format.REGSZ,
					[]byte("test.dll\x00"),
				)

				// Add large config value
				configData := make([]byte, 25*1024)
				for i := range configData {
					configData[i] = byte(i % 256)
				}
				p.AddSetValue(
					[]string{"ControlSet001", "Services", "E2ETestService", "Parameters"},
					"ConfigData",
					format.REGBinary,
					configData,
				)
			},
			ExpectedStats: merge.Applied{
				KeysCreated:   2, // E2ETestService, Parameters (ControlSet001 and Services already exist)
				KeysDeleted:   0,
				ValuesSet:     4,
				ValuesDeleted: 0,
			},
			VerifyKeys: []KeyVerification{
				{Path: []string{"ControlSet001", "Services", "E2ETestService"}},
				{Path: []string{"ControlSet001", "Services", "E2ETestService", "Parameters"}},
			},
			VerifyValues: []ValueVerification{
				{
					KeyPath:   []string{"ControlSet001", "Services", "E2ETestService"},
					ValueName: "DisplayName",
				},
				{
					KeyPath:   []string{"ControlSet001", "Services", "E2ETestService"},
					ValueName: "Start",
				},
				{
					KeyPath: []string{
						"ControlSet001",
						"Services",
						"E2ETestService",
						"Parameters",
					},
					ValueName: "ServiceDll",
				},
				{
					KeyPath: []string{
						"ControlSet001",
						"Services",
						"E2ETestService",
						"Parameters",
					},
					ValueName: "ConfigData",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			runMergeTest(t, tc)
		})
	}
}

// runMergeTest executes a single merge test case.
func runMergeTest(t *testing.T, tc MergeTestCase) {
	// Step 1: Copy base hive to temp location
	tempDir := t.TempDir()
	tempHivePath := filepath.Join(tempDir, "test-hive")

	baseHivePath := filepath.Join("../../testdata/suite", tc.BaseHive)
	if err := copyFile(baseHivePath, tempHivePath); err != nil {
		t.Fatalf("Failed to copy base hive: %v", err)
	}

	// Step 2: Open hive and setup components
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	// CRITICAL: Build index from existing hive to avoid creating duplicate keys
	builder := walker.NewIndexBuilder(h, 10000, 10000)
	idx, err := builder.Build()
	if err != nil {
		h.Close()
		t.Fatalf("Failed to build index: %v", err)
	}

	// Create session (it will create its own allocator and dirty tracker)
	session, err := merge.NewSessionWithIndex(
		h,
		idx,
		merge.Options{Strategy: merge.StrategyInPlace},
	)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create session: %v", err)
	}

	// Step 3: Build and apply merge plan
	plan := merge.NewPlan()
	tc.Operations(plan)

	result, err := session.ApplyWithTx(plan)
	if err != nil {
		session.Close()
		h.Close()
		t.Fatalf("Merge execution failed: %v", err)
	}

	// Step 4: Verify exact statistics
	if result != tc.ExpectedStats {
		t.Errorf("Statistics mismatch:\n  Got:      %+v\n  Expected: %+v", result, tc.ExpectedStats)
	}

	// Step 4b: Close session (flushes dirty pages automatically)
	if closeErr := session.Close(); closeErr != nil {
		h.Close()
		t.Fatalf("Failed to close session: %v", closeErr)
	}

	// Close hive
	h.Close()

	// Step 5: Reopen hive and verify structure
	h, err = hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Step 6: Walk hive and verify expected keys exist
	rootOffset := h.RootCellOffset()

	for _, keyVerif := range tc.VerifyKeys {
		if !verifyKeyExists(t, h, rootOffset, keyVerif.Path) {
			t.Errorf("Expected key not found: %v", keyVerif.Path)
		}
	}

	// Step 7: Verify expected values exist
	for _, valVerif := range tc.VerifyValues {
		if !verifyValueExists(t, h, rootOffset, valVerif.KeyPath, valVerif.ValueName) {
			t.Errorf("Expected value not found: %v -> %s", valVerif.KeyPath, valVerif.ValueName)
		}
	}

	// Step 8: Verify keys that should NOT exist
	for _, notExistPath := range tc.VerifyNotExist {
		pathParts := strings.Split(notExistPath, "\\")
		if verifyKeyExists(t, h, rootOffset, pathParts) {
			t.Errorf("Key should not exist but does: %s", notExistPath)
		}
	}

	t.Logf("✓ Merge test passed: %d keys created, %d deleted, %d values set, %d deleted",
		result.KeysCreated, result.KeysDeleted, result.ValuesSet, result.ValuesDeleted)
}

// verifyKeyExists walks the hive to verify a key exists at the given path.
func verifyKeyExists(t *testing.T, h *hive.Hive, rootOffset uint32, path []string) bool {
	if len(path) == 0 {
		return true
	}

	currentOffset := rootOffset
	for _, keyName := range path {
		found := false
		keyNameLower := strings.ToLower(keyName)

		// Walk current key's subkeys
		err := walker.WalkSubkeys(h, currentOffset, func(nk hive.NK, ref uint32) error {
			name := nk.Name()
			nameStr := decodeNKName(name, nk.IsCompressedName())

			if strings.ToLower(nameStr) == keyNameLower {
				currentOffset = ref
				found = true
				return walker.ErrStopWalk
			}
			return nil
		})

		if err != nil && !errors.Is(err, walker.ErrStopWalk) {
			t.Logf("Walk error at %s: %v", keyName, err)
			return false
		}

		if !found {
			return false
		}
	}

	return true
}

// verifyValueExists walks the hive to verify a value exists.
func verifyValueExists(
	_ *testing.T,
	h *hive.Hive,
	rootOffset uint32,
	keyPath []string,
	valueName string,
) bool {
	// First find the key
	if len(keyPath) == 0 {
		return false
	}

	currentOffset := rootOffset
	for _, keyName := range keyPath {
		found := false
		keyNameLower := strings.ToLower(keyName)

		err := walker.WalkSubkeys(h, currentOffset, func(nk hive.NK, ref uint32) error {
			name := nk.Name()
			nameStr := decodeNKName(name, nk.IsCompressedName())

			if strings.ToLower(nameStr) == keyNameLower {
				currentOffset = ref
				found = true
				return walker.ErrStopWalk
			}
			return nil
		})

		if err != nil && !errors.Is(err, walker.ErrStopWalk) {
			return false
		}

		if !found {
			return false
		}
	}

	// Now check if value exists in this key
	valueNameLower := strings.ToLower(valueName)
	valueFound := false

	err := walker.WalkValues(h, currentOffset, func(vk hive.VK, _ uint32) error {
		name := vk.Name()
		nameStr := decodeVKName(name, vk.NameCompressed())

		if strings.ToLower(nameStr) == valueNameLower {
			valueFound = true
			return walker.ErrStopWalk
		}
		return nil
	})

	if err != nil && !errors.Is(err, walker.ErrStopWalk) {
		return false
	}

	return valueFound
}

// decodeNKName decodes an NK (key) name.
func decodeNKName(nameBytes []byte, compressed bool) string {
	if compressed {
		// ASCII
		return string(nameBytes)
	}
	// UTF-16LE - simple decode
	if len(nameBytes)%2 != 0 {
		nameBytes = nameBytes[:len(nameBytes)-1]
	}
	runes := make([]rune, len(nameBytes)/2)
	for i := 0; i < len(runes); i++ {
		runes[i] = rune(nameBytes[i*2]) | rune(nameBytes[i*2+1])<<8
	}
	return string(runes)
}

// decodeVKName decodes a VK (value) name.
func decodeVKName(nameBytes []byte, compressed bool) string {
	return decodeNKName(nameBytes, compressed) // Same logic
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
