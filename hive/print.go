package hive

import (
	"fmt"
	"io"

	"github.com/joshuapare/hivekit/hive/printer"
)

// PrintKey prints a registry key and optionally its values.
//
// The output format is controlled by the Options.Format field (text, json, or reg).
// By default, only the key metadata is printed. Set Options.ShowValues to true
// to include value data.
//
// Example:
//
//	h, _ := hive.Open("SOFTWARE")
//	defer h.Close()
//
//	opts := printer.DefaultOptions()
//	opts.ShowValues = true
//	h.PrintKey(os.Stdout, "Microsoft\\Windows\\CurrentVersion", opts)
func (h *Hive) PrintKey(w io.Writer, path string, opts printer.Options) error {
	r, err := h.Reader()
	if err != nil {
		return fmt.Errorf("get reader: %w", err)
	}

	p := printer.New(r, w, opts)
	return p.PrintKey(path)
}

// PrintValue prints a single registry value.
//
// The value is automatically decoded based on its type (REG_SZ, REG_DWORD, etc.).
// The output format is controlled by Options.Format.
//
// Example:
//
//	h, _ := hive.Open("SOFTWARE")
//	defer h.Close()
//
//	opts := printer.DefaultOptions()
//	opts.ShowValueTypes = true
//	h.PrintValue(os.Stdout, "Microsoft\\Windows NT\\CurrentVersion", "ProductName", opts)
func (h *Hive) PrintValue(w io.Writer, keyPath, valueName string, opts printer.Options) error {
	r, err := h.Reader()
	if err != nil {
		return fmt.Errorf("get reader: %w", err)
	}

	p := printer.New(r, w, opts)
	return p.PrintValue(keyPath, valueName)
}

// PrintTree prints an entire registry subtree recursively.
//
// The output includes all subkeys and values from the starting path. Use
// Options.MaxDepth to limit recursion depth. This is useful for exporting
// entire registry sections.
//
// Example:
//
//	h, _ := hive.Open("SOFTWARE")
//	defer h.Close()
//
//	opts := printer.DefaultOptions()
//	opts.Format = printer.FormatReg
//	opts.ShowValues = true
//	opts.MaxDepth = 3
//
//	// Export to .reg file
//	f, _ := os.Create("export.reg")
//	defer f.Close()
//	h.PrintTree(f, "Microsoft\\Windows\\CurrentVersion\\Uninstall", opts)
func (h *Hive) PrintTree(w io.Writer, path string, opts printer.Options) error {
	r, err := h.Reader()
	if err != nil {
		return fmt.Errorf("get reader: %w", err)
	}

	p := printer.New(r, w, opts)
	return p.PrintTree(path)
}
