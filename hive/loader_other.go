//go:build !linux && !darwin

package hive

import (
	"fmt"
	"io"
	"os"

	"github.com/joshuapare/hivekit/internal/format"
)

// Open loads the hive into memory on non-unix platforms (or when mmap isn't used).
func Open(path string) (*Hive, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	sz := st.Size()
	if sz == 0 {
		f.Close()
		return nil, fmt.Errorf("empty hive file: %s", path)
	}

	buf := make([]byte, sz)
	if _, err := io.ReadFull(f, buf); err != nil {
		f.Close()
		return nil, err
	}

	// parse REGF
	bb, err := ParseBaseBlock(buf)
	if err != nil {
		f.Close()
		return nil, err
	}

	// check vs actual file size (must allow equality)
	if err := bb.ValidateSanity(len(buf)); err != nil {
		f.Close()
		return nil, err
	}

	h := &Hive{
		f:    f,
		data: buf,
		size: sz,
		base: bb,
	}
	return h, nil
}

func (h *Hive) Close() error {
	var err error
	if h.f != nil {
		err = h.f.Close()
		h.f = nil
	}
	h.data = nil
	h.base = nil
	return err
}

// Append grows the hive file by n bytes and extends the in-memory buffer.
// The new bytes are zero-initialized.
func (h *Hive) Append(n int64) error {
	if h == nil || h.f == nil {
		return fmt.Errorf("hive: cannot append to nil or closed hive")
	}
	if n <= 0 {
		return nil
	}

	newSize := h.size + n

	// Grow the byte slice (zeros are automatically added)
	newData := make([]byte, newSize)
	copy(newData, h.data)

	// Write the new data to the file
	if _, err := h.f.Seek(h.size, 0); err != nil {
		return fmt.Errorf("hive: failed to seek to end: %w", err)
	}
	zeros := make([]byte, n)
	if _, err := h.f.Write(zeros); err != nil {
		return fmt.Errorf("hive: failed to write extension: %w", err)
	}

	h.data = newData
	h.size = newSize

	// CRITICAL: Re-parse base block since h.data changed
	// The old base block wraps a slice into the old data, which is now invalid
	bb, err := ParseBaseBlock(h.data)
	if err != nil {
		return fmt.Errorf("hive: failed to re-parse base block after append: %w", err)
	}
	h.base = bb

	return nil
}

// Truncate shrinks the hive file to newSize bytes and resizes the in-memory buffer.
// This is used for space reclamation after removing HBINs.
// Returns error if newSize is invalid or larger than current size.
func (h *Hive) Truncate(newSize int64) error {
	if h == nil || h.f == nil {
		return fmt.Errorf("hive: cannot truncate nil or closed hive")
	}
	if newSize < int64(format.HeaderSize) {
		return fmt.Errorf("hive: truncate size %d too small (minimum %d)", newSize, format.HeaderSize)
	}
	if newSize > h.size {
		return fmt.Errorf("hive: truncate cannot grow (current: %d, requested: %d), use Append instead", h.size, newSize)
	}
	if newSize == h.size {
		return nil // No-op
	}

	// Truncate the file first (before allocating new memory)
	if err := h.f.Truncate(newSize); err != nil {
		return fmt.Errorf("hive: failed to truncate file: %w", err)
	}

	// Now create the smaller slice and copy data
	// This avoids temporarily doubling memory usage
	newData := make([]byte, newSize)
	copy(newData, h.data[:newSize])

	h.data = newData
	h.size = newSize

	// CRITICAL: Re-parse base block since h.data changed
	// The old base block wraps a slice into the old data, which is now invalid
	bb, err := ParseBaseBlock(h.data)
	if err != nil {
		return fmt.Errorf("hive: failed to re-parse base block after truncate: %w", err)
	}
	h.base = bb

	return nil
}
