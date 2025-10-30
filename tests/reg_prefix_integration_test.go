package tests

import (
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/joshuapare/hivekit/pkg/types"
)

func TestParseRegWithManualPrefixStripping(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp]",
		"\"Version\"=\"1.0\"",
		"\"Count\"=dword:0000002a",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp\\SubKey]",
		"\"Data\"=hex:01,02,03,04",
		"",
		"[-HKEY_LOCAL_MACHINE\\SOFTWARE\\ObsoleteApp]",
		"",
	}, "\r\n")

	codec := regtext.NewCodec()
	ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	// Verify we got the expected operations
	if len(ops) != 6 {
		t.Fatalf("Expected 6 operations, got %d", len(ops))
	}

	// Check first operation - create TestApp key
	createKey1, ok := ops[0].(types.OpCreateKey)
	if !ok {
		t.Fatalf("Operation 0 should be OpCreateKey, got %T", ops[0])
	}
	if createKey1.Path != "TestApp" {
		t.Errorf("Expected path 'TestApp', got %q", createKey1.Path)
	}

	// Check second operation - set Version value
	setValue1, ok := ops[1].(types.OpSetValue)
	if !ok {
		t.Fatalf("Operation 1 should be OpSetValue, got %T", ops[1])
	}
	if setValue1.Path != "TestApp" {
		t.Errorf("Expected path 'TestApp', got %q", setValue1.Path)
	}
	if setValue1.Name != "Version" {
		t.Errorf("Expected name 'Version', got %q", setValue1.Name)
	}

	// Check fifth operation - create SubKey
	createKey2, ok := ops[3].(types.OpCreateKey)
	if !ok {
		t.Fatalf("Operation 3 should be OpCreateKey, got %T", ops[3])
	}
	if createKey2.Path != "TestApp\\SubKey" {
		t.Errorf("Expected path 'TestApp\\SubKey', got %q", createKey2.Path)
	}

	// Check last operation - delete key
	deleteKey, ok := ops[5].(types.OpDeleteKey)
	if !ok {
		t.Fatalf("Last operation should be OpDeleteKey, got %T", ops[5])
	}
	if deleteKey.Path != "ObsoleteApp" {
		t.Errorf("Expected delete path 'ObsoleteApp', got %q", deleteKey.Path)
	}
}

func TestParseRegWithAutoPrefixStripping(t *testing.T) {
	tests := []struct {
		name           string
		regContent     string
		expectedPath   string
		expectedOpType string
	}{
		{
			name: "SOFTWARE hive auto-detect",
			regContent: strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows]",
				"\"Version\"=\"10.0\"",
			}, "\r\n"),
			expectedPath:   "Microsoft\\Windows",
			expectedOpType: "OpCreateKey",
		},
		{
			name: "SYSTEM hive auto-detect",
			regContent: strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_LOCAL_MACHINE\\SYSTEM\\CurrentControlSet]",
				"\"Data\"=hex:01,02",
			}, "\r\n"),
			expectedPath:   "CurrentControlSet",
			expectedOpType: "OpCreateKey",
		},
		{
			name: "SAM hive auto-detect",
			regContent: strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_LOCAL_MACHINE\\SAM\\SAM\\Domains]",
				"\"Count\"=dword:00000001",
			}, "\r\n"),
			expectedPath:   "SAM\\Domains",
			expectedOpType: "OpCreateKey",
		},
		{
			name: "SECURITY hive auto-detect",
			regContent: strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_LOCAL_MACHINE\\SECURITY\\Policy]",
				"\"Value\"=\"test\"",
			}, "\r\n"),
			expectedPath:   "Policy",
			expectedOpType: "OpCreateKey",
		},
		{
			name: "NTUSER.DAT (HKCU) auto-detect",
			regContent: strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_CURRENT_USER\\Software\\Test]",
				"\"Data\"=\"value\"",
			}, "\r\n"),
			expectedPath:   "Software\\Test",
			expectedOpType: "OpCreateKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := regtext.NewCodec()
			ops, err := codec.ParseReg([]byte(tt.regContent), hive.RegParseOptions{
				AutoPrefix: true,
			})
			if err != nil {
				t.Fatalf("ParseReg failed: %v", err)
			}

			if len(ops) < 1 {
				t.Fatalf("Expected at least 1 operation, got %d", len(ops))
			}

			// Check first operation
			createKey, ok := ops[0].(types.OpCreateKey)
			if !ok {
				t.Fatalf("First operation should be OpCreateKey, got %T", ops[0])
			}
			if createKey.Path != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, createKey.Path)
			}
		})
	}
}

func TestParseRegWithAliasExpansion(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKLM\\SOFTWARE\\Test]",
		"\"Data\"=\"value\"",
		"",
		"[HKCU\\Software\\Test]",
		"\"Data\"=\"value\"",
		"",
	}, "\r\n")

	codec := regtext.NewCodec()
	ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		AutoPrefix: true,
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	// Both should be stripped after alias expansion
	if len(ops) < 2 {
		t.Fatalf("Expected at least 2 operations, got %d", len(ops))
	}

	// First key: HKLM\SOFTWARE\Test -> Test
	createKey1, ok := ops[0].(types.OpCreateKey)
	if !ok {
		t.Fatalf("Operation 0 should be OpCreateKey, got %T", ops[0])
	}
	if createKey1.Path != "Test" {
		t.Errorf("Expected path 'Test', got %q", createKey1.Path)
	}

	// Third key: HKCU\Software\Test -> Software\Test
	createKey2, ok := ops[2].(types.OpCreateKey)
	if !ok {
		t.Fatalf("Operation 2 should be OpCreateKey, got %T", ops[2])
	}
	if createKey2.Path != "Software\\Test" {
		t.Errorf("Expected path 'Software\\Test', got %q", createKey2.Path)
	}
}

