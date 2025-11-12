// Package buf contains helpers for endian-safe decoding routines.
package buf

import "encoding/binary"

const (
	// sizeUint16 is the byte size of a uint16 value.
	sizeUint16 = 2
	// sizeUint32 is the byte size of a uint32 value.
	sizeUint32 = 4
	// sizeUint64 is the byte size of a uint64 value.
	sizeUint64 = 8
)

// U16LE reads a little-endian uint16 from b. Returns 0 when b is too short.
func U16LE(b []byte) uint16 {
	if len(b) < sizeUint16 {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

// U32LE reads a little-endian uint32 from b. Returns 0 when b is too short.
func U32LE(b []byte) uint32 {
	if len(b) < sizeUint32 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

// U64LE reads a little-endian uint64 from b. Returns 0 when b is too short.
func U64LE(b []byte) uint64 {
	if len(b) < sizeUint64 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// U32BE reads a big-endian uint32 from b. Returns 0 when b is too short.
func U32BE(b []byte) uint32 {
	if len(b) < sizeUint32 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

// I32LE reads a little-endian int32 from b. Returns 0 when b is too short.
// This reinterprets the uint32 bit pattern as int32 (for signed cell sizes in registry format).
// The direct cast preserves the bit pattern, which is intentional for binary format reading.
func I32LE(b []byte) int32 {
	if len(b) < sizeUint32 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(b))
}
