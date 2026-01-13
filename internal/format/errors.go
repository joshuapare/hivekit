package format

import "errors"

var (
	// ErrSignatureMismatch indicates a structure had an unexpected magic.
	ErrSignatureMismatch = errors.New("format: signature mismatch")
	// ErrTruncated indicates the buffer lacked the bytes required for a structure.
	ErrTruncated = errors.New("format: truncated buffer")
	// ErrFreeCell indicates a cell marked free was encountered where allocation was required.
	ErrFreeCell = errors.New("format: cell not in use")
	// ErrNotFound indicates a requested subkey or value was missing.
	ErrNotFound = errors.New("format: not found")
	// ErrUnsupported indicates the structure or feature is not yet supported.
	ErrUnsupported = errors.New("format: unsupported feature")

	// ErrBoundsCheck indicates a buffer access exceeded bounds.
	// This is returned by Checked* encoding functions when the offset
	// or required size would exceed the buffer length.
	ErrBoundsCheck = errors.New("format: buffer bounds exceeded")

	// ErrSanityLimit indicates a parsed value exceeded sanity limits.
	// This prevents integer overflow attacks and excessive allocations
	// from malformed hive files.
	ErrSanityLimit = errors.New("format: value exceeds sanity limit")

	// ErrIntegerOverflow indicates an integer operation would overflow.
	// This is returned when count * elementSize or similar calculations
	// would exceed the maximum int value.
	ErrIntegerOverflow = errors.New("format: integer overflow")
)
