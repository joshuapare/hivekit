//go:build windows

package mmfile

import (
	"os"
)

// Map maps the file at path into memory and returns its contents.
func Map(path string) ([]byte, func() error, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return data, func() error { return nil }, nil
}
