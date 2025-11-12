package format

import "encoding/binary"

// Binary encoding utilities for little-endian integers.
//
// This package provides efficient encoding/decoding of integers in the
// Windows Registry hive format, which uses little-endian byte order.
//
// Implementation: Uses encoding/binary.LittleEndian
//
// Performance Note: After benchmarking, we determined that Go's standard
// library implementation is already highly optimized by the compiler.
// Unsafe pointer implementations provided no measurable benefit and added
// complexity. Modern Go compilers inline and optimize binary.LittleEndian
// calls extremely well.

// PutU16 writes a uint16 value to the buffer at the specified offset in little-endian format.
func PutU16(b []byte, off int, v uint16) {
	binary.LittleEndian.PutUint16(b[off:off+2], v)
}

// PutU32 writes a uint32 value to the buffer at the specified offset in little-endian format.
func PutU32(b []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(b[off:off+4], v)
}

// PutI32 writes an int32 value to the buffer at the specified offset in little-endian format.
func PutI32(b []byte, off int, v int32) {
	binary.LittleEndian.PutUint32(b[off:off+4], uint32(v))
}

// PutU64 writes a uint64 value to the buffer at the specified offset in little-endian format.
func PutU64(b []byte, off int, v uint64) {
	binary.LittleEndian.PutUint64(b[off:off+8], v)
}

// ReadU16 reads a uint16 value from the buffer at the specified offset in little-endian format.
func ReadU16(b []byte, off int) uint16 {
	return binary.LittleEndian.Uint16(b[off : off+2])
}

// ReadU32 reads a uint32 value from the buffer at the specified offset in little-endian format.
func ReadU32(b []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(b[off : off+4])
}

// ReadI32 reads an int32 value from the buffer at the specified offset in little-endian format.
func ReadI32(b []byte, off int) int32 {
	return int32(binary.LittleEndian.Uint32(b[off : off+4]))
}

// ReadU64 reads a uint64 value from the buffer at the specified offset in little-endian format.
func ReadU64(b []byte, off int) uint64 {
	return binary.LittleEndian.Uint64(b[off : off+8])
}