func TestParseRegPrefixMismatchError(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SYSTEM\\Test]",
		"\"Data\"=\"value\"",
	}, "\r\n")

	codec := regtext.NewCodec()
	_, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE", // Mismatch - file has SYSTEM
	})

	if err == nil {
		t.Fatal("Expected error for prefix mismatch, got nil")
	}

	// Check that error message mentions the mismatch
	errMsg := err.Error()
	if !strings.Contains(errMsg, "does not start with expected prefix") {
		t.Errorf("Expected error about prefix mismatch, got: %v", err)
	}
}

func TestParseRegCaseInsensitivePrefixStripping(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[hkey_local_machine\\software\\Test]",
		"\"Data\"=\"value\"",
	}, "\r\n")

	codec := regtext.NewCodec()
	ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	if len(ops) < 1 {
		t.Fatalf("Expected at least 1 operation, got %d", len(ops))
	}

	createKey, ok := ops[0].(types.OpCreateKey)
	if !ok {
		t.Fatalf("First operation should be OpCreateKey, got %T", ops[0])
	}
	if createKey.Path != "Test" {
		t.Errorf("Expected path 'Test', got %q", createKey.Path)
	}
}

func TestParseRegWithDeleteOperationsAndPrefixStripping(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\KeepThis]",
		"\"Value\"=\"data\"",
		"",
		"[-HKEY_LOCAL_MACHINE\\SOFTWARE\\DeleteThis]",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\AlsoKeep]",
		"\"DeleteValue\"=-",
	}, "\r\n")

	codec := regtext.NewCodec()
	ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	// Should have: CreateKey, SetValue, DeleteKey, CreateKey, DeleteValue
	if len(ops) != 5 {
		t.Fatalf("Expected 5 operations, got %d", len(ops))
	}

	// Check delete key operation
	deleteKey, ok := ops[2].(types.OpDeleteKey)
	if !ok {
		t.Fatalf("Operation 2 should be OpDeleteKey, got %T", ops[2])
	}
	if deleteKey.Path != "DeleteThis" {
		t.Errorf("Expected delete path 'DeleteThis', got %q", deleteKey.Path)
	}

	// Check delete value operation
	deleteValue, ok := ops[4].(types.OpDeleteValue)
	if !ok {
		t.Fatalf("Operation 4 should be OpDeleteValue, got %T", ops[4])
	}
	if deleteValue.Path != "AlsoKeep" {
		t.Errorf("Expected path 'AlsoKeep', got %q", deleteValue.Path)
	}
	if deleteValue.Name != "DeleteValue" {
		t.Errorf("Expected name 'DeleteValue', got %q", deleteValue.Name)
	}
}

func TestParseRegWithREG_QWORD(t *testing.T) {
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Test]",
		"\"QWordValue\"=hex(b):ef,cd,ab,90,78,56,34,12",
	}, "\r\n")

	codec := regtext.NewCodec()
	ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(ops))
	}

	setValue, ok := ops[1].(types.OpSetValue)
	if !ok {
		t.Fatalf("Operation 1 should be OpSetValue, got %T", ops[1])
	}

	if setValue.Type != types.REG_QWORD {
		t.Errorf("Expected type REG_QWORD (%d), got %d", types.REG_QWORD, setValue.Type)
	}

	if setValue.Name != "QWordValue" {
		t.Errorf("Expected name 'QWordValue', got %q", setValue.Name)
	}
}

func TestParseRegWithAllHexTypes(t *testing.T) {
	tests := []struct {
		name         string
		hexType      string
		expectedType types.RegType
	}{
		{"REG_NONE", "hex(0):", types.REG_NONE},
		{"REG_SZ", "hex(1):", types.REG_SZ},
		{"REG_EXPAND_SZ", "hex(2):", types.REG_EXPAND_SZ},
		{"REG_BINARY", "hex(3):", types.REG_BINARY},
		{"REG_DWORD", "hex(4):", types.REG_DWORD},
		{"REG_DWORD_BE", "hex(5):", types.REG_DWORD_BE},
		{"REG_LINK", "hex(6):", types.REG_LINK},
		{"REG_MULTI_SZ", "hex(7):", types.REG_MULTI_SZ},
		{"REG_RESOURCE_LIST", "hex(8):", types.REG_RESOURCE_LIST},
		{"REG_FULL_RESOURCE_DESCRIPTOR", "hex(9):", types.REG_FULL_RESOURCE_DESCRIPTOR},
		{"REG_RESOURCE_REQUIREMENTS_LIST", "hex(a):", types.REG_RESOURCE_REQUIREMENTS_LIST},
		{"REG_QWORD", "hex(b):", types.REG_QWORD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regContent := strings.Join([]string{
				"Windows Registry Editor Version 5.00",
				"",
				"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Test]",
				"\"Value\"=" + tt.hexType + "01,02,03,04",
			}, "\r\n")

			codec := regtext.NewCodec()
			ops, err := codec.ParseReg([]byte(regContent), hive.RegParseOptions{
				AutoPrefix: true,
			})
			if err != nil {
				t.Fatalf("ParseReg failed: %v", err)
			}

			if len(ops) < 2 {
				t.Fatalf("Expected at least 2 operations, got %d", len(ops))
			}

			setValue, ok := ops[1].(types.OpSetValue)
			if !ok {
				t.Fatalf("Operation 1 should be OpSetValue, got %T", ops[1])
			}

			if setValue.Type != tt.expectedType {
				t.Errorf("Expected type %s (%d), got %d", tt.name, tt.expectedType, setValue.Type)
			}
		})
	}
}
