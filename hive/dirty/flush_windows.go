//go:build windows

package dirty

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

// flushRanges flushes dirty ranges to disk.
//
// On Windows, we flush each coalesced range individually using FlushViewOfFile.
// The context can be used to cancel the operation between range flushes.
func (t *Tracker) flushRanges(ctx context.Context, data []byte) error {
	// Coalesce ranges
	coalesced := t.coalesce()

	// Flush each range (excluding header)
	for _, r := range coalesced {
		// Check for cancellation between ranges
		if err := ctx.Err(); err != nil {
			return err
		}

		// Skip header range (offset 0)
		if r.Off == 0 {
			continue
		}

		// Bounds check
		start := int(r.Off)
		end := int(r.Off + r.Len)
		if end > len(data) {
			continue
		}

		// Flush this range
		if err := msync(data[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// msync performs memory sync for the given byte slice using FlushViewOfFile.
//
// On Windows, FlushViewOfFile flushes the memory-mapped pages to disk.
func msync(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// FlushViewOfFile requires the address and length
	// Use unsafe.Pointer in a single expression to avoid linter warnings
	addr := uintptr(unsafe.Pointer(&data[0]))
	return windows.FlushViewOfFile(addr, uintptr(len(data)))
}

// fdatasync performs file descriptor sync using FlushFileBuffers.
//
// On Windows, FlushFileBuffers ensures all file data and metadata is written to disk.
// The fullfsync parameter is ignored on Windows.
func fdatasync(fd int, _ bool) error {
	return windows.FlushFileBuffers(windows.Handle(fd))
}
