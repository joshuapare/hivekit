package hive

import (
	"github.com/joshuapare/hivekit/internal/reader"
)

// Open opens a registry hive file for reading.
// Returns a Reader interface that can be used to query the hive contents.
// The caller must call Close() when done to release resources.
//
// Example:
//
//	r, err := ops.Open("system.hive", ops.OpenOptions{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer r.Close()
func Open(path string, opts OpenOptions) (Reader, error) {
	return reader.Open(path, opts)
}

// OpenBytes opens a registry hive from a byte slice.
// Returns a Reader interface that can be used to query the hive contents.
// The caller must call Close() when done to release resources.
//
// Example:
//
//	data, _ := os.ReadFile("system.hive")
//	r, err := ops.OpenBytes(data, ops.OpenOptions{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer r.Close()
func OpenBytes(buf []byte, opts OpenOptions) (Reader, error) {
	return reader.OpenBytes(buf, opts)
}
