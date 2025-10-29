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
)
