package hive

import (
	"errors"
	"fmt"
	"io"

	"github.com/joshuapare/hivekit/internal/format"
)

// ValueList is a zero-copy view of a value list cell payload.
//
// In the Windows Registry format, an NK (node key) that has values stores
// them as a value list: a separate cell containing an array of uint32
// HCELL_INDEX entries, where each entry points to a VK (value key) cell.
//
// The value list has NO signature - it's just raw uint32 array data.
// Layout:
//
//	[0..3]    uint32  HCELL_INDEX to first VK
//	[4..7]    uint32  HCELL_INDEX to second VK
//	...
//	[N*4..]   uint32  HCELL_INDEX to Nth VK
//
// Why this exists: NK cells would be too large if they stored VK offsets
// inline, so the offsets are stored in a separate cell that the NK
// references via ValueListOffsetRel().
type ValueList struct {
	buf []byte // raw list payload (4 * N bytes)
	off int
}

// ParseValueList parses a value list from cell payload.
// The payload should be the raw cell payload (after the 4-byte cell header).
// expectedCount is the value count from the NK cell (NK.ValueCount()).
//
// Why expectedCount is validated: The NK cell declares how many values exist.
// The value list cell must have at least that many entries. If it doesn't,
// the hive is corrupt or the NK count is wrong.
func ParseValueList(payload []byte, expectedCount int) (ValueList, error) {
	if expectedCount < 0 {
		return ValueList{}, errors.New("hive: negative value count")
	}

	needed := expectedCount * format.DWORDSize
	if len(payload) < needed {
		return ValueList{}, fmt.Errorf(
			"hive: value list too small: need %d bytes for %d values, have %d",
			needed, expectedCount, len(payload))
	}

	return ValueList{buf: payload, off: 0}, nil
}

// Count returns the number of VK offsets in the list.
// This is the byte length divided by 4 (sizeof uint32).
func (vl ValueList) Count() int {
	return len(vl.buf) / format.DWORDSize
}

// VKOffsetAt returns the HCELL_INDEX to the VK cell at position i.
// Returns io.EOF if i is out of bounds (including negative indices).
//
// Why this returns uint32: The offset is relative to the first HBIN
// (at file offset 0x1000). To resolve it to an absolute file offset,
// use: abs = hive.HBINStart() + VKOffsetAt(i).
func (vl ValueList) VKOffsetAt(i int) (uint32, error) {
	if i < 0 {
		return 0, io.EOF
	}
	off := i * format.DWORDSize
	if off+format.DWORDSize > len(vl.buf) {
		return 0, io.EOF
	}
	return format.ReadU32(vl.buf, off), nil
}

// ValidateCount ensures the list has at least n entries (n * 4 bytes).
// This is typically called with the NK's ValueCount() to verify consistency.
//
// Why this matters for security: If the NK claims N values but the value list
// cell is smaller, parsing could read out of bounds. This check prevents that.
func (vl ValueList) ValidateCount(n int) error {
	if n < 0 {
		return errors.New("hive: negative count")
	}
	if n*format.DWORDSize > len(vl.buf) {
		return fmt.Errorf("hive: value list too small: need %d bytes for %d values, have %d",
			n*format.DWORDSize, n, len(vl.buf))
	}
	return nil
}

// Raw returns the raw byte slice backing this value list.
// This is a zero-copy view - modifications to the returned slice will
// affect the underlying hive data.
//
// Why this exists: For performance-critical code that wants to parse
// the uint32 array directly without the bounds checks of VKOffsetAt().
func (vl ValueList) Raw() []byte {
	return vl.buf
}
