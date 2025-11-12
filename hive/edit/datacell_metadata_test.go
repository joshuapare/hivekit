//go:build linux || darwin

package edit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_DataCell_MetadataMatches verifies data cell metadata matches payload
// This is Test #17 from DEBUG.md: "DataCell_MetadataMatches".
func Test_DataCell_MetadataMatches(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_MetadataTest"})
	require.NoError(t, err)

	testCases := []struct {
		name     string
		valName  string
		valType  uint32
		data     []byte
		validate func(t *testing.T, vkRef uint32)
	}{
		{
			name:    "REGSZ",
			valName: "StringValue",
			valType: format.REGSZ,
			data:    []byte("H\x00e\x00l\x00l\x00o\x00\x00\x00"), // UTF-16LE "Hello"
			validate: func(t *testing.T, vkRef uint32) {
				payload, resolveErr := h.ResolveCellPayload(vkRef)
				require.NoError(t, resolveErr)

				vk, parseErr := ParseVK(payload)
				require.NoError(t, parseErr)

				require.Equal(t, format.REGSZ, vk.DataType, "Type should be REGSZ")
				require.Equal(
					t,
					uint32(12),
					vk.DataSize&0x7FFFFFFF,
					"Size should match data length (12 bytes = 'Hello' UTF-16LE with null)",
				)
			},
		},
		{
			name:    "REGDWORD",
			valName: "DwordValue",
			valType: format.REGDWORD,
			data:    []byte{0x42, 0x00, 0x00, 0x00}, // DWORD 0x42
			validate: func(t *testing.T, vkRef uint32) {
				payload, resolveErr := h.ResolveCellPayload(vkRef)
				require.NoError(t, resolveErr)

				vk, parseErr := ParseVK(payload)
				require.NoError(t, parseErr)

				require.Equal(t, format.REGDWORD, vk.DataType, "Type should be REGDWORD")
				require.Equal(t, uint32(4), vk.DataSize&0x7FFFFFFF, "Size should be 4 bytes")
			},
		},
		{
			name:    "REGBinary_Small",
			valName: "SmallBinary",
			valType: format.REGBinary,
			data:    []byte{0x01, 0x02, 0x03, 0x04},
			validate: func(t *testing.T, vkRef uint32) {
				payload, resolveErr := h.ResolveCellPayload(vkRef)
				require.NoError(t, resolveErr)

				vk, parseErr := ParseVK(payload)
				require.NoError(t, parseErr)

				require.Equal(t, format.REGBinary, vk.DataType, "Type should be REGBinary")
				require.Equal(t, uint32(4), vk.DataSize&0x7FFFFFFF, "Size should match data length")
			},
		},
		{
			name:    "REGBinary_Large",
			valName: "LargeBinary",
			valType: format.REGBinary,
			data:    bytes.Repeat([]byte{0xAA}, 20*1024), // 20KB
			validate: func(t *testing.T, vkRef uint32) {
				payload, resolveErr := h.ResolveCellPayload(vkRef)
				require.NoError(t, resolveErr)

				vk, parseErr := ParseVK(payload)
				require.NoError(t, parseErr)

				require.Equal(t, format.REGBinary, vk.DataType, "Type should be REGBinary")
				expectedSize := uint32(20 * 1024)
				actualSize := vk.DataSize & 0x7FFFFFFF
				require.Equal(t, expectedSize, actualSize,
					"Size should match data length (expected %d, got %d)", expectedSize, actualSize)
			},
		},
		{
			name:    "REGMultiSZ",
			valName: "MultiString",
			valType: format.REGMultiSZ,
			data: []byte("F\x00i\x00r\x00s\x00t\x00\x00\x00" + // UTF-16LE "First"
				"S\x00e\x00c\x00o\x00n\x00d\x00\x00\x00" + // UTF-16LE "Second"
				"\x00\x00"), // Double null terminator
			validate: func(t *testing.T, vkRef uint32) {
				payload, resolveErr := h.ResolveCellPayload(vkRef)
				require.NoError(t, resolveErr)

				vk, parseErr := ParseVK(payload)
				require.NoError(t, parseErr)

				require.Equal(t, format.REGMultiSZ, vk.DataType, "Type should be REGMultiSZ")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Store the value
			upsertErr := valueEditor.UpsertValue(keyRef, tc.valName, tc.valType, tc.data)
			require.NoError(t, upsertErr)

			// Get VK reference from index
			vkRef, ok := idx.GetVK(keyRef, normalizeName(tc.valName))
			require.True(t, ok, "Value should be in index")

			// Validate metadata
			tc.validate(t, vkRef)

			t.Logf("%s: type and size metadata match payload", tc.name)
		})
	}
}

// ParseVK is a minimal VK parser for testing.
type VK struct {
	DataType uint32
	DataSize uint32
}

func ParseVK(payload []byte) (*VK, error) {
	if len(payload) < 24 {
		return nil, &InvalidCellDataError{}
	}

	// VK structure:
	// 0x00: signature "vk"
	// 0x02: name length
	// 0x04: data size (4 bytes)
	// 0x08: data offset or inline data (4 bytes)
	// 0x0C: data type (4 bytes)
	// ...

	dataSize := format.ReadU32(payload, 4)

	dataType := format.ReadU32(payload, 12)

	return &VK{
		DataType: dataType,
		DataSize: dataSize,
	}, nil
}

type InvalidCellDataError struct{}

func (e *InvalidCellDataError) Error() string {
	return "invalid cell data"
}
