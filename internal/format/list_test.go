package format

import (
	"encoding/binary"
	"testing"
)

func TestDecodeSubkeyListLI(t *testing.T) {
	b := make([]byte, 4+2*4)
	copy(b, LISignature)
	binary.LittleEndian.PutUint16(b[2:], 2)
	binary.LittleEndian.PutUint32(b[4:], 0x100)
	binary.LittleEndian.PutUint32(b[8:], 0x200)
	out, err := DecodeSubkeyList(b, 0)
	if err != nil {
		t.Fatalf("DecodeSubkeyList: %v", err)
	}
	if len(out) != 2 || out[0] != 0x100 || out[1] != 0x200 {
		t.Fatalf("unexpected result: %v", out)
	}
}

func TestDecodeValueList(t *testing.T) {
	b := make([]byte, 3*4)
	binary.LittleEndian.PutUint32(b[0:], 0x10)
	binary.LittleEndian.PutUint32(b[4:], 0x20)
	binary.LittleEndian.PutUint32(b[8:], 0x30)
	vals, err := DecodeValueList(b, 3)
	if err != nil {
		t.Fatalf("DecodeValueList: %v", err)
	}
	if len(vals) != 3 || vals[2] != 0x30 {
		t.Fatalf("unexpected values: %v", vals)
	}
}
