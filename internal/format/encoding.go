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

// ============================================================================
// Checked Encoding Functions
// ============================================================================
//
// These functions perform bounds checking before accessing buffer memory.
// Use these when parsing untrusted input (hive files, cell payloads).
// They return ErrBoundsCheck instead of panicking on out-of-bounds access.

// CheckedReadU16 reads a uint16 with bounds checking.
// Returns (0, ErrBoundsCheck) if b[off:off+2] would exceed buffer bounds.
func CheckedReadU16(b []byte, off int) (uint16, error) {
	if off < 0 || off+2 > len(b) {
		return 0, ErrBoundsCheck
	}
	return binary.LittleEndian.Uint16(b[off : off+2]), nil
}

// CheckedReadU32 reads a uint32 with bounds checking.
// Returns (0, ErrBoundsCheck) if b[off:off+4] would exceed buffer bounds.
func CheckedReadU32(b []byte, off int) (uint32, error) {
	if off < 0 || off+4 > len(b) {
		return 0, ErrBoundsCheck
	}
	return binary.LittleEndian.Uint32(b[off : off+4]), nil
}

// CheckedReadI32 reads an int32 with bounds checking.
// Returns (0, ErrBoundsCheck) if b[off:off+4] would exceed buffer bounds.
func CheckedReadI32(b []byte, off int) (int32, error) {
	if off < 0 || off+4 > len(b) {
		return 0, ErrBoundsCheck
	}
	return int32(binary.LittleEndian.Uint32(b[off : off+4])), nil
}

// CheckedReadU64 reads a uint64 with bounds checking.
// Returns (0, ErrBoundsCheck) if b[off:off+8] would exceed buffer bounds.
func CheckedReadU64(b []byte, off int) (uint64, error) {
	if off < 0 || off+8 > len(b) {
		return 0, ErrBoundsCheck
	}
	return binary.LittleEndian.Uint64(b[off : off+8]), nil
}

// CheckedPutU16 writes a uint16 with bounds checking.
// Returns ErrBoundsCheck if b[off:off+2] would exceed buffer bounds.
func CheckedPutU16(b []byte, off int, v uint16) error {
	if off < 0 || off+2 > len(b) {
		return ErrBoundsCheck
	}
	binary.LittleEndian.PutUint16(b[off:off+2], v)
	return nil
}

// CheckedPutU32 writes a uint32 with bounds checking.
// Returns ErrBoundsCheck if b[off:off+4] would exceed buffer bounds.
func CheckedPutU32(b []byte, off int, v uint32) error {
	if off < 0 || off+4 > len(b) {
		return ErrBoundsCheck
	}
	binary.LittleEndian.PutUint32(b[off:off+4], v)
	return nil
}

// CheckedPutI32 writes an int32 with bounds checking.
// Returns ErrBoundsCheck if b[off:off+4] would exceed buffer bounds.
func CheckedPutI32(b []byte, off int, v int32) error {
	if off < 0 || off+4 > len(b) {
		return ErrBoundsCheck
	}
	binary.LittleEndian.PutUint32(b[off:off+4], uint32(v))
	return nil
}

// CheckedPutU64 writes a uint64 with bounds checking.
// Returns ErrBoundsCheck if b[off:off+8] would exceed buffer bounds.
func CheckedPutU64(b []byte, off int, v uint64) error {
	if off < 0 || off+8 > len(b) {
		return ErrBoundsCheck
	}
	binary.LittleEndian.PutUint64(b[off:off+8], v)
	return nil
}
