//go:build darwin

package dirty

import (
	"golang.org/x/sys/unix"
)

// flushRanges flushes dirty ranges to disk.
//
// On macOS, msync() requires the address to match the original mmap() address.
// We cannot pass sub-slices because their base pointer differs from the mmap address.
// Solution: Flush the entire mmap'd region. The kernel only writes dirty pages anyway.
func (t *Tracker) flushRanges(data []byte) error {
	// On Darwin, we must sync the entire mmap'd region
	// The kernel will only write pages that are actually dirty
	return unix.Msync(data, unix.MS_SYNC)
}

// msync flushes a memory region to disk.
func msync(data []byte) error {
	return unix.Msync(data, unix.MS_SYNC)
}

// fdatasync performs file descriptor sync.
//
// On macOS, if fullfsync is true, use F_FULLFSYNC for maximum durability.
// F_FULLFSYNC ensures data is written to the physical disk, not just the drive cache.
// Otherwise, use regular fsync.
func fdatasync(fd int, fullfsync bool) error {
	if fullfsync {
		// macOS: F_FULLFSYNC for power-loss durability
		_, err := unix.FcntlInt(uintptr(fd), unix.F_FULLFSYNC, 0)
		return err
	}
	// macOS doesn't have fdatasync, use fsync
	return unix.Fsync(fd)
}
