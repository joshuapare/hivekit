// Package buf contains helpers for endian-safe decoding routines.
package buf

import "encoding/binary"

// U16LE reads a little-endian uint16 from b. Returns 0 when b is too short.
func U16LE(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

// U32LE reads a little-endian uint32 from b. Returns 0 when b is too short.
func U32LE(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

// U64LE reads a little-endian uint64 from b. Returns 0 when b is too short.
func U64LE(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// U32BE reads a big-endian uint32 from b. Returns 0 when b is too short.
func U32BE(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

// I32LE reads a little-endian int32 from b. Returns 0 when b is too short.
func I32LE(b []byte) int32 {
	if len(b) < 4 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(b))
}
