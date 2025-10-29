//go:build !unix && !windows

// Package mmfile provides platform-specific helpers for memory-mapping hive files.
package mmfile

import "os"

// Map reads the entire file when mmap is not available.
func Map(path string) ([]byte, func() error, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return data, func() error { return nil }, nil
}
