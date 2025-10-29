package hive

import (
	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
)

// Open opens a registry hive file for reading.
// Returns a Reader interface that can be used to query the 
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
// Returns a Reader interface that can be used to query the 
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

// NewEditor creates an Editor for transaction-based editing of a 
// The Editor allows creating transactions that can be committed or rolled back atomically.
//
// Example:
//
//	r, _ := ops.Open("system.hive", ops.OpenOptions{})
//	defer r.Close()
//
//	ed := ops.NewEditor(r)
//	tx := ed.Begin()
//	tx.CreateKey("Software\\MyApp", ops.CreateKeyOptions{CreateParents: true})
//	tx.Commit(&ops.FileWriter{Path: "output.hive"}, ops.WriteOptions{})
func NewEditor(r Reader) Editor {
	return edit.NewEditor(r)
}

// NewHive creates an empty hive that can be populated using transactions.
// Unlike NewEditor which modifies an existing hive, NewHive creates a hive from scratch
// with only a root key (no values or subkeys).
//
// This is useful for:
//   - Creating new registry hives programmatically
//   - Building test hives with specific structures
//   - Generating hives for deployment or configuration
//
// Example:
//
//	ed := hive.NewHive()
//	tx := ed.Begin()
//	tx.CreateKey("Software\\MyApp", hive.CreateKeyOptions{CreateParents: true})
//	tx.SetValue("Software\\MyApp", "Version", hive.REG_SZ, encodeUTF16("1.0"))
//	hiveData, _ := tx.Commit(hive.WriteOptions{})
//	os.WriteFile("my.hive", hiveData, 0644)
func NewHive() Editor {
	return edit.NewEditor(nil)
}
