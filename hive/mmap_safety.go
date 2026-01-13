//go:build linux

package hive

import (
	"fmt"
	"runtime/debug"
	"syscall"
	"unsafe"
)

// MADV_POPULATE_READ is available since Linux 5.14.
// It pre-faults pages and returns EFAULT instead of generating SIGBUS.
const (
	MADV_POPULATE_READ  = 22 // Linux 5.14+
	MADV_POPULATE_WRITE = 23 // Linux 5.14+
)

// PreFaultPages attempts to pre-fault all pages in the mapped region to detect
// inaccessible memory before it causes SIGBUS during normal access.
//
// This function tries multiple strategies in order of preference:
// 1. MADV_POPULATE_READ (Linux 5.14+) - returns error instead of SIGBUS
// 2. Manual read-through with SetPanicOnFault protection
//
// Returns nil if all pages are accessible, or an error describing which
// region failed.
func PreFaultPages(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Strategy 1: Try MADV_POPULATE_READ (Linux 5.14+)
	// This is the best option - it returns EFAULT instead of SIGBUS
	err := tryMadvisePopulate(data)
	if err == nil {
		return nil // Success - all pages are accessible
	}
	// If MADV_POPULATE_READ isn't supported (EINVAL), fall through to manual approach
	if err != syscall.EINVAL && err != syscall.ENOSYS {
		// A real error occurred (e.g., EFAULT means inaccessible pages)
		return fmt.Errorf("madvise populate failed: %w", err)
	}

	// Strategy 2: Manual read-through with panic recovery
	// This forces all pages to be loaded, catching any SIGBUS as a panic
	return manualPreFault(data)
}

// tryMadvisePopulate attempts to use MADV_POPULATE_READ to pre-fault pages.
// Returns nil on success, EINVAL/ENOSYS if not supported, or other error on failure.
func tryMadvisePopulate(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Get the base pointer and length for madvise
	ptr := unsafe.Pointer(&data[0])
	_, _, errno := syscall.Syscall(
		syscall.SYS_MADVISE,
		uintptr(ptr),
		uintptr(len(data)),
		uintptr(MADV_POPULATE_READ),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// manualPreFault reads through all pages to force them to be loaded.
// Uses SetPanicOnFault to convert any SIGBUS to a recoverable panic.
func manualPreFault(data []byte) (retErr error) {
	// Enable panic-on-fault for this goroutine
	// The deferred restore ensures we reset to the previous state
	oldSetting := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(oldSetting)

	// Set up recovery to catch any SIGBUS converted to panic
	defer func() {
		if r := recover(); r != nil {
			// Extract fault address if available
			if err, ok := r.(error); ok {
				retErr = fmt.Errorf("memory access fault during pre-fault: %w", err)
			} else {
				retErr = fmt.Errorf("memory access fault during pre-fault: %v", r)
			}
		}
	}()

	// Read through all pages (page size is typically 4KB)
	// We only need to touch one byte per page to fault it in
	const pageSize = 4096
	var sink byte // Prevent compiler from optimizing away reads

	for i := 0; i < len(data); i += pageSize {
		sink ^= data[i]
	}
	// Also touch the last byte to ensure the final partial page is faulted
	if len(data) > 0 {
		sink ^= data[len(data)-1]
	}

	// Use sink to prevent compiler optimization
	_ = sink

	return nil
}

// ValidateMappedRegion performs comprehensive validation of a memory-mapped region.
// It checks both Go-level bounds and actual memory accessibility.
//
// This should be called after mmap to ensure the entire region is accessible
// before any processing begins.
func ValidateMappedRegion(data []byte, expectedSize int64) error {
	// Check Go-level length matches expected size
	if int64(len(data)) != expectedSize {
		return fmt.Errorf("mapped size mismatch: got %d, expected %d", len(data), expectedSize)
	}

	// Pre-fault all pages to detect inaccessible regions
	if err := PreFaultPages(data); err != nil {
		return fmt.Errorf("mapped region contains inaccessible pages: %w", err)
	}

	return nil
}
