//go:build unix

package mmfile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Map maps the file at path into memory and returns its contents.
func Map(path string) ([]byte, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close() // safe before return; mapping keeps pages alive

	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	size := info.Size()
	if size == 0 {
		return []byte{}, func() error { return nil }, nil
	}
	if size > int64(^uint(0)>>1) {
		return nil, nil, fmt.Errorf("mmfile: file too large to map (%d bytes)", size)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() error {
		if data == nil {
			return nil
		}
		err := syscall.Munmap(data)
		if errors.Is(err, syscall.EINVAL) {
			// Treat double-unmap as no-op for callers.
			return nil
		}
		return err
	}
	return data, cleanup, nil
}
