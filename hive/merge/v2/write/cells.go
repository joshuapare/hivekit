package write

import (
	"time"

	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// WriteNK writes an NK (Node Key) cell payload into buf.
// buf must be at least format.NKFixedHeaderSize + len(name) bytes.
//
// The cell is written with:
//   - "nk" signature
//   - KEY_COMP_NAME flag (ASCII name)
//   - Current time as last write timestamp
//   - Specified parent, SK, subkey list, value list references
//   - Zero subkey/value counts (caller updates these separately)
//
// Limitation: name is assumed to be ASCII-representable. The KEY_COMP_NAME
// flag is always set and the name bytes are copied as-is. Non-ASCII key
// names (which require UTF-16LE encoding without the compressed flag)
// are not supported yet. In practice, Windows registry key names are
// overwhelmingly ASCII. Full UTF-16LE support can be added when needed.
func WriteNK(buf []byte, name string, parentRef, skRef uint32) {
	// Signature.
	buf[0] = 'n'
	buf[1] = 'k'

	// Flags: compressed (ASCII) name.
	format.PutU16(buf, format.NKFlagsOffset, format.NKFlagCompressedName)

	// Timestamp: current time as Windows FILETIME.
	ft := format.TimeToFiletime(time.Now())
	format.PutU64(buf, format.NKLastWriteOffset, ft)

	// Access bits (spare on older hives).
	format.PutU32(buf, format.NKAccessBitsOffset, 0)

	// Parent offset.
	format.PutU32(buf, format.NKParentOffset, parentRef)

	// Subkey count: 0 initially.
	format.PutU32(buf, format.NKSubkeyCountOffset, 0)

	// Volatile subkey count: 0.
	format.PutU32(buf, format.NKVolSubkeyCountOffset, 0)

	// Subkey list: none.
	format.PutU32(buf, format.NKSubkeyListOffset, format.InvalidOffset)

	// Volatile subkey list: none.
	format.PutU32(buf, format.NKVolSubkeyListOffset, format.InvalidOffset)

	// Value count: 0 initially.
	format.PutU32(buf, format.NKValueCountOffset, 0)

	// Value list: none.
	format.PutU32(buf, format.NKValueListOffset, format.InvalidOffset)

	// Security descriptor offset.
	format.PutU32(buf, format.NKSecurityOffset, skRef)

	// Class name offset: none.
	format.PutU32(buf, format.NKClassNameOffset, format.InvalidOffset)

	// Max name/class/value stats: zero.
	format.PutU32(buf, format.NKMaxNameLenOffset, 0)
	format.PutU32(buf, format.NKMaxClassLenOffset, 0)
	format.PutU32(buf, format.NKMaxValueNameOffset, 0)
	format.PutU32(buf, format.NKMaxValueDataOffset, 0)

	// Work var: unused.
	format.PutU32(buf, format.NKWorkVarOffset, 0)

	// Name length.
	nameBytes := []byte(name)
	format.PutU16(buf, format.NKNameLenOffset, uint16(len(nameBytes)))

	// Class length: 0.
	format.PutU16(buf, format.NKClassLenOffset, 0)

	// Copy name bytes.
	copy(buf[format.NKNameOffset:], nameBytes)
}

// WriteNKWithCounts is like WriteNK but also sets the subkey and value
// counts and their list offsets. Used when the counts are known at write time.
func WriteNKWithCounts(
	buf []byte,
	name string,
	parentRef, skRef uint32,
	subkeyCount uint32, subkeyListRef uint32,
	valueCount uint32, valueListRef uint32,
) {
	WriteNK(buf, name, parentRef, skRef)
	format.PutU32(buf, format.NKSubkeyCountOffset, subkeyCount)
	format.PutU32(buf, format.NKSubkeyListOffset, subkeyListRef)
	format.PutU32(buf, format.NKValueCountOffset, valueCount)
	format.PutU32(buf, format.NKValueListOffset, valueListRef)
}

// WriteVK writes a VK (Value Key) cell payload into buf.
// buf must be at least format.VKFixedHeaderSize + len(name) bytes.
//
// For inline data (len(data) <= 4), the data is stored directly in the
// DataOffset field and the high bit of DataLength is set.
// For external data, dataRef must point to a separately allocated data cell.
func WriteVK(buf []byte, name string, valType uint32, data []byte, dataRef uint32) {
	// Signature.
	buf[0] = 'v'
	buf[1] = 'k'

	nameBytes := []byte(name)
	nameLen := uint16(len(nameBytes))

	// Name length.
	format.PutU16(buf, format.VKNameLenOffset, nameLen)

	// Data length and offset.
	dataLen := len(data)
	if dataLen <= format.DWORDSize {
		// Inline: data stored in DataOffset field, high bit set in DataLength.
		format.PutU32(buf, format.VKDataLenOffset, uint32(dataLen)|format.VKSmallDataMask)
		// Copy inline data into the data offset field.
		var inlineBuf [4]byte
		copy(inlineBuf[:], data)
		format.PutU32(buf, format.VKDataOffOffset, format.ReadU32(inlineBuf[:], 0))
	} else {
		// External: data in a separate cell.
		format.PutU32(buf, format.VKDataLenOffset, uint32(dataLen))
		format.PutU32(buf, format.VKDataOffOffset, dataRef)
	}

	// Value type.
	format.PutU32(buf, format.VKTypeOffset, valType)

	// Flags: compressed name (ASCII).
	format.PutU16(buf, format.VKFlagsOffset, format.VKFlagNameCompressed)

	// Spare: 0.
	format.PutU16(buf, format.VKSpareOffset, 0)

	// Copy name bytes.
	copy(buf[format.VKNameOffset:], nameBytes)
}

// WriteLHList writes an LH (hash leaf) subkey list cell payload into buf.
// buf must be at least format.ListHeaderSize + len(entries)*8 bytes.
func WriteLHList(buf []byte, entries []subkeys.RawEntry) {
	count := uint16(len(entries))

	// Signature: "lh".
	buf[0] = 'l'
	buf[1] = 'h'

	// Count.
	format.PutU16(buf, 2, count)

	// Entries: 4-byte NKRef + 4-byte Hash each.
	for i, entry := range entries {
		off := format.ListHeaderSize + i*format.QWORDSize
		format.PutU32(buf, off, entry.NKRef)
		format.PutU32(buf, off+4, entry.Hash)
	}
}

// WriteLHListFromEntries writes an LH list from full Entry objects.
// Computes Hash if not already set.
func WriteLHListFromEntries(buf []byte, entries []subkeys.Entry) {
	count := uint16(len(entries))

	buf[0] = 'l'
	buf[1] = 'h'

	format.PutU16(buf, 2, count)

	for i, entry := range entries {
		off := format.ListHeaderSize + i*format.QWORDSize
		format.PutU32(buf, off, entry.NKRef)
		hash := entry.Hash
		if hash == 0 {
			hash = subkeys.Hash(entry.NameLower)
		}
		format.PutU32(buf, off+4, hash)
	}
}

// WriteValueList writes a value list cell payload into buf.
// buf must be at least len(vkRefs)*4 bytes.
// A value list is a flat array of uint32 VK cell offsets.
func WriteValueList(buf []byte, vkRefs []uint32) {
	for i, ref := range vkRefs {
		format.PutU32(buf, i*format.DWORDSize, ref)
	}
}

// WriteDataCell writes raw data into a data cell payload buffer.
// buf must be at least len(data) bytes.
func WriteDataCell(buf []byte, data []byte) {
	copy(buf, data)
}

// NKPayloadSize returns the payload size needed for an NK cell with the given name.
func NKPayloadSize(name string) int {
	return format.NKFixedHeaderSize + len(name)
}

// VKPayloadSize returns the payload size needed for a VK cell with the given name.
func VKPayloadSize(name string) int {
	return format.VKFixedHeaderSize + len(name)
}

// LHListPayloadSize returns the payload size for an LH list with count entries.
func LHListPayloadSize(count int) int {
	return format.ListHeaderSize + count*format.QWORDSize
}

// ValueListPayloadSize returns the payload size for a value list with count entries.
func ValueListPayloadSize(count int) int {
	return count * format.DWORDSize
}
