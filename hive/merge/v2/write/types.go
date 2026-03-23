package write

// InPlaceUpdate represents a modification to an existing cell.
type InPlaceUpdate struct {
	Offset   int32 // absolute file offset
	Data     []byte // bytes to write
	Category int   // flush ordering category (0=NK field, 1=SK refcount, 2=cell free)
}

// WriteStats tracks what was written during the execute phase.
type WriteStats struct {
	KeysCreated   int
	ValuesSet     int
	ValuesDeleted int
	KeysDeleted   int
}
