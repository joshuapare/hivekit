package hive

import (
	"fmt"
	"os"
	"time"

	"github.com/joshuapare/hivekit/internal/format"
)

// Hive is the opened hive, backed by mmap (unix/darwin) or a byte slice (others).
type Hive struct {
	f    *os.File
	data []byte
	size int64
	base *BaseBlock
}

// HBINStart returns the absolute file offset where the HBIN area begins.
// In on-disk Windows hives this is always 0x1000 (4096).
func (h *Hive) HBINStart() uint32 {
	return uint32(format.HeaderSize)
}

// RootOffset returns the ABSOLUTE file offset of the root NK cell.
// The REGF header stores this as an offset *relative* to the HBIN start (0x1000),
// so we must add the HBIN start to it.
func (h *Hive) RootOffset() uint32 {
	if h == nil || h.base == nil {
		return 0
	}
	rel := h.base.RootCellOffset() // e.g. 0x20
	return uint32(format.HeaderSize) + rel
}

// RootCellOffset returns the NK root pointer RELATIVE TO 0x1000.
func (h *Hive) RootCellOffset() uint32 {
	if h.base == nil {
		return 0
	}
	return h.base.RootCellOffset()
}

// ResolveCellPayload resolves a relative cell offset and returns the payload bytes.
// This skips the 4-byte cell size header and returns just the payload data.
func (h *Hive) ResolveCellPayload(relOff uint32) ([]byte, error) {
	return resolveRelCellPayload(h.Bytes(), relOff)
}

func (h *Hive) Bytes() []byte { return h.data }

func (h *Hive) Size() int64 { return h.size }

func (h *Hive) FD() int {
	if h == nil || h.f == nil {
		return -1
	}
	return int(h.f.Fd())
}

// HBINs returns an iterator over all HBINs, starting at 0x1000.
func (h *Hive) HBINs() (*HBINIterator, error) {
	start := h.HBINStart()
	if int(start) > len(h.data) {
		return nil, fmt.Errorf("hive: HBIN start (%d) beyond file size (%d)", start, len(h.data))
	}
	return &HBINIterator{
		h:    h,
		next: start,
	}, nil
}

// BumpDataSize adds delta to the base block's HBIN data size field.
// This must be called after growing the hive by appending HBINs.
func (h *Hive) BumpDataSize(delta uint32) {
	if h == nil || h.data == nil || len(h.data) < format.HeaderSize {
		return
	}
	// Read current data size at offset 0x28
	current := format.ReadU32(h.data, format.REGFDataSizeOffset)
	newSize := current + delta
	// Write back the updated size
	format.PutU32(h.data, format.REGFDataSizeOffset, newSize)
}

// TouchNowAndBumpSeq updates the base block's last-write timestamp to now
// and increments both sequence numbers (primary and secondary).
func (h *Hive) TouchNowAndBumpSeq() {
	if h == nil || h.data == nil || len(h.data) < format.HeaderSize {
		return
	}
	// Read current sequence numbers
	seq1 := format.ReadU32(h.data, format.REGFPrimarySeqOffset)
	seq2 := format.ReadU32(h.data, format.REGFSecondarySeqOffset)

	// Increment both sequences
	seq1++
	seq2++

	// Write back sequence numbers
	format.PutU32(h.data, format.REGFPrimarySeqOffset, seq1)
	format.PutU32(h.data, format.REGFSecondarySeqOffset, seq2)

	// Update timestamp to now (Windows FILETIME format)
	nowFiletime := format.TimeToFiletime(time.Now())
	format.PutU64(h.data, format.REGFTimeStampOffset, nowFiletime)
}
