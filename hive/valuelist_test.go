package hive

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// ============================================================================
// Test Helpers
// ============================================================================

// makeValueList creates a value list payload (array of uint32 VK offsets).
func makeValueList(t *testing.T, vkOffsets []uint32) []byte {
	t.Helper()
	buf := make([]byte, len(vkOffsets)*4)
	for i, offset := range vkOffsets {
		format.PutU32(buf, i*4, offset)
	}
	return buf
}

// ============================================================================
// Basic Parsing Tests
// ============================================================================

func TestValueList_ParseEmpty(t *testing.T) {
	// Why this test: An NK with no values is valid. The value list may not
	// exist at all, or it may exist as an empty cell (0 bytes).
	//
	// Spec note: If NK.ValueCount() == 0, the ValueListOffsetRel() field
	// may point to 0xFFFFFFFF (special "no value" marker) or may point to
	// an empty cell.
	payload := makeValueList(t, []uint32{})

	vl, err := ParseValueList(payload, 0)
	require.NoError(t, err)
	require.Equal(t, 0, vl.Count())
}

func TestValueList_ParseSingleValue(t *testing.T) {
	// Why this test: Tests minimal non-empty case. An NK with exactly one
	// value is common (e.g., registry keys with just a default value).
	payload := makeValueList(t, []uint32{0x1234})

	vl, err := ParseValueList(payload, 1)
	require.NoError(t, err)
	require.Equal(t, 1, vl.Count())

	offset, err := vl.VKOffsetAt(0)
	require.NoError(t, err)
	require.Equal(t, uint32(0x1234), offset)
}

func TestValueList_ParseMultipleValues(t *testing.T) {
	// Why this test: Tests typical case where an NK has multiple values.
	// Example: A registry key storing configuration settings.
	offsets := []uint32{0x1000, 0x2000, 0x3000, 0x4000, 0x5000}
	payload := makeValueList(t, offsets)

	vl, err := ParseValueList(payload, len(offsets))
	require.NoError(t, err)
	require.Equal(t, len(offsets), vl.Count())

	// Verify each offset
	for i, expected := range offsets {
		actual, vkErr := vl.VKOffsetAt(i)
		require.NoError(t, vkErr)
		require.Equal(t, expected, actual, "offset mismatch at index %d", i)
	}
}

func TestValueList_ParseTooSmall(t *testing.T) {
	// Why this test: Validates we catch corruption where NK claims N values
	// but the value list cell is smaller than N * 4 bytes.
	//
	// Security note: This prevents out-of-bounds reads during value iteration.
	payload := makeValueList(t, []uint32{0x1000, 0x2000}) // 8 bytes

	// NK claims 3 values, but only 2 offsets exist
	_, err := ParseValueList(payload, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bounds")
}

func TestValueList_ParseNegativeCount(t *testing.T) {
	// Why this test: Defensive programming against invalid negative counts.
	// Should never happen in practice, but we validate the input.
	payload := makeValueList(t, []uint32{0x1000})

	_, err := ParseValueList(payload, -1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "negative")
}

// ============================================================================
// Count and Validation Tests
// ============================================================================

func TestValueList_Count(t *testing.T) {
	// Why this test: Validates Count() returns the correct number of entries
	// based on payload size (length / 4 since each offset is 4 bytes).
	tests := []struct {
		name     string
		offsets  []uint32
		expected int
	}{
		{"empty", []uint32{}, 0},
		{"single", []uint32{0x1000}, 1},
		{"multiple", []uint32{0x1000, 0x2000, 0x3000}, 3},
		{"many", make([]uint32, 100), 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := makeValueList(t, tc.offsets)
			vl, err := ParseValueList(payload, len(tc.offsets))
			require.NoError(t, err)
			require.Equal(t, tc.expected, vl.Count())
		})
	}
}

func TestValueList_ValidateCount_OK(t *testing.T) {
	// Why this test: ValidateCount ensures the value list has enough bytes
	// to contain the expected number of VK offsets.
	//
	// Why this matters: The NK cell claims N values, but we need to verify
	// the value list cell actually contains N * 4 bytes before reading it.
	payload := makeValueList(t, []uint32{0x1000, 0x2000, 0x3000})
	vl := ValueList{buf: payload, off: 0}

	err := vl.ValidateCount(3)
	require.NoError(t, err)

	// Also OK if we validate for fewer than available
	err = vl.ValidateCount(2)
	require.NoError(t, err)
}

