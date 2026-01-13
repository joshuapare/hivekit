package reader

import (
	"math"

	"github.com/joshuapare/hivekit/pkg/types"
)

// ScanSubkeys returns an iterator over direct child keys of id.
func (r *reader) ScanSubkeys(id types.NodeID) (types.NodeIter, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return nil, err
	}
	if nk.SubkeyCount == 0 || nk.SubkeyListOffset == math.MaxUint32 {
		return &emptyNodeIter{}, nil
	}
	list, err := r.subkeyList(nk.SubkeyListOffset, nk.SubkeyCount)
	if err != nil {
		return nil, err
	}
	return &sliceNodeIter{data: list}, nil
}

// ScanValues returns an iterator over value handles associated with id.
func (r *reader) ScanValues(id types.NodeID) (types.ValueIter, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return nil, err
	}
	if nk.ValueCount == 0 || nk.ValueListOffset == math.MaxUint32 {
		return &emptyValueIter{}, nil
	}
	list, err := r.valueList(nk.ValueListOffset, nk.ValueCount)
	if err != nil {
		return nil, err
	}
	return &sliceValueIter{data: list}, nil
}

type sliceNodeIter struct {
	data []uint32
	idx  int
}

func (it *sliceNodeIter) Next() bool {
	if it.idx >= len(it.data) {
		return false
	}
	it.idx++
	return true
}

func (it *sliceNodeIter) Err() error { return nil }

func (it *sliceNodeIter) Node() types.NodeID { return types.NodeID(it.data[it.idx-1]) }

type sliceValueIter struct {
	data []types.ValueID
	idx  int
}

func (it *sliceValueIter) Next() bool {
	if it.idx >= len(it.data) {
		return false
	}
	it.idx++
	return true
}

func (it *sliceValueIter) Err() error { return nil }

func (it *sliceValueIter) Value() types.ValueID { return it.data[it.idx-1] }

type emptyNodeIter struct{}

func (emptyNodeIter) Next() bool         { return false }
func (emptyNodeIter) Err() error         { return nil }
func (emptyNodeIter) Node() types.NodeID { return 0 }

type emptyValueIter struct{}

func (emptyValueIter) Next() bool           { return false }
func (emptyValueIter) Err() error           { return nil }
func (emptyValueIter) Value() types.ValueID { return 0 }
