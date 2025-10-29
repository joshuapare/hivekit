package writer

// MemWriter captures hive bytes in memory.
type MemWriter struct {
	Buf []byte
}

// WriteHive stores the provided buffer; placeholder implementation copies it.
func (w *MemWriter) WriteHive(buf []byte) error {
	w.Buf = append(w.Buf[:0], buf...)
	return nil
}
