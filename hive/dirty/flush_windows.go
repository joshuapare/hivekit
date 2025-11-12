//go:build windows

package dirty

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

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
