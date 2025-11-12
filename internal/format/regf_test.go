package format

import (
	"encoding/binary"
	"testing"
)

func TestParseHeaderSuccess(t *testing.T) {
	buf := make([]byte, HeaderSize)
	copy(buf, REGFSignature)
	binary.LittleEndian.PutUint32(buf[REGFPrimarySeqOffset:], 1)
	binary.LittleEndian.PutUint32(buf[REGFSecondarySeqOffset:], 2)
	binary.LittleEndian.PutUint64(buf[REGFTimeStampOffset:], 123456789)
	binary.LittleEndian.PutUint32(buf[REGFMajorVersionOffset:], 5)
	binary.LittleEndian.PutUint32(buf[REGFMinorVersionOffset:], 6)
	binary.LittleEndian.PutUint32(buf[REGFTypeOffset:], 7)
	binary.LittleEndian.PutUint32(buf[REGFRootCellOffset:], 0x200)
	binary.LittleEndian.PutUint32(buf[REGFDataSizeOffset:], 0x3000)
	binary.LittleEndian.PutUint32(buf[REGFClusterOffset:], 1)

	hdr, err := ParseHeader(buf)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if hdr.PrimarySequence != 1 || hdr.SecondarySequence != 2 {
		t.Fatalf("sequence mismatch: %+v", hdr)
	}
	if hdr.RootCellOffset != 0x200 {
		t.Fatalf("root offset mismatch: %+v", hdr)
	}
	if hdr.HiveBinsDataSize != 0x3000 {
		t.Fatalf("data size mismatch: %+v", hdr)
	}
}

func TestParseHeaderErrors(t *testing.T) {
	buf := make([]byte, HeaderSize)
	if _, err := ParseHeader(buf[:10]); err == nil {
		t.Fatalf("expected truncation error")
	}
	copy(buf, []byte{'B', 'A', 'D', '!'})
	if _, err := ParseHeader(buf); err == nil {
		t.Fatalf("expected signature error")
	}
}
