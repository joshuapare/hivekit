//go:build linux || freebsd

package dirty

import (
	"golang.org/x/sys/unix"
)

// flushRanges flushes individual dirty ranges to disk.
//
// On Linux and other Unix systems, msync() can handle sub-slices correctly.
func (t *Tracker) flushRanges(data []byte) error {
	// Coalesce ranges
	coalesced := t.coalesce()

	// Flush each range (excluding header)
	for _, r := range coalesced {
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
		if err := unix.Msync(data[start:end], unix.MS_SYNC); err != nil {
			return err
		}
	}

	return nil
}

// msync flushes a memory region to disk.
func msync(data []byte) error {
	return unix.Msync(data, unix.MS_SYNC)
}

// fdatasync performs file descriptor sync.
//
// On Linux/FreeBSD, fdatasync() provides sufficient guarantees.
// The fullfsync parameter is ignored on Linux/FreeBSD.
func fdatasync(fd int, _ bool) error {
	// Linux/FreeBSD: fdatasync
	return unix.Fdatasync(fd)
}
