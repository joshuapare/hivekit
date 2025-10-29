package buf

import (
	"math"
	"testing"
)

func TestAddOverflowSafe(t *testing.T) {
	if sum, ok := AddOverflowSafe(10, 5); !ok || sum != 15 {
		t.Fatalf("AddOverflowSafe(10,5)=%d,%v want 15,true", sum, ok)
	}
	if _, ok := AddOverflowSafe(math.MaxInt, 1); ok {
		t.Fatalf("expected overflow when adding to MaxInt")
	}
	if _, ok := AddOverflowSafe(math.MinInt, -1); ok {
		t.Fatalf("expected underflow when subtracting from MinInt")
	}
}

func TestSliceAndHas(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4}
	if got, ok := Slice(data, 1, 3); !ok || len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("Slice returned unexpected result: %v, %v", got, ok)
	}
	if _, ok := Slice(data, 4, 2); ok {
		t.Fatalf("Slice should fail when extending beyond len")
	}
	if Has(data, 2, 4) {
		t.Fatalf("Has should be false for out-of-bounds range")
	}
	if !Has(data, 2, 1) {
		t.Fatalf("Has should be true for valid range")
	}

	if _, ok := Slice(data, -1, 1); ok {
		t.Fatalf("Slice should reject negative offset")
	}
	if _, ok := Slice(data, 1, -1); ok {
		t.Fatalf("Slice should reject negative length")
	}
}
