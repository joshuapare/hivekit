// Package flush implements the flush phase of the v2 merge engine.
// It applies in-place updates to existing cells and finalizes the hive header.
package flush

import (
	"encoding/binary"
)

// DeltaChecksum updates a base block XOR checksum by XOR-ing out the old
// value and XOR-ing in the new value for a single 4-byte field.
// Call once per changed field (Sequence1, Sequence2, TimeStamp low/high, Length).
//
// fieldOffset must be 4-byte aligned and within the checksum region (0..507).
// If the field is outside the checksum region, the current checksum is returned unchanged.
func DeltaChecksum(currentChecksum uint32, fieldOffset int, oldValue, newValue uint32) uint32 {
	if fieldOffset%4 != 0 || fieldOffset >= 508 {
		return currentChecksum
	}
	return currentChecksum ^ oldValue ^ newValue
}

// ComputeFullChecksum computes the base block checksum from scratch by XOR-ing
// 127 DWORDs from the first 508 bytes of the header. Used for verification.
//
// Special cases per Windows hive specification:
//   - If XOR result is 0x00000000, returns 0x00000001.
//   - If XOR result is 0xFFFFFFFF, returns 0xFFFFFFFE.
func ComputeFullChecksum(header []byte) uint32 {
	var sum uint32
	for i := 0; i < 508; i += 4 {
		sum ^= binary.LittleEndian.Uint32(header[i : i+4])
	}
	if sum == 0 {
		return 1
	}
	if sum == 0xFFFFFFFF {
		return 0xFFFFFFFE
	}
	return sum
}

// readU32 reads a little-endian uint32 from b at offset off.
func readU32(b []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(b[off : off+4])
}

// writeU32 writes a little-endian uint32 to b at offset off.
func writeU32(b []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(b[off:off+4], v)
}
