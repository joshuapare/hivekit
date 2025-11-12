package values

import "errors"

var (
	// ErrNoValueList indicates the NK has no value list.
	ErrNoValueList = errors.New("values: no value list")

	// ErrTruncated indicates the value list data is too short.
	ErrTruncated = errors.New("values: truncated value list data")

	// ErrInvalidCount indicates the value count is inconsistent.
	ErrInvalidCount = errors.New("values: invalid value count")

	// ErrNotFound indicates the requested value was not found.
	ErrNotFound = errors.New("values: value not found")
)
