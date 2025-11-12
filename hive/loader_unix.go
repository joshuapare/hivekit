//go:build linux || darwin

package hive

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/joshuapare/hivekit/internal/format"
)

// Open mmaps the hive RW so we can mutate in place.
func Open(path string) (*Hive, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	sz := st.Size()
	if sz == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("empty hive file: %s", path)
	}

	data, err := syscall.Mmap(
		int(f.Fd()),
		0,
		int(sz),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	bb, err := ParseBaseBlock(data)
	if err != nil {
		_ = syscall.Munmap(data)
		_ = f.Close()
		return nil, err
	}

	if validateErr := bb.ValidateSanity(len(data)); validateErr != nil {
		_ = syscall.Munmap(data)
		_ = f.Close()
		return nil, validateErr
	}

	h := &Hive{
		f:    f,
		data: data,
		size: sz,
		base: bb,
	}

	// CRITICAL: Truncate any trailing slack space immediately after opening.
	// Per Windows Registry spec, HBINs must be contiguous from 0x1000 onwards.
	// This must happen BEFORE any code (like index builders) stores references to the data.
	// The data size field at offset 0x28 indicates the logical end of HBINs.
	headerDataSize := format.ReadU32(data, format.REGFDataSizeOffset)
	logicalEnd := format.HeaderSize + int64(headerDataSize)
	if sz > logicalEnd {
		// Truncate trailing slack
		if truncateErr := h.Truncate(logicalEnd); truncateErr != nil {
			_ = h.Close()
			return nil, fmt.Errorf("truncate trailing slack: %w", truncateErr)
		}
	}

	return h, nil
}

func (h *Hive) Close() error {
	var err error
	if h.data != nil {
		_ = syscall.Munmap(h.data)
		h.data = nil
	}
	if h.f != nil {
		err = h.f.Close()
		h.f = nil
	}
	h.base = nil
	return err
}

// Append grows the hive file by n bytes and remaps the memory mapping.
// The new bytes are zero-initialized by the OS.
func (h *Hive) Append(n int64) error {
	if h == nil || h.f == nil {
		return errors.New("hive: cannot append to nil or closed hive")
	}
	if n <= 0 {
		return nil
	}

	newSize := h.size + n

	// Unmap the current mapping
	if h.data != nil {
		if err := syscall.Munmap(h.data); err != nil {
			return fmt.Errorf("hive: failed to unmap before grow: %w", err)
		}
		h.data = nil
	}

	// Truncate file to new size (extends with zeros)
	if err := h.f.Truncate(newSize); err != nil {
		// Try to remap old size to recover
		data, _ := syscall.Mmap(
			int(h.f.Fd()),
			0,
			int(h.size),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED,
		)
		h.data = data
		return fmt.Errorf("hive: failed to truncate file: %w", err)
	}

	// Remap the entire file at the new size
	data, err := syscall.Mmap(
		int(h.f.Fd()),
		0,
		int(newSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
	if err != nil {
		// Try to remap old size to recover
		oldData, _ := syscall.Mmap(
			int(h.f.Fd()),
			0,
			int(h.size),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED,
		)
		h.data = oldData
		return fmt.Errorf("hive: failed to remap after grow: %w", err)
	}

	h.data = data
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

// Truncate shrinks the hive file to the specified size and remaps the memory mapping.
// This is used to remove trailing slack space before appending new HBINs.
func (h *Hive) Truncate(newSize int64) error {
	if h == nil || h.f == nil {
		return errors.New("hive: cannot truncate nil or closed hive")
	}
	if newSize < int64(format.HeaderSize) {
		return fmt.Errorf("hive: truncate size %d too small (minimum %d)", newSize, format.HeaderSize)
	}
	if newSize > h.size {
		return fmt.Errorf(
			"hive: truncate cannot grow (current: %d, requested: %d), use Append instead",
			h.size,
			newSize,
		)
	}
	if newSize == h.size {
		return nil // No-op
	}

	// Unmap the current mapping
	if h.data != nil {
		if err := syscall.Munmap(h.data); err != nil {
			return fmt.Errorf("hive: failed to unmap before truncate: %w", err)
		}
		h.data = nil
	}

	// Truncate file to new size
	if err := h.f.Truncate(newSize); err != nil {
		// Try to remap old size to recover
		data, _ := syscall.Mmap(
			int(h.f.Fd()),
			0,
			int(h.size),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED,
		)
		h.data = data
		return fmt.Errorf("hive: failed to truncate file: %w", err)
	}

	// Remap the file at the new size
	data, err := syscall.Mmap(
		int(h.f.Fd()),
		0,
		int(newSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
	if err != nil {
		// Try to remap old size to recover
		oldData, _ := syscall.Mmap(
			int(h.f.Fd()),
			0,
			int(h.size),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED,
		)
		h.data = oldData
		return fmt.Errorf("hive: failed to remap after truncate: %w", err)
	}

	h.data = data
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
