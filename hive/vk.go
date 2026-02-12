package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// VK is a zero-cost view over a "vk" (value key) cell payload.
type VK struct {
	buf []byte // payload starting at 'vk'
	off int
}

func ParseVK(payload []byte) (VK, error) {
	if len(payload) < format.VKFixedHeaderSize {
		return VK{}, errors.New("vk: truncated header")
	}
	if payload[0] != 'v' || payload[1] != 'k' {
		return VK{}, fmt.Errorf("vk: bad signature %c%c", payload[0], payload[1])
	}
	// name slice bounds check happens in Name()
	return VK{buf: payload, off: 0}, nil
}

func (v VK) Flags() uint16 { return format.ReadU16(v.buf, v.off+format.VKFlagsOffset) }
func (v VK) Type() uint32  { return format.ReadU32(v.buf, v.off+format.VKTypeOffset) }

func (v VK) NameLen() uint16 {
	return format.ReadU16(v.buf, v.off+format.VKNameLenOffset)
}

func (v VK) NameCompressed() bool {
	return v.Flags()&format.VKFlagNameCompressed != 0
}

// Name returns raw name bytes (ASCII if compressed, UTF-16LE otherwise).
func (v VK) Name() []byte {
	n := int(v.NameLen())
	start := v.off + format.VKNameOffset
	end := start + n
	if n == 0 || end > len(v.buf) {
		return nil
	}
	return v.buf[start:end]
}

func (v VK) RawDataLen() uint32 {
	return format.ReadU32(v.buf, v.off+format.VKDataLenOffset)
}

func (v VK) IsSmallData() bool {
	return (v.RawDataLen() & format.VKSmallDataMask) != 0
}

func (v VK) DataLen() int {
	if v.IsSmallData() {
		return int(v.RawDataLen() &^ format.VKSmallDataMask)
	}
	return int(v.RawDataLen())
}

func (v VK) DataOffsetRel() uint32 {
	return format.ReadU32(v.buf, v.off+format.VKDataOffOffset)
}

// Data returns the value bytes, handling inline (small), external, and big data (db) formats.
// hiveBuf must be the whole hive (needed for following the HCELL index).
func (v VK) Data(hiveBuf []byte) ([]byte, error) {
	n := v.DataLen()
	if n == 0 {
		return nil, nil
	}

	if v.IsSmallData() {
		// Inline in the 4-byte DataOff field.
		// Return a slice over a small temp buffer? Zero-copy isn't possible here
		// because the 4 bytes are part of the header, but we can return those 4
		// bytes directly as a subslice of v.buf (safe).
		raw := v.buf[v.off+format.VKDataOffOffset : v.off+format.VKDataOffOffset+4]
		return raw[:n:n], nil // cap to n for safety
	}

	// External cell: resolve to its payload.
	rel := v.DataOffsetRel()
	pl, err := resolveRelCellPayload(hiveBuf, rel)
	if err != nil {
		return nil, fmt.Errorf("vk data: %w", err)
	}

	// Check if this is a Big Data (db) record for large values (>16KB).
	if format.IsDBRecord(pl) {
		return v.readBigData(hiveBuf, pl, n)
	}

	if len(pl) < n {
		return nil, fmt.Errorf("vk data: truncated external cell: have=%d need=%d", len(pl), n)
	}
	return pl[:n:n], nil
}

// readBigData reads and assembles data from a Big Data (db) record.
// Large registry values (>16KB) are split across multiple cells.
func (v VK) readBigData(hiveBuf, dbData []byte, expectedLen int) ([]byte, error) {
	db, err := format.DecodeDB(dbData)
	if err != nil {
		return nil, fmt.Errorf("vk data: %w", err)
	}

	// Read the blocklist cell.
	blocklistPl, err := resolveRelCellPayload(hiveBuf, db.BlocklistOffset)
	if err != nil {
		return nil, fmt.Errorf("vk data: db blocklist: %w", err)
	}

	// Parse block offsets from the blocklist (each offset is 4 bytes).
	numBlocks := int(db.NumBlocks)
	blocklistNeeded := numBlocks * format.OffsetFieldSize
	if len(blocklistPl) < blocklistNeeded {
		return nil, fmt.Errorf("vk data: db blocklist truncated: need %d bytes for %d blocks, have %d",
			blocklistNeeded, numBlocks, len(blocklistPl))
	}

	// Pre-allocate result buffer.
	result := make([]byte, expectedLen)
	bytesRead := 0

	for i := range numBlocks {
		off := i * format.OffsetFieldSize
		blockOffset := format.ReadU32(blocklistPl, off)

		blockPl, err := resolveRelCellPayload(hiveBuf, blockOffset)
		if err != nil {
			return nil, fmt.Errorf("vk data: db block %d: %w", i, err)
		}

		// DB blocks have 4 bytes of padding at the end.
		blockData := blockPl
		if len(blockData) > format.DBBlockPadding {
			blockData = blockData[:len(blockData)-format.DBBlockPadding]
		}

		// Copy block data, truncating if we've read enough.
		bytesAvailable := expectedLen - bytesRead
		if len(blockData) > bytesAvailable {
			blockData = blockData[:bytesAvailable]
		}

		copy(result[bytesRead:], blockData)
		bytesRead += len(blockData)

		if bytesRead >= expectedLen {
			break
		}
	}

	if bytesRead != expectedLen {
		return nil, fmt.Errorf("vk data: db size mismatch: expected %d bytes, got %d", expectedLen, bytesRead)
	}

	return result, nil
}
