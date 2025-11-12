package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- "lf" (fast leaf): CM_INDEX entries {Cell, NameHint[4]} ---

func ParseLF(payload []byte) (LF, error) {
	if !hasPrefix(payload, format.LFSignature) {
		return LF{}, errors.New("lf: bad signature")
	}
	cnt, err := checkIndexHeader(payload)
	if err != nil {
		return LF{}, err
	}
	need := int(format.IdxListOffset) + int(cnt)*format.LFFHEntrySize
	if len(payload) < need {
		return LF{}, fmt.Errorf("lf: truncated list: have=%d need=%d", len(payload), need)
	}
	return LF{buf: payload}, nil
}

type LF struct {
	buf []byte // payload beginning with "lf"
}

func (lf LF) entryBytes(i int) []byte {
	off := format.IdxListOffset + i*format.LFFHEntrySize
	return lf.buf[off : off+format.LFFHEntrySize]
}

func (lf LF) Entry(i int) LFEntry { return LFEntry{raw: lf.entryBytes(i)} }

func (lf LF) Count() int { return int(format.ReadU16(lf.buf, format.IdxCountOffset)) }

// LFEntry provides a zero-copy view into.
type LFEntry struct {
	// raw 8 bytes window: [0..3]=Cell, [4..7]=NameHint (verbatim 4 bytes)
	raw []byte
}

func (e LFEntry) Cell() uint32 { return format.ReadU32(e.raw, 0) }

// HintBytes returns the 4-byte “fast hint” (first 4 ASCII chars, case-sensitive; 0 for non-ASCII).
func (e LFEntry) HintBytes() []byte { return e.raw[4:8] }

func (lf LF) RawList() []byte {
	return lf.buf[format.IdxListOffset : format.IdxListOffset+lf.Count()*format.LFFHEntrySize]
}
