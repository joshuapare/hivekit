package hive

import (
	"errors"
	"fmt"
	"io"

	"github.com/joshuapare/hivekit/internal/format"
)

// DB is a zero-cost view over a "db" (big-data) HEADER payload only.
// Layout (offsets relative to start of payload):
//
//	0x00: "db"
//	0x02: Count (uint16)
//	0x04: BlocklistOffset (uint32)  // HCELL_INDEX to separate cell
//	0x08: Unknown (uint32)
type DB struct {
	buf []byte // header payload (starts at 'db')
	off int
}

// ParseDB validates the "db" header. It does NOT touch the external block list.
func ParseDB(payload []byte) (DB, error) {
	if len(payload) < format.DBHeaderSize {
		return DB{}, fmt.Errorf("hive: DB header too small: %d", len(payload))
	}
	sig := payload[format.DBSignatureOffset : format.DBSignatureOffset+format.DBSignatureLen]
	if string(sig) != string(format.DBSignature) {
		return DB{}, fmt.Errorf("hive: DB bad signature: %q", sig)
	}
	cnt := format.ReadU16(payload, format.DBCountOffset)
	if cnt < format.DBMinBlockCount {
		return DB{}, fmt.Errorf("hive: DB block count %d invalid (min %d)",
			cnt, format.DBMinBlockCount)
	}
	return DB{buf: payload, off: 0}, nil
}

// Count returns the number of data blocks referenced by this big-data value.
func (d DB) Count() int {
	return int(format.ReadU16(d.buf, d.off+format.DBCountOffset))
}

// BlocklistOffset returns the HCELL_INDEX (relative to first HBIN) of the list cell.
func (d DB) BlocklistOffset() uint32 {
	return format.ReadU32(d.buf, d.off+format.DBListOffset)
}

// Unknown returns the trailing uint32 field at 0x08.
func (d DB) Unknown() uint32 {
	return format.ReadU32(d.buf, d.off+format.DBUnknown1Offset)
}

// DBList is a zero-copy view of the separate list cell payload.
// The payload is an array of uint32 HCELL_INDEX entries (no signature).
type DBList struct {
	buf []byte // raw list payload (4 * N bytes)
	off int
}

// ResolveList resolves the external block-list cell and returns a DBList view.
func (d DB) ResolveList(h *Hive) (DBList, error) {
	rel := d.BlocklistOffset()
	abs := int(h.HBINStart() + rel)
	cell, err := newCellAt(h.data, abs)
	if err != nil {
		return DBList{}, err
	}
	return DBList{buf: cell.Payload(), off: 0}, nil
}

// ValidateCount ensures the list has at least n entries (n * 4 bytes).
func (l DBList) ValidateCount(n int) error {
	if n < 0 {
		return errors.New("hive: negative count")
	}
	if n*format.DWORDSize > len(l.buf) {
		return fmt.Errorf("hive: DB list too small: need %d bytes, have %d",
			n*format.DWORDSize, len(l.buf))
	}
	return nil
}

func (l DBList) Len() int { return len(l.buf) / format.DWORDSize }

func (l DBList) At(i int) (uint32, error) {
	off := i * format.DWORDSize
	if off+format.DWORDSize > len(l.buf) {
		return 0, io.EOF
	}
	return format.ReadU32(l.buf, off), nil
}

func (l DBList) Raw() []byte { return l.buf } // zero-copy, for hot loops
