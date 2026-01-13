package hive

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// ============================================================================
// Helper Functions
// ============================================================================

// makeDBHeader builds a synthetic DB header (12 bytes) for testing.
func makeDBHeader(t *testing.T, count uint16, blocklistOffset uint32, mutate func([]byte)) []byte {
	t.Helper()

	buf := make([]byte, format.DBHeaderSize)

	// Signature "db" @ 0x00
	copy(buf[format.DBSignatureOffset:], format.DBSignature)

	// Count @ 0x02 (uint16!)
	format.PutU16(buf, format.DBCountOffset, count)

	// BlocklistOffset @ 0x04
	format.PutU32(buf, format.DBListOffset, blocklistOffset)

	// Unknown1 @ 0x08
	format.PutU32(buf, format.DBUnknown1Offset, 0x00000000)

	if mutate != nil {
		mutate(buf)
	}

	return buf
}

// makeBlocklist creates a blocklist cell payload with specified block offsets.
func makeBlocklist(t *testing.T, blockOffsets []uint32) []byte {
	t.Helper()

	buf := make([]byte, len(blockOffsets)*4)
	for i, offset := range blockOffsets {
		format.PutU32(buf, i*4, offset)
	}
	return buf
}

// ============================================================================
// DB Header Parsing Tests
// ============================================================================

func TestDB_ParseOK(t *testing.T) {
	// Why this test: Validates basic DB header parsing with valid structure.
	//
	// The DB header is only 12 bytes and contains metadata about the big data
	// structure. The actual block offsets are stored in a separate cell.
	const count = 2
	const blocklistOff = 0x2000
	payload := makeDBHeader(t, count, blocklistOff, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)

	// Verify signature
	require.Equal(t, "db", string(payload[0:2]))

	// Verify Count (critical: must be uint16!)
	require.Equal(t, count, db.Count())

	// Verify BlocklistOffset
	require.Equal(t, uint32(blocklistOff), db.BlocklistOffset())

	// Verify Unknown field is accessible
	require.Equal(t, uint32(0), db.Unknown())
}