func TestValueList_ValidateCount_TooLarge(t *testing.T) {
	// Why this test: Validates we catch cases where the NK claims more values
	// than the value list cell actually contains.
	//
	// Security note: This prevents out-of-bounds reads when accessing VK offsets.
	payload := makeValueList(t, []uint32{0x1000, 0x2000})
	vl := ValueList{buf: payload, off: 0}

	err := vl.ValidateCount(3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bounds")
}

func TestValueList_ValidateCount_Negative(t *testing.T) {
	// Why this test: Validates defensive programming against negative counts.
	payload := makeValueList(t, []uint32{0x1000})
	vl := ValueList{buf: payload, off: 0}

	err := vl.ValidateCount(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "negative")
}

func TestValueList_ValidateCount_Zero(t *testing.T) {
	// Why this test: An NK with 0 values is valid. ValidateCount(0) should
	// succeed even on empty payloads.
	payload := makeValueList(t, []uint32{})
	vl := ValueList{buf: payload, off: 0}

	err := vl.ValidateCount(0)
	require.NoError(t, err)
}

// ============================================================================
// VKOffsetAt Tests
// ============================================================================

func TestValueList_VKOffsetAt_ValidIndices(t *testing.T) {
	// Why this test: Validates VKOffsetAt() correctly retrieves individual
	// VK offsets with proper bounds checking.
	//
	// Why we need this: The value list is just raw bytes (array of uint32).
	// VKOffsetAt() provides safe, indexed access with error handling.
	offsets := []uint32{0x1000, 0x2000, 0x3000, 0x4000}
	payload := makeValueList(t, offsets)
	vl := ValueList{buf: payload, off: 0}

	for i, expected := range offsets {
		actual, err := vl.VKOffsetAt(i)
		require.NoError(t, err)
		require.Equal(t, expected, actual, "offset mismatch at index %d", i)
	}
}

func TestValueList_VKOffsetAt_OutOfBounds(t *testing.T) {
	// Why this test: Validates defensive bounds checking prevents crashes or
	// undefined behavior when accessing invalid indices.
	//
	// Why io.EOF: VKOffsetAt() returns io.EOF (not error) for out-of-bounds
	// to match Go's iterator conventions.
	payload := makeValueList(t, []uint32{0x1000, 0x2000})
	vl := ValueList{buf: payload, off: 0}

	// Valid indices: 0, 1
	// Invalid indices: 2, 3, -1, etc.
	_, err := vl.VKOffsetAt(2)
	require.ErrorIs(t, err, io.EOF)

	_, err = vl.VKOffsetAt(100)
	require.ErrorIs(t, err, io.EOF)
}

func TestValueList_VKOffsetAt_NegativeIndex(t *testing.T) {
	// Why this test: Negative indices are invalid. Should return EOF.
	payload := makeValueList(t, []uint32{0x1000})
	vl := ValueList{buf: payload, off: 0}

	_, err := vl.VKOffsetAt(-1)
	require.ErrorIs(t, err, io.EOF)
}

// ============================================================================
// Raw Access Tests
// ============================================================================

func TestValueList_Raw_ZeroCopy(t *testing.T) {
	// Why this test: Validates that Raw() returns a zero-copy slice of the
	// underlying buffer, not a copy.
	//
	// Why this matters: For performance in hot loops when reading many values.
	// The reader can get the raw bytes once and parse offsets directly.
	payload := makeValueList(t, []uint32{0x1000, 0x2000, 0x3000})
	vl := ValueList{buf: payload, off: 0}

	raw := vl.Raw()

	// Verify it's the same data
	require.Len(t, raw, len(payload))
	require.Equal(t, payload, raw)

	// Verify it's zero-copy (same underlying array)
	require.Equal(t, &payload[0], &raw[0], "Raw() should return zero-copy slice")
}

func TestValueList_Raw_MultipleCallsSameSlice(t *testing.T) {
	// Why this test: Ensures multiple Raw() calls return equivalent slices.
	// Validates consistency of zero-copy access.
	payload := makeValueList(t, []uint32{0x1000, 0x2000})
	vl := ValueList{buf: payload, off: 0}

	raw1 := vl.Raw()
	raw2 := vl.Raw()

	require.Equal(t, raw1, raw2)
	require.Equal(t, &raw1[0], &raw2[0], "multiple Raw() calls should return same slice")
}

func TestValueList_Raw_Format(t *testing.T) {
	// Why this test: Documents the exact binary format of the value list.
	//
	// Format specification:
	//   Offset 0x00: uint32 HCELL_INDEX to first VK
	//   Offset 0x04: uint32 HCELL_INDEX to second VK
	//   ...
	//   Offset N*4: uint32 HCELL_INDEX to Nth VK
	offsets := []uint32{0x12345678, 0xAABBCCDD, 0x11223344}
	payload := makeValueList(t, offsets)
	vl := ValueList{buf: payload, off: 0}

	raw := vl.Raw()
	require.Len(t, raw, 12) // 3 offsets × 4 bytes

	// Verify little-endian encoding
	require.Equal(t, uint32(0x12345678), format.ReadU32(raw, 0))
	require.Equal(t, uint32(0xAABBCCDD), format.ReadU32(raw, 4))
	require.Equal(t, uint32(0x11223344), format.ReadU32(raw, 8))
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestValueList_LargeCount(t *testing.T) {
	// Why this test: Tests handling of NK cells with many values.
	// Some registry keys (e.g., COM class registrations) can have hundreds
	// of values.
	//
	// Note: Unlike DB which has a max count of 65535, value lists are
	// limited by cell size (max ~16KB), which allows ~4000 values maximum.
	const count = 1000
	offsets := make([]uint32, count)
	for i := range offsets {
		offsets[i] = uint32((i + 1) * 0x1000)
	}

	payload := makeValueList(t, offsets)
	vl, err := ParseValueList(payload, count)
	require.NoError(t, err)
	require.Equal(t, count, vl.Count())

	// Spot check a few offsets
	offset, err := vl.VKOffsetAt(0)
	require.NoError(t, err)
	require.Equal(t, uint32(0x1000), offset)

	offset, err = vl.VKOffsetAt(500)
	require.NoError(t, err)
	require.Equal(t, uint32(501*0x1000), offset)

	offset, err = vl.VKOffsetAt(999)
	require.NoError(t, err)
	require.Equal(t, uint32(1000*0x1000), offset)
}

func TestValueList_ParseExtraBytes(t *testing.T) {
	// Why this test: The value list cell may be larger than needed
	// (cells are 8-byte aligned). ParseValueList should succeed as long
	// as there are at least expectedCount * 4 bytes.
	//
	// Example: NK claims 3 values (need 12 bytes), but the cell is 16 bytes
	// due to alignment padding.
	offsets := []uint32{0x1000, 0x2000, 0x3000}
	payload := makeValueList(t, offsets)
	payload = append(payload, 0, 0, 0, 0) // Add 4 bytes padding

	vl, err := ParseValueList(payload, 3)
	require.NoError(t, err)
	require.Equal(t, 4, vl.Count()) // Count based on actual size
	require.Len(t, vl.Raw(), 16)

	// Verify original 3 offsets are intact
	offset, err := vl.VKOffsetAt(0)
	require.NoError(t, err)
	require.Equal(t, uint32(0x1000), offset)

	offset, err = vl.VKOffsetAt(1)
	require.NoError(t, err)
	require.Equal(t, uint32(0x2000), offset)

	offset, err = vl.VKOffsetAt(2)
	require.NoError(t, err)
	require.Equal(t, uint32(0x3000), offset)
}

// ============================================================================
// Documentation Tests
// ============================================================================

func TestValueList_ExampleUsage(t *testing.T) {
	// Why this test: Documents typical usage pattern for parsing value lists.
	//
	// Typical flow:
	// 1. Parse NK cell
	// 2. Get value count and value list offset
	// 3. Resolve value list cell
	// 4. Parse value list
	// 5. Iterate VK offsets
	// 6. Resolve and parse each VK

	// Simulated: NK says it has 3 values at offset 0x5000
	valueCount := 3
	valueListOffsets := []uint32{0x1000, 0x2000, 0x3000}
	valueListPayload := makeValueList(t, valueListOffsets)

	// Parse value list
	vl, err := ParseValueList(valueListPayload, valueCount)
	require.NoError(t, err)

	// Iterate VK offsets
	for i := range vl.Count() {
		vkOffset, vkErr := vl.VKOffsetAt(i)
		require.NoError(t, vkErr)

		// In real code, you would:
		// abs := int(hive.HBINStart() + vkOffset)
		// vkCell := resolveCell(hive.Data(), abs)
		// vk := ParseVK(vkCell.Payload())

		require.NotZero(t, vkOffset)
	}
}

func TestValueList_NKIntegration_Concept(t *testing.T) {
	// Why this test: Documents how ValueList integrates with NK parsing.
	//
	// Relationship:
	// - NK.ValueCount() tells how many values exist
	// - NK.ValueListOffsetRel() tells where the value list cell is
	// - ValueList stores an array of HCELL_INDEX to VK cells
	// - Each VK cell stores one name/value pair

	// Simulated NK fields
	valueCount := 4
	valueListOffset := uint32(0x5000) // HCELL_INDEX

	// Simulated value list cell content
	vkOffsets := []uint32{0x1000, 0x2000, 0x3000, 0x4000}
	valueListPayload := makeValueList(t, vkOffsets)

	// Parse the value list
	vl, err := ParseValueList(valueListPayload, valueCount)
	require.NoError(t, err)
	require.Equal(t, valueCount, vl.Count())

	// Validate the value list matches NK's claim
	require.NoError(t, vl.ValidateCount(valueCount))

	// In real usage:
	// for i := 0; i < vl.Count(); i++ {
	//     vkOffset, _ := vl.VKOffsetAt(i)
	//     vk := resolveAndParseVK(hive, vkOffset)
	//     name := vk.Name()
	//     data := vk.Data(hive.Bytes())
	//     // process name and data
	// }

	_ = valueListOffset // Would be used to resolve the cell
}
