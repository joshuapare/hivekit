package format

import (
	"encoding/binary"
	"testing"
)

func TestDecodeSK(t *testing.T) {
	const descriptorLen = 0x10
	buf := make([]byte, SKDataOffset+descriptorLen*2)
	copy(buf, SKSignature)
	binary.LittleEndian.PutUint32(buf[SKLengthOffset:], descriptorLen)    // descriptor length
	binary.LittleEndian.PutUint32(buf[SKDescOffsetField:], SKDataOffset) // descriptor offset
	for i := 0; i < descriptorLen; i++ {
		buf[SKDataOffset+i] = byte(i)
	}
	cellOff := 0x200
	start, n, err := DecodeSK(buf, cellOff)
	if err != nil {
		t.Fatalf("DecodeSK: %v", err)
	}
	expectedStart := cellOff + SKDataOffset
	if start != expectedStart || n != descriptorLen {
		t.Fatalf("unexpected result start=%d (expected %d) n=%d (expected %d)", start, expectedStart, n, descriptorLen)
	}
}

func TestDecodeSKErrors(t *testing.T) {
	buf := make([]byte, 2)
	copy(buf, SKSignature)
	if _, _, err := DecodeSK(buf, 0); err == nil {
		t.Fatalf("expected truncation error")
	}
}
