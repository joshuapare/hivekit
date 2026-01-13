package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- "lh" (hash leaf): CM_INDEX entries {Cell, HashKey} ---

type LH struct {
	buf []byte // payload beginning with "lh"
}

func ParseLH(payload []byte) (LH, error) {
	if !hasPrefix(payload, format.LHSignature) {
		return LH{}, errors.New("lh: bad signature")
	}
	cnt, err := checkIndexHeader(payload)
	if err != nil {
		return LH{}, err
	}
	need := int(format.IdxListOffset) + int(cnt)*format.LFFHEntrySize
	if len(payload) < need {
		return LH{}, fmt.Errorf("lh: truncated list: have=%d need=%d", len(payload), need)
	}
	return LH{buf: payload}, nil
}

func (lh LH) Count() int { return int(format.ReadU16(lh.buf, format.IdxCountOffset)) }

type LHEntry struct{ raw []byte }

func (lh LH) entryBytes(i int) []byte {
	off := format.IdxListOffset + i*format.LFFHEntrySize
	return lh.buf[off : off+format.LFFHEntrySize]
}

func (lh LH) Entry(i int) LHEntry { return LHEntry{raw: lh.entryBytes(i)} }
func (e LHEntry) Cell() uint32    { return format.ReadU32(e.raw, 0) }
func (e LHEntry) HashKey() uint32 { return format.ReadU32(e.raw, format.DWORDSize) }
func (lh LH) RawList() []byte {
	return lh.buf[format.IdxListOffset : format.IdxListOffset+lh.Count()*format.LFFHEntrySize]
}
