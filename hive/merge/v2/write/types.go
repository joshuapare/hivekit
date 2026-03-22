package write

// InPlaceUpdate represents a modification to an existing cell.
type InPlaceUpdate struct {
	Offset int32  // absolute file offset
	Data   []byte // bytes to write
}

// WriteStats tracks what was written during the execute phase.
type WriteStats struct {
	KeysCreated   int
	ValuesSet     int
	ValuesDeleted int
	KeysDeleted   int
}
