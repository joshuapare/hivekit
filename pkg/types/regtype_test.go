package types

import (
	"testing"
)

func TestRegType_String(t *testing.T) {
	tests := []struct {
		name     string
		regType  RegType
		expected string
	}{
		// Known types
		{
			name:     "REG_NONE",
			regType:  REG_NONE,
			expected: "REG_NONE",
		},
		{
			name:     "REG_SZ",
			regType:  REG_SZ,
			expected: "REG_SZ",
		},
		{
			name:     "REG_EXPAND_SZ",
			regType:  REG_EXPAND_SZ,
			expected: "REG_EXPAND_SZ",
		},
		{
			name:     "REG_BINARY",
			regType:  REG_BINARY,
			expected: "REG_BINARY",
		},
		{
			name:     "REG_DWORD",
			regType:  REG_DWORD,
			expected: "REG_DWORD",
		},
		{
			name:     "REG_DWORD_BE",
			regType:  REG_DWORD_BE,
			expected: "REG_DWORD_BE",
		},
		{
			name:     "REG_LINK",
			regType:  REG_LINK,
			expected: "REG_LINK",
		},
		{
			name:     "REG_MULTI_SZ",
			regType:  REG_MULTI_SZ,
			expected: "REG_MULTI_SZ",
		},
		{
			name:     "REG_QWORD",
			regType:  REG_QWORD,
			expected: "REG_QWORD",
		},
		// Unknown types (these should match hivex format: UNKNOWN_TYPE_<signed int32>)
		{
			name:     "Unknown type 100",
			regType:  RegType(100),
			expected: "UNKNOWN_TYPE_100",
		},
		{
			name:     "Unknown type 255",
			regType:  RegType(255),
			expected: "UNKNOWN_TYPE_255",
		},
		{
			name:     "Invalid type -1 (0xFFFFFFFF)",
			regType:  RegType(0xFFFFFFFF),
			expected: "UNKNOWN_TYPE_-1",
		},
		{
			name:     "Invalid type -65511 (0xFFFF0019) - from real data",
			regType:  RegType(4294901785), // 0xFFFF0019
			expected: "UNKNOWN_TYPE_-65511",
		},
		{
			name:     "Invalid type -65519 (0xFFFF0011)",
			regType:  RegType(4294901777), // 0xFFFF0011
			expected: "UNKNOWN_TYPE_-65519",
		},
		{
			name:     "Invalid type -65518 (0xFFFF0012)",
			regType:  RegType(4294901778), // 0xFFFF0012
			expected: "UNKNOWN_TYPE_-65518",
		},
		{
			name:     "Invalid type -65529 (0xFFFF0007)",
			regType:  RegType(4294901767), // 0xFFFF0007
			expected: "UNKNOWN_TYPE_-65529",
		},
		{
			name:     "Invalid type -65517 (0xFFFF0013)",
			regType:  RegType(4294901779), // 0xFFFF0013
			expected: "UNKNOWN_TYPE_-65517",
		},
		{
			name:     "Invalid type -57326 (0xFFFF1F12)",
			regType:  RegType(4294909970), // 0xFFFF1F12
			expected: "UNKNOWN_TYPE_-57326",
		},
		{
			name:     "Very large unknown type",
			regType:  RegType(2147483648), // 2^31
			expected: "UNKNOWN_TYPE_-2147483648",
		},
		{
			name:     "Type 12 (unknown but valid range)",
			regType:  RegType(12),
			expected: "UNKNOWN_TYPE_12",
		},
		{
			name:     "Type 8 (REG_RESOURCE_LIST)",
			regType:  RegType(8),
			expected: "UNKNOWN_TYPE_8", // Not in our constants, but valid Windows type
		},
		{
			name:     "Type 9 (REG_FULL_RESOURCE_DESCRIPTOR)",
			regType:  RegType(9),
			expected: "UNKNOWN_TYPE_9", // Not in our constants, but valid Windows type
		},
		{
			name:     "Type 10 (REG_RESOURCE_REQUIREMENTS_LIST)",
			regType:  RegType(10),
			expected: "UNKNOWN_TYPE_10", // Not in our constants, but valid Windows type
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.regType.String()
			if result != tt.expected {
				t.Errorf("RegType(%d).String() = %q, expected %q (0x%08x as int32: %d)",
					uint32(tt.regType), result, tt.expected,
					uint32(tt.regType), int32(tt.regType))
			}
		})
	}
}
