package index

import "sync"

var numericPool = sync.Pool{}

// AcquireNumericIndex returns a NumericIndex from the pool or creates a new one.
// The returned index is empty and ready for use.
func AcquireNumericIndex(nkCap, vkCap int) *NumericIndex {
	if v := numericPool.Get(); v != nil {
		idx := v.(*NumericIndex)
		idx.Reset()
		return idx
	}
	return NewNumericIndex(nkCap, vkCap)
}

// ReleaseNumericIndex returns a NumericIndex to the pool for reuse.
func ReleaseNumericIndex(idx *NumericIndex) {
	if idx == nil {
		return
	}
	numericPool.Put(idx)
}