func TestDB_BadSignature(t *testing.T) {
	// Why this test: Ensures signature validation catches corruption or
	// incorrect cell type references.
	//
	// Spec note: Windows verifies the 'db' signature on hive load, but we
	// validate it again to catch programming errors where a non-DB cell
	// was incorrectly treated as a DB cell.
	payload := makeDBHeader(t, 2, 0x2000, func(b []byte) {
		b[0] = 'x'
		b[1] = 'x'
	})

	_, err := ParseDB(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad signature")
}

func TestDB_TooSmall(t *testing.T) {
	// Why this test: Ensures we catch truncated cells that don't have
	// enough bytes for the minimum DB header (12 bytes).
	//
	// The DB header is a fixed 12 bytes. Any smaller is corruption.
	payload := make([]byte, 10)
	copy(payload, format.DBSignature)

	_, err := ParseDB(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too small")
}

func TestDB_ExactlyMinSize(t *testing.T) {
	// Why this test: Tests the boundary condition where the header is
	// exactly the minimum size (12 bytes).
	payload := makeDBHeader(t, 2, 0x2000, nil)
	require.Len(t, payload, format.DBHeaderSize)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, 2, db.Count())
}

func TestDB_OneByteShortOfMinSize(t *testing.T) {
	// Why this test: Tests that we properly reject headers that are just
	// one byte short of the minimum size.
	payload := make([]byte, format.DBHeaderSize-1)
	copy(payload, format.DBSignature)

	_, err := ParseDB(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too small")
}

// ============================================================================
// Block Count Validation Tests
// ============================================================================

func TestDB_CountZero_Invalid(t *testing.T) {
	// Why this test: The spec explicitly states count must be >= 2. A count of 0
	// means the value is empty, so the DB format should not be used at all.
	//
	// Spec reference: "if it was set to 0, that would mean that the value is empty
	// so the big data object shouldn't be present at all"
	payload := makeDBHeader(t, 0, 0x2000, nil)

	_, err := ParseDB(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

func TestDB_CountOne_Invalid(t *testing.T) {
	// Why this test: The spec explicitly states count must be >= 2. A count of 1
	// means the data fits in a single block, so a direct cell reference should
	// have been used instead of the DB format.
	//
	// Spec reference: "If it was equal to 1, a direct backing buffer should have
	// been used instead"
	//
	// Historical note: Integer overflow bugs in older Windows versions could
	// create count=0 or count=1, but we reject them for structural correctness.
	payload := makeDBHeader(t, 1, 0x2000, nil)

	_, err := ParseDB(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

func TestDB_CountTwo_MinValid(t *testing.T) {
	// Why this test: Tests the boundary condition where count is exactly at the
	// minimum valid value. Count=2 is the smallest valid DB record.
	//
	// Example: A 20KB value would be split into 2 blocks of 16,344 bytes each.
	payload := makeDBHeader(t, 2, 0x2000, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, 2, db.Count())
}

func TestDB_CountTypical(t *testing.T) {
	// Why this test: Tests a normal case where a large value is split into
	// multiple chunks.
	//
	// Example: A 200KB value would be split into ~13 blocks:
	//   - 12 full blocks of 16,344 bytes = 196,128 bytes
	//   - 1 partial block of 3,872 bytes
	//   - Total: 200,000 bytes
	const count = 15
	payload := makeDBHeader(t, count, 0x2000, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, count, db.Count())
}

func TestDB_CountMax_65535(t *testing.T) {
	// Why this test: Tests the maximum possible block count. The Count field
	// is uint16 (2 bytes), so the maximum value is 65535.
	//
	// Spec note: This allows values up to ~1GB in size:
	//   65535 blocks × 16,344 bytes/block ≈ 1,071,104,040 bytes (~1.07GB)
	const maxCount = 65535
	payload := makeDBHeader(t, maxCount, 0x2000, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, maxCount, db.Count())
}

// ============================================================================
// DB Header Field Accessor Tests
// ============================================================================

func TestDB_BlocklistOffset(t *testing.T) {
	// Why this test: Validates that BlocklistOffset() correctly returns the
	// HCELL_INDEX to the blocklist cell.
	//
	// Why the blocklist is a separate cell: The blocklist is an array of uint32
	// offsets (4 bytes each) pointing to data blocks. It's stored in its own cell
	// to allow flexible sizing - a DB record with many blocks needs a large
	// blocklist, but the DB header itself remains a fixed 12 bytes.
	const blocklistOff = 0x3500
	payload := makeDBHeader(t, 2, blocklistOff, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(blocklistOff), db.BlocklistOffset())
}

func TestDB_Unknown(t *testing.T) {
	// Why this test: Validates the Unknown() accessor works.
	//
	// Spec note: This field at offset 0x08 is never accessed by Windows and
	// its purpose is unknown. We provide an accessor for completeness and
	// potential forensic analysis.
	const unknownValue = 0x12345678
	payload := makeDBHeader(t, 2, 0x2000, func(b []byte) {
		format.PutU32(b, format.DBUnknown1Offset, unknownValue)
	})

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(unknownValue), db.Unknown())
}

// ============================================================================
// DBList Tests (Blocklist Cell)
// ============================================================================

func TestDBList_ValidateCount_OK(t *testing.T) {
	// Why this test: ValidateCount ensures the blocklist cell has enough bytes
	// to contain the expected number of block offsets.
	//
	// Why this matters: The DB header claims N blocks, but we need to verify
	// the blocklist cell actually contains N * 4 bytes before reading it.
	blockOffsets := []uint32{0x1000, 0x2000, 0x3000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	// Should accept count <= actual length
	require.NoError(t, list.ValidateCount(3))
	require.NoError(t, list.ValidateCount(2))
	require.NoError(t, list.ValidateCount(1))
	require.NoError(t, list.ValidateCount(0))
}

func TestDBList_ValidateCount_TooLarge(t *testing.T) {
	// Why this test: Validates we catch cases where the DB header claims more
	// blocks than the blocklist cell actually contains.
	//
	// Security note: This prevents out-of-bounds reads when accessing block offsets.
	blockOffsets := []uint32{0x1000, 0x2000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	// Should reject count > actual length
	err := list.ValidateCount(3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too small")
}

func TestDBList_ValidateCount_Negative(t *testing.T) {
	// Why this test: Validates defensive programming against negative counts.
	listPayload := makeBlocklist(t, []uint32{0x1000})

	list := DBList{buf: listPayload, off: 0}

	err := list.ValidateCount(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "negative")
}

func TestDBList_Len(t *testing.T) {
	// Why this test: Validates Len() returns the correct number of entries
	// based on the payload size (length / 4 since each offset is 4 bytes).
	blockOffsets := []uint32{0x1000, 0x2000, 0x3000, 0x4000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	require.Equal(t, 4, list.Len())
	require.Equal(t, len(blockOffsets), list.Len())
}

func TestDBList_At_ValidIndices(t *testing.T) {
	// Why this test: Validates At() correctly retrieves individual block
	// offsets with proper bounds checking.
	//
	// Why we need this: The blocklist is just raw bytes (array of uint32).
	// At() provides safe, indexed access with error handling.
	blockOffsets := []uint32{0x1000, 0x2000, 0x3000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	// Test accessing each entry
	for i := range blockOffsets {
		offset, err := list.At(i)
		require.NoError(t, err)
		require.Equal(t, blockOffsets[i], offset)
	}
}

func TestDBList_At_OutOfBounds(t *testing.T) {
	// Why this test: Validates defensive bounds checking prevents crashes or
	// undefined behavior when accessing invalid indices.
	//
	// Why io.EOF: At() returns io.EOF (not error) for out-of-bounds to match
	// Go's iterator conventions.
	blockOffsets := []uint32{0x1000, 0x2000, 0x3000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	// Test index == length
	_, err := list.At(3)
	require.Equal(t, io.EOF, err)

	// Test index > length
	_, err = list.At(100)
	require.Equal(t, io.EOF, err)
}

func TestDBList_Raw_ZeroCopy(t *testing.T) {
	// Why this test: Validates that Raw() returns a zero-copy slice of the
	// underlying buffer, not a copy.
	//
	// Why this matters: For performance in hot loops when reading many blocks.
	// The reader can get the raw bytes once and parse offsets directly.
	blockOffsets := []uint32{0x1000, 0x2000, 0x3000}
	listPayload := makeBlocklist(t, blockOffsets)

	list := DBList{buf: listPayload, off: 0}

	raw := list.Raw()

	// Verify it's the actual data
	require.Equal(t, listPayload, raw)
	require.Len(t, raw, len(blockOffsets)*4)

	// Verify we can parse it back
	for i := range blockOffsets {
		offset := format.ReadU32(raw, i*4)
		require.Equal(t, blockOffsets[i], offset)
	}
}

func TestDBList_Raw_MultipleCallsSameSlice(t *testing.T) {
	// Why this test: Ensures multiple Raw() calls return equivalent slices.
	// Validates consistency of zero-copy access.
	listPayload := makeBlocklist(t, []uint32{0x1000, 0x2000})

	list := DBList{buf: listPayload, off: 0}

	raw1 := list.Raw()
	raw2 := list.Raw()

	require.Equal(t, raw1, raw2)
	require.Len(t, raw2, len(raw1))
}

// ============================================================================
// Chunk Size Documentation Tests
// ============================================================================

func TestDB_StandardChunkSize_Documentation(t *testing.T) {
	// Why this test: Documents the standard chunk size of 16,344 bytes.
	//
	// Spec reference: "Conceptually, it is simply a means of dividing one long
	// data blob into smaller portions of 16344 bytes, each stored in a separate cell."
	//
	// Why 16,344 bytes: This is 16KB (16,384 bytes) minus the 4-byte cell header
	// overhead. Each data block is followed by the next cell's header, which must
	// be trimmed when assembling the value.
	//
	// Example: A value of exactly 32,688 bytes would be split into:
	//   - Block 0: 16,344 bytes
	//   - Block 1: 16,344 bytes
	//   - Total: 32,688 bytes (count = 2)
	const count = 2
	const expectedTotalSize = format.DBChunkSize * count // 32,688 bytes

	payload := makeDBHeader(t, count, 0x2000, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, count, db.Count())

	// Document the expected total size
	t.Logf("DB with %d blocks can store up to %d bytes", count, expectedTotalSize)
	t.Logf("Each block stores %d bytes of data", format.DBChunkSize)
	t.Logf("Plus %d bytes padding per block (cell header)", format.DBBlockPadding)
}

func TestDB_LastChunkSmaller_Documentation(t *testing.T) {
	// Why this test: Documents that the last chunk may be smaller if the total
	// value length isn't evenly divisible by 16,344.
	//
	// Spec reference: "If the length of the overall value is not divisible by
	// 16344, the final chunk contains the remaining 1–16343 bytes."
	//
	// Example: A real BootPlan value from Windows hive:
	//   - Total size: 25,544 bytes
	//   - Block 0: 16,344 bytes (full)
	//   - Block 1: 9,200 bytes (partial, remainder)
	//   - Count: 2
	const totalSize = 25544
	const fullBlocks = totalSize / format.DBChunkSize    // = 1
	const lastBlockSize = totalSize % format.DBChunkSize // = 9200
	const count = fullBlocks + 1                         // = 2

	payload := makeDBHeader(t, uint16(count), 0x2000, nil)

	db, err := ParseDB(payload)
	require.NoError(t, err)
	require.Equal(t, count, db.Count())

	// Document the layout
	t.Logf("Value size: %d bytes", totalSize)
	t.Logf("Full blocks: %d × %d bytes = %d bytes", fullBlocks, format.DBChunkSize, fullBlocks*format.DBChunkSize)
	t.Logf("Last block: %d bytes (remainder)", lastBlockSize)
	t.Logf("Total blocks: %d", count)
}

func TestDB_BlockPadding_Documentation(t *testing.T) {
	// Why this test: Documents the 4-byte padding at the end of each data block.
	//
	// Why padding exists: Each data block is followed by the next cell's header
	// (4 bytes), which is the standard cell header size that appears between all
	// registry cells. This padding must be trimmed when assembling the value data.
	//
	// Example: Reading 2 blocks of 16,344 bytes each:
	//   - Read block 0: 16,344 bytes + 4 bytes padding = 16,348 bytes raw
	//   - Trim 4 bytes: 16,344 bytes actual data
	//   - Read block 1: 16,344 bytes + 4 bytes padding = 16,348 bytes raw
	//   - Trim 4 bytes: 16,344 bytes actual data
	//   - Total: 32,688 bytes assembled
	const count = 2
	const chunkSize = format.DBChunkSize
	const padding = format.DBBlockPadding

	payload := makeDBHeader(t, count, 0x2000, nil)

	_, err := ParseDB(payload)
	require.NoError(t, err)

	// Document the padding
	t.Logf("Each data block: %d bytes", chunkSize)
	t.Logf("Padding per block: %d bytes (cell header)", padding)
	t.Logf("Raw bytes read per block: %d bytes", chunkSize+padding)
	t.Logf("After trimming, %d blocks yield %d bytes", count, count*chunkSize)
}

// ============================================================================
// Integration Test (with HBIN context)
// ============================================================================

func TestDB_InHBINContext(t *testing.T) {
	// Why this test: Tests DB header parsing in the context of a full HBIN
	// structure, similar to how it would be encountered in a real hive file.
	//
	// This validates that DB cells work correctly when embedded in proper HBIN
	// structure with cell headers and alignment.
	const count = 2
	const blocklistOff = 0x2000
	dbPayload := makeDBHeader(t, count, blocklistOff, nil)

	// Build an HBIN with the DB cell
	cells := []CellSpec{
		{
			Allocated: true,
			Size:      len(dbPayload) + format.CellHeaderSize,
			Payload:   dbPayload,
		},
	}

	hbin := buildHBINFromSpec(t, cells)

	// Extract the cell payload (skip cell header)
	cellStart := format.HBINHeaderSize + format.CellHeaderSize
	extractedPayload := hbin[cellStart : cellStart+len(dbPayload)]

	// Parse the extracted DB header
	db, err := ParseDB(extractedPayload)
	require.NoError(t, err)
	require.Equal(t, count, db.Count())
	require.Equal(t, uint32(blocklistOff), db.BlocklistOffset())
}
