package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

func ParseLI(payload []byte) (LI, error) {
	if !hasPrefix(payload, format.LISignature) {
		return LI{}, errors.New("li: bad signature")
	}
	cnt, err := checkIndexHeader(payload)
	if err != nil {
		return LI{}, err
	}
	need := int(format.IdxListOffset) + int(cnt)*format.LIEntrySize
	if len(payload) < need {
		return LI{}, fmt.Errorf("li: truncated list: have=%d need=%d", len(payload), need)
	}
	return LI{buf: payload}, nil
}

// --- Common typed view for "li" (index leaf) ---

// LI and RI have similar structures but represent different Windows Registry
// concepts (index leaf vs root index). Keeping them separate improves clarity
// even though the implementation is similar.
type LI struct {
	buf []byte // payload beginning with "li"
}

func (li LI) Count() int {
	return int(format.ReadU16(li.buf, format.IdxCountOffset))
}

// CellIndexAt returns the NK cell RELATIVE offset at position i.
func (li LI) CellIndexAt(i int) uint32 {
	base := format.IdxListOffset + i*format.LIEntrySize
	return u32(li.buf, base)
}

// RawList returns a slice of raw uint32 list (zero-copy).
func (li LI) RawList() []byte {
	return li.buf[format.IdxListOffset : format.IdxListOffset+li.Count()*format.LIEntrySize]
}
