package format

import (
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// DBRecord represents a "db" (Big Data) record used for storing large registry
// values that exceed a single cell's capacity. The data is split across multiple
// data blocks, with this record containing a pointer to a blocklist.
//
// Format (from hivex source):
//   Offset 0x00: Signature "db" (2 bytes)
//   Offset 0x02: Number of blocks (2 bytes, uint16)
//   Offset 0x04: Blocklist offset (4 bytes, uint32) - points to cell containing block offsets
//   Offset 0x08: Unknown1 (4 bytes, uint32)
//
// The blocklist offset points to another cell that contains an array of uint32 offsets,
// each pointing to a data block cell. Data blocks should be concatenated in order
// to reconstruct the full value (up to the length specified in the VK record).
type DBRecord struct {
	NumBlocks       uint16 // Number of data blocks
	BlocklistOffset uint32 // Offset to cell containing the list of block offsets (relative to hive bins start)
	Unknown1        uint32 // Unknown field
}

// DecodeDB decodes a Big Data (db) record from the given cell data.
// The input should be the cell payload (after the 4-byte cell size header).
func DecodeDB(b []byte) (DBRecord, error) {
	if len(b) < DBMinSize {
		return DBRecord{}, fmt.Errorf("db: %w (need %d bytes, have %d)", ErrTruncated, DBMinSize, len(b))
	}

	// Check signature
	if b[DBSignatureOffset] != DBSignature[0] || b[DBSignatureOffset+1] != DBSignature[1] {
		return DBRecord{}, fmt.Errorf("db: %w", ErrSignatureMismatch)
	}

	// Read fields
	numBlocks := buf.U16LE(b[DBNumBlocksOffset:])
	blocklistOffset := buf.U32LE(b[DBBlocklistOffset:])
	unknown1 := buf.U32LE(b[DBUnknown1Offset:])

	return DBRecord{
		NumBlocks:       numBlocks,
		BlocklistOffset: blocklistOffset,
		Unknown1:        unknown1,
	}, nil
}

// IsDBRecord checks if the given cell data starts with the "db" signature.
// This is a quick check to determine if a cell contains a Big Data record.
func IsDBRecord(b []byte) bool {
	return len(b) >= 2 && b[0] == DBSignature[0] && b[1] == DBSignature[1]
}
