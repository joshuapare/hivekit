package format

// Alignment utilities for Windows Registry hive format.
// The hive format requires various data structures to be aligned to specific byte boundaries.

// Align8 returns n aligned up to the next 8-byte boundary.
// Used for cell sizes and other structures that must be 8-byte aligned.
//
// Example:
//
//	Align8(1)  = 8
//	Align8(8)  = 8
//	Align8(9)  = 16
//	Align8(16) = 16
func Align8(n int) int {
	return (n + CellAlignmentMask) & ^CellAlignmentMask
}

// AlignHBIN returns n aligned up to the next 4KB (4096-byte) boundary.
// Used for HBIN (hive bin) structures which must start on 4KB boundaries.
//
// Example:
//
//	AlignHBIN(1)    = 4096
//	AlignHBIN(4096) = 4096
//	AlignHBIN(4097) = 8192
func AlignHBIN(n int) int {
	return (n + HBINAlignmentMask) & ^HBINAlignmentMask
}

// Align16 returns n aligned up to the next 16-byte boundary.
// May be used for certain data structures requiring 16-byte alignment.
//
// Example:
//
//	Align16(1)  = 16
//	Align16(16) = 16
//	Align16(17) = 32
func Align16(n int) int {
	return (n + Align16Mask) & ^Align16Mask
}

// Align8I32 returns n aligned up to the next 8-byte boundary.
// int32 version for use in allocator code to avoid G115 warnings.
func Align8I32(n int32) int32 {
	return (n + CellAlignmentMask) & ^CellAlignmentMask
}

// AlignHBINI32 returns n aligned up to the next 4KB (4096-byte) boundary.
// int32 version for use in allocator code to avoid G115 warnings.
func AlignHBINI32(n int32) int32 {
	return (n + HBINAlignmentMask) & ^HBINAlignmentMask
}
