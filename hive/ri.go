package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// RI represents an "ri" (root index) cell - a list of cell indices pointing to leaf lists.
// LI and RI have similar structures but represent different Windows Registry
// concepts (index leaf vs root index). Keeping them separate improves clarity
// even though the implementation is similar.
type RI struct {
	buf []byte // payload beginning with "ri"
}

func ParseRI(payload []byte) (RI, error) {
	if !hasPrefix(payload, format.RISignature) {
		return RI{}, errors.New("ri: bad signature")
	}
	cnt, err := checkIndexHeader(payload)
	if err != nil {
		return RI{}, err
	}
	need := int(format.IdxListOffset) + int(cnt)*format.LIEntrySize
	if len(payload) < need {
		return RI{}, fmt.Errorf("ri: truncated list: have=%d need=%d", len(payload), need)
	}
	return RI{buf: payload}, nil
}

func (ri RI) Count() int { return int(format.ReadU16(ri.buf, format.IdxCountOffset)) }

// LeafCellAt returns the RELATIVE cell index of the child leaf (li/lf/lh).
func (ri RI) LeafCellAt(i int) uint32 {
	base := format.IdxListOffset + i*format.LIEntrySize
	return u32(ri.buf, base)
}

func (ri RI) RawList() []byte {
	return ri.buf[format.IdxListOffset : format.IdxListOffset+ri.Count()*format.LIEntrySize]
}
