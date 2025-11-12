package edit

import "errors"

var (
	// ErrNotImplemented indicates a feature is not yet implemented.
	ErrNotImplemented = errors.New("edit: not implemented")

	// ErrInvalidRef indicates an invalid HCELL_INDEX.
	ErrInvalidRef = errors.New("edit: invalid cell reference")

	// ErrKeyNotFound indicates the specified key doesn't exist.
	ErrKeyNotFound = errors.New("edit: key not found")

	// ErrValueNotFound indicates the specified value doesn't exist.
	ErrValueNotFound = errors.New("edit: value not found")

	// ErrInvalidKeyName indicates an invalid or empty key name.
	ErrInvalidKeyName = errors.New("edit: invalid key name")

	// ErrInvalidValueName indicates an invalid value name.
	ErrInvalidValueName = errors.New("edit: invalid value name")

	// ErrDataTooLarge indicates data exceeds maximum supported size.
	ErrDataTooLarge = errors.New("edit: data too large")

	// ErrCannotDeleteRoot indicates an attempt to delete the root key.
	ErrCannotDeleteRoot = errors.New("edit: cannot delete root key")

	// ErrKeyHasSubkeys indicates a key has subkeys and recursive=false.
	ErrKeyHasSubkeys = errors.New("edit: key has subkeys; use recursive=true")
)
