package buf

import "testing"

func TestEndianHelpers(t *testing.T) {
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}

	if got := U16LE(data); got != 0x2301 {
		t.Fatalf("U16LE = 0x%x, want 0x2301", got)
	}
	if got := U32LE(data); got != 0x67452301 {
		t.Fatalf("U32LE = 0x%x, want 0x67452301", got)
	}
	if got := U64LE(data); got != 0xefcdab8967452301 {
		t.Fatalf("U64LE = 0x%x, want 0xefcdab8967452301", got)
	}
	if got := U32BE(data); got != 0x01234567 {
		t.Fatalf("U32BE = 0x%x, want 0x01234567", got)
	}
	if got := I32LE(data); got != 0x67452301 {
		t.Fatalf("I32LE = 0x%x, want 0x67452301", got)
	}

	short := []byte{0xAA}
	if U16LE(short) != 0 {
		t.Fatalf("U16LE short should be 0")
	}
	if U32LE(short) != 0 || U32BE(short) != 0 || U64LE(short) != 0 || I32LE(short) != 0 {
		t.Fatalf("short reads should return 0")
	}
}
