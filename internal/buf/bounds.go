package buf

import "math"

// AddOverflowSafe adds a and b, returning ok = false when the result would overflow int.
func AddOverflowSafe(a, b int) (sum int, ok bool) {
	switch {
	case b > 0 && a > math.MaxInt-b:
		return 0, false
	case b < 0 && a < math.MinInt-b:
		return 0, false
	default:
		return a + b, true
	}
}

// Slice returns the sub-slice [off:off+n] if it fits within len(b).
func Slice(b []byte, off, n int) ([]byte, bool) {
	if off < 0 || n < 0 || off > len(b) {
		return nil, false
	}
	end, ok := AddOverflowSafe(off, n)
	if !ok || end > len(b) {
		return nil, false
	}
	return b[off:end], true
}

// Has reports whether b[off:off+n] is within bounds.
func Has(b []byte, off, n int) bool {
	_, ok := Slice(b, off, n)
	return ok
}
