package alloc

import "errors"

var (
	// ErrNoSpace indicates that no free cell large enough was found and growth failed.
	ErrNoSpace = errors.New("alloc: no free cell large enough")

	// ErrBadRef indicates an invalid or out-of-bounds cell reference.
	ErrBadRef = errors.New("alloc: bad cell reference")

	// ErrGrowFail indicates that attempting to grow the hive (append HBIN) failed.
	ErrGrowFail = errors.New("alloc: grow failed")

	// ErrNotFree indicates an attempt to free a cell that is not marked as free.
	ErrNotFree = errors.New("alloc: expected free cell")

	// ErrNeedSmall indicates the requested size is too small (must include 4-byte header).
	ErrNeedSmall = errors.New("alloc: need must include header and be >= 4 bytes")
)
