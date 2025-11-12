package subkeys

import "errors"

var (
	// ErrInvalidSignature indicates the list signature is not LF, LH, LI, or RI.
	ErrInvalidSignature = errors.New("subkeys: invalid list signature")

	// ErrInvalidCount indicates the list count is inconsistent or corrupted.
	ErrInvalidCount = errors.New("subkeys: invalid entry count")

	// ErrTruncated indicates the list data is too short for the declared count.
	ErrTruncated = errors.New("subkeys: truncated list data")

	// ErrDuplicateKey indicates a duplicate key name was found.
	ErrDuplicateKey = errors.New("subkeys: duplicate key name")

	// ErrNotFound indicates the requested key was not found.
	ErrNotFound = errors.New("subkeys: key not found")

	// ErrInvalidRI indicates an RI (indirect) list has invalid structure.
	ErrInvalidRI = errors.New("subkeys: invalid RI list structure")
)
