package buf

import (
	"fmt"
	"math"
)

// AddOverflowSafe adds a and b, returning ok = false when the result would overflow int.
func AddOverflowSafe(a, b int) (int, bool) {
	switch {
	case b > 0 && a > math.MaxInt-b:
		return 0, false
	case b < 0 && a < math.MinInt-b:
		return 0, false
	default:
		return a + b, true
	}
}

// MulOverflowSafe multiplies a and b, returning ok = false when the result would overflow int.
// This is essential for count * elementSize calculations in list parsing.
func MulOverflowSafe(a, b int) (int, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	// For positive numbers, check if result would overflow
	if a > 0 && b > 0 {
		if a > math.MaxInt/b {
			return 0, false
		}
	}
	// For negative numbers
	if a < 0 && b < 0 {
		if a < math.MaxInt/b {
			return 0, false
		}
	}
	// Mixed signs - check against MinInt
	if a > 0 && b < 0 {
		if b < math.MinInt/a {
			return 0, false
		}
	}
	if a < 0 && b > 0 {
		if a < math.MinInt/b {
			return 0, false
		}
	}
	return a * b, true
}

// CheckListBounds validates that count elements of elementSize bytes fit in buffer
// starting at offset. Returns the end offset if valid, or an error describing
// the specific failure (overflow or out of bounds).
//
// This is the recommended way to validate list structures before iterating:
//
//	endOff, err := buf.CheckListBounds(len(data), offset, int(count), elementSize)
//	if err != nil {
//	    return fmt.Errorf("list: %w", err)
//	}
//	// Safe to iterate from offset to endOff
func CheckListBounds(bufLen, offset, count, elementSize int) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("negative offset: %d", offset)
	}
	if count < 0 {
		return 0, fmt.Errorf("negative count: %d", count)
	}
	if elementSize < 0 {
		return 0, fmt.Errorf("negative element size: %d", elementSize)
	}

	// Check count * elementSize for overflow
	totalSize, ok := MulOverflowSafe(count, elementSize)
	if !ok {
		return 0, fmt.Errorf("overflow: count=%d * elemSize=%d", count, elementSize)
	}

	// Check offset + totalSize for overflow
	endOffset, ok := AddOverflowSafe(offset, totalSize)
	if !ok {
		return 0, fmt.Errorf("overflow: offset=%d + size=%d", offset, totalSize)
	}

	// Check bounds
	if endOffset > bufLen {
		return 0, fmt.Errorf("bounds: end=%d > len=%d", endOffset, bufLen)
	}

	return endOffset, nil
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
