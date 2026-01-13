package bigdata

import (
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// DBSignature is the 2-byte signature for DB (big-data) headers.
	DBSignature = "db"

	// DBHeaderSize is the size of the DB header (12 bytes).
	DBHeaderSize = 12

	// MaxBlockSize is the maximum size of a single data block (16344 bytes)
	// This is the hivex convention for chunked data.
	MaxBlockSize = 16344

	// Byte offsets within DB header.
	dbCountOffset     = 2 // Offset to count field (uint16)
	dbBlocklistOffset = 4 // Offset to blocklist field (uint32)
	dbReservedOffset  = 8 // Offset to reserved field (uint32)

	// Bit shift amounts for byte extraction from multi-byte values.
	bitsPerByte   = 8  // Number of bits in a byte
	bitsPerUint16 = 16 // Number of bits in uint16
	bitsPerUint24 = 24 // Bit position for 4th byte (24 bits)
)

// DBHeader represents the structure of a DB (big-data) header cell.
type DBHeader struct {
	Signature [2]byte // 'db'
	Count     uint16  // Number of data blocks
	Blocklist uint32  // HCELL_INDEX to blocklist cell
	Reserved  uint32  // Reserved/Unknown - write zero
}

// WriteDBHeader writes a DB header to the buffer.
func WriteDBHeader(buf []byte, count uint16, blocklistRef uint32) error {
	if len(buf) < DBHeaderSize {
		return ErrTruncated
	}

	// Write signature
	buf[0] = 'd'
	buf[1] = 'b'

	// Write count (uint16, little-endian)
	buf[dbCountOffset] = byte(count)
	buf[dbCountOffset+1] = byte(count >> bitsPerByte)

	// Write blocklist HCELL_INDEX (uint32, little-endian)
	buf[dbBlocklistOffset] = byte(blocklistRef)
	buf[dbBlocklistOffset+1] = byte(blocklistRef >> bitsPerByte)
	buf[dbBlocklistOffset+2] = byte(blocklistRef >> bitsPerUint16)
	buf[dbBlocklistOffset+3] = byte(blocklistRef >> bitsPerUint24)

	// Write reserved (uint32 = 0)
	buf[dbReservedOffset] = 0
	buf[9] = 0
	buf[10] = 0
	buf[11] = 0

	return nil
}

// ReadDBHeader reads a DB header from the buffer.
func ReadDBHeader(buf []byte) (*DBHeader, error) {
	if len(buf) < DBHeaderSize {
		return nil, ErrTruncated
	}

	// Verify signature
	if buf[0] != 'd' || buf[1] != 'b' {
		return nil, ErrInvalidSignature
	}

	// Read count with bounds checking
	count, err := format.CheckedReadU16(buf, dbCountOffset)
	if err != nil {
		return nil, ErrTruncated
	}

	// Sanity check: block count
	if uint32(count) > format.DBMaxBlockCount {
		return nil, format.ErrSanityLimit
	}

	// Read blocklist HCELL_INDEX with bounds checking
	blocklist, err := format.CheckedReadU32(buf, dbBlocklistOffset)
	if err != nil {
		return nil, ErrTruncated
	}

	// Read reserved with bounds checking (we don't validate it, just read it)
	reserved, err := format.CheckedReadU32(buf, dbReservedOffset)
	if err != nil {
		return nil, ErrTruncated
	}

	return &DBHeader{
		Signature: [2]byte{buf[0], buf[1]},
		Count:     count,
		Blocklist: blocklist,
		Reserved:  reserved,
	}, nil
}

// WriteBlocklist writes a blocklist (array of HCELL_INDEX) to the buffer.
func WriteBlocklist(buf []byte, blockRefs []uint32) error {
	need := len(blockRefs) * format.DWORDSize
	if len(buf) < need {
		return ErrTruncated
	}

	for i, ref := range blockRefs {
		offset := i * format.DWORDSize
		buf[offset] = byte(ref)
		buf[offset+1] = byte(ref >> bitsPerByte)
		buf[offset+2] = byte(ref >> bitsPerUint16)
		buf[offset+3] = byte(ref >> bitsPerUint24)
	}

	return nil
}

// ReadBlocklist reads a blocklist from the buffer.
func ReadBlocklist(buf []byte, count uint16) ([]uint32, error) {
	// Sanity check: block count (uint16 max is 65535, but still check against format limit)
	if uint32(count) > format.DBMaxBlockCount {
		return nil, format.ErrSanityLimit
	}

	// Simple bounds check is sufficient here since uint16 * 4 cannot overflow int
	// (max: 65535 * 4 = 262140, well within int32 range)
	need := int(count) * format.DWORDSize
	if len(buf) < need {
		return nil, ErrTruncated
	}

	refs := make([]uint32, count)
	for i := range count {
		offset := int(i) * format.DWORDSize
		val, err := format.CheckedReadU32(buf, offset)
		if err != nil {
			// Return partial results on read error
			return refs[:i], nil
		}
		refs[i] = val
	}

	return refs, nil
}
