package printer

import (
	"fmt"
	"io"

	"github.com/joshuapare/hivekit/pkg/types"
)

const (
	DefaultIndentSize    = 2
	DefaultMaxDepth      = 0
	DefaultMaxValueBytes = 32
)

// Format specifies the output format for printing.
type Format string

const (
	// FormatText outputs human-readable text format.
	FormatText Format = "text"

	// FormatJSON outputs JSON format.
	FormatJSON Format = "json"

	// FormatReg outputs Windows .reg file format.
	FormatReg Format = "reg"
)

// Options controls printing behavior.
type Options struct {
	// Format specifies output format (text, json, reg).
	// Default: FormatText
	Format Format

	// IndentSize is the number of spaces per indent level (text format only).
	// Default: 2
	IndentSize int

	// MaxDepth limits recursion depth (0 = unlimited).
	// Default: 0 (unlimited)
	MaxDepth int

	// ShowValues includes value data in output.
	// Default: true
	ShowValues bool

	// ShowTimestamps includes last-write times.
	// Default: false
	ShowTimestamps bool

	// ShowValueTypes includes REG_* type names.
	// Default: true
	ShowValueTypes bool

	// Recursive enables recursive printing of subkeys.
	// Default: false
	Recursive bool

	// MaxValueBytes limits how many bytes of binary values to display.
	// Longer values are truncated. Set to 0 for no limit.
	// Default: 32
	MaxValueBytes int

	// PrintMetadata includes metadata (subkey/value counts, timestamps, etc).
	// When false, shows keys/values without metadata counts (clean tree output).
	// When true, shows full metadata including counts (dump/ls output).
	// Default: false
	PrintMetadata bool
}

// DefaultOptions returns sensible defaults for printing.
func DefaultOptions() Options {
	return Options{
		Format:         FormatText,
		IndentSize:     DefaultIndentSize,
		MaxDepth:       DefaultMaxDepth,
		ShowValues:     true,
		ShowTimestamps: false,
		ShowValueTypes: true,
		Recursive:      false,
		MaxValueBytes:  DefaultMaxValueBytes,
		PrintMetadata:  false,
	}
}

// Printer handles formatted output of registry structures.
type Printer struct {
	opts   Options
	writer io.Writer
	reader types.Reader
}

// New creates a new Printer.
//
// The Reader is used to access registry data, the Writer receives the output,
// and Options controls formatting behavior.
//
// Example:
//
//	r, _ := reader.Open("SOFTWARE")
//	p := printer.New(r, os.Stdout, printer.DefaultOptions())
//	p.PrintKey("Microsoft\\Windows\\CurrentVersion")
func New(r types.Reader, w io.Writer, opts Options) *Printer {
	return &Printer{
		reader: r,
		writer: w,
		opts:   opts,
	}
}

// PrintKey prints a key and optionally its values/children.
//
// The path is case-insensitive and common hive root prefixes are stripped.
//
// Example:
//
//	p.PrintKey("Software\\Microsoft\\Windows\\CurrentVersion")
func (p *Printer) PrintKey(path string) error {
	node, err := p.reader.Find(path)
	if err != nil {
		return fmt.Errorf("find key %q: %w", path, err)
	}

	switch p.opts.Format {
	case FormatJSON:
		return p.printKeyJSON(node, path, 0)
	case FormatReg:
		return p.printKeyReg(node, path)
	case FormatText:
		return p.printKeyText(node, 0)
	default:
		return p.printKeyText(node, 0)
	}
}

// PrintValue prints a single value.
//
// Example:
//
//	p.PrintValue("Software\\MyApp", "Version")
func (p *Printer) PrintValue(keyPath, valueName string) error {
	node, err := p.reader.Find(keyPath)
	if err != nil {
		return fmt.Errorf("find key %q: %w", keyPath, err)
	}

	valID, err := p.reader.GetValue(node, valueName)
	if err != nil {
		return fmt.Errorf("get value %q: %w", valueName, err)
	}

	switch p.opts.Format {
	case FormatJSON:
		return p.printValueJSON(valID, 0)
	case FormatText:
		return p.printValueText(valID, 0)
	case FormatReg:
		return p.printValueReg(valID)
	default:
		return p.printValueText(valID, 0)
	}
}

// PrintTree prints an entire subtree recursively.
//
// This automatically sets Recursive to true and walks the tree from the
// starting path.
//
// Example:
//
//	opts := printer.DefaultOptions()
//	opts.MaxDepth = 3
//	p := printer.New(r, os.Stdout, opts)
//	p.PrintTree("Software\\Microsoft")
func (p *Printer) PrintTree(path string) error {
	node, err := p.reader.Find(path)
	if err != nil {
		return fmt.Errorf("find key %q: %w", path, err)
	}

	switch p.opts.Format {
	case FormatJSON:
		return p.printTreeJSON(node, path, 0)
	case FormatReg:
		return p.printTreeReg(node, path, 0)
	case FormatText:
		return p.printTreeText(node, path, 0)
	default:
		return p.printTreeText(node, path, 0)
	}
}
