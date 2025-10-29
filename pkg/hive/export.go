package hive

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
)

// ExportReg exports a registry hive (or subtree) to .reg format.
//
// The output is written to regPath in Windows Registry Editor format.
// By default, the entire hive is exported. Use opts.SubtreePath to export
// only a specific subtree.
//
// Example (export entire hive):
//
//	err := ops.ExportReg("software.hive", "backup.reg", nil)
//
// Example (export subtree):
//
//	opts := &ops.ExportOptions{
//	    SubtreePath: "Microsoft\\Windows",
//	}
//	err := ops.ExportReg("software.hive", "windows.reg", opts)
func ExportReg(hivePath, regPath string, opts *ExportOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Export to string
	regContent, err := ExportRegString(hivePath, opts)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(regPath, []byte(regContent), 0644); err != nil {
		return fmt.Errorf("failed to write .reg file %s: %w", regPath, err)
	}

	return nil
}

// ExportRegString exports a registry hive (or subtree) to a string.
//
// This returns the .reg content as a string instead of writing to a file.
// Useful for in-memory processing or testing.
//
// Example:
//
//	regContent, err := ops.ExportRegString("software.hive", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(regContent)
func ExportRegString(hivePath string, opts *ExportOptions) (string, error) {
	if !fileExists(hivePath) {
		return "", fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Apply defaults
	if opts == nil {
		opts = &ExportOptions{}
	}
	if opts.Encoding == "" {
		opts.Encoding = "UTF-8" // Default to UTF-8 for simplicity
	}
	// BOM is default for UTF-16LE
	if opts.Encoding == "UTF-16LE" && !opts.WithBOM {
		opts.WithBOM = true
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return "", fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Determine root node for export
	rootNode, err := r.Root()
	if err != nil {
		return "", fmt.Errorf("failed to get root node: %w", err)
	}

	// If subtree specified, find that node
	if opts.SubtreePath != "" {
		subtreeNode, err := r.Find(opts.SubtreePath)
		if err != nil {
			return "", fmt.Errorf("failed to find subtree %s: %w", opts.SubtreePath, err)
		}
		rootNode = subtreeNode
	}

	// Create codec and export
	codec := regtext.NewCodec()
	exportOpts := RegExportOptions{
		OutputEncoding: opts.Encoding,
		WithBOM:        opts.WithBOM,
	}

	regData, err := codec.ExportReg(r, rootNode, exportOpts)
	if err != nil {
		return "", fmt.Errorf("failed to export .reg data: %w", err)
	}

	return string(regData), nil
}
