# Printer Package

High-performance, flexible printing for Windows Registry hive data. Supports text, JSON, and .reg file formats with full control over output detail and formatting.

## Overview

The printer package provides formatted output for Windows Registry hive files. It works with hive files on disk (not live registries), making it ideal for forensic analysis, offline processing, and cross-platform registry inspection.

**Common Windows hive file locations:**
- `C:\Windows\System32\config\SOFTWARE` - Installed software and system settings
- `C:\Windows\System32\config\SYSTEM` - System configuration and services
- `C:\Windows\System32\config\SAM` - User accounts and security
- `C:\Users\<username>\NTUSER.DAT` - User-specific settings
- `C:\Users\<username>\AppData\Local\Microsoft\Windows\UsrClass.dat` - User application classes

On Linux/macOS, you might extract these from disk images or network shares:
- `/mnt/windows/System32/config/SOFTWARE`
- `/forensics/evidence/NTUSER.DAT`
- `./extracted-hives/SYSTEM`

## Features

- **Three output formats** - Human-readable text, structured JSON, and Windows .reg file format
- **Flexible options** - Control depth, value display, timestamps, type names, and more
- **All value types supported** - REG_SZ, REG_DWORD, REG_QWORD, REG_BINARY, REG_MULTI_SZ, and all others
- **Zero-copy performance** - Direct access to memory-mapped hive data
- **Tree traversal** - Recursively print entire subtrees with depth limits
- **Value decoding** - Automatic decoding of strings (UTF-16LE to UTF-8), integers, and binary data

## Quick Start

### Using the Hive Handle (Recommended)

The simplest way to print registry data is through the Hive handle methods:

```go
package main

import (
    "os"
    "github.com/joshuapare/hivekit/hive"
    "github.com/joshuapare/hivekit/hive/printer"
)

func main() {
    // Open a hive file
    h, err := hive.Open("/path/to/SOFTWARE")
    if err != nil {
        panic(err)
    }
    defer h.Close()

    // Print a key with default options
    opts := printer.DefaultOptions()
    opts.ShowValues = true
    opts.PrintMetadata = true  // Enable metadata display

    h.PrintKey(os.Stdout, "Microsoft\\Windows\\CurrentVersion", opts)
}
```

**Output:**
```
[CurrentVersion]
  Subkeys: 15, Values: 8
  "ProgramFilesDir" [REG_SZ] = "C:\Program Files"
  "CommonFilesDir" [REG_SZ] = "C:\Program Files\Common Files"
  "ProgramFilesDir (x86)" [REG_SZ] = "C:\Program Files (x86)"
  "ProgramW6432Dir" [REG_SZ] = "C:\Program Files"
```

### Using the Printer Directly

For more control or when you already have a Reader:

```go
package main

import (
    "os"
    "github.com/joshuapare/hivekit/hive"
    "github.com/joshuapare/hivekit/hive/printer"
)

func main() {
    h, _ := hive.Open("/path/to/SOFTWARE")
    defer h.Close()

    // Get a reader
    r, _ := h.Reader()

    // Create a printer
    opts := printer.DefaultOptions()
    opts.ShowValues = true
    opts.ShowTimestamps = true

    p := printer.New(r, os.Stdout, opts)

    // Print a key
    p.PrintKey("Microsoft\\Windows\\CurrentVersion")
}
```

## Output Formats

### Text Format (Default)

Human-readable hierarchical output with indentation.

```go
opts := printer.DefaultOptions()
opts.Format = printer.FormatText
opts.ShowValues = true
opts.ShowTimestamps = true
opts.PrintMetadata = true  // Enable metadata display
opts.IndentSize = 2

h.PrintKey(os.Stdout, "Software\\MyApp", opts)
```

**Output:**
```
[MyApp]
  Last Write: 2024-01-15 10:30:45
  Subkeys: 2, Values: 5
  "Version" [REG_SZ] = "1.0.0"
  "InstallDate" [REG_SZ] = "2024-01-15"
  "Timeout" [REG_DWORD] = 0x0000001E (30)
  "MaxSize" [REG_QWORD] = 0x0000000100000000 (4294967296)
  "Config" [REG_BINARY] = 0102030405060708
```

### JSON Format

Structured JSON output suitable for programmatic processing.

```go
opts := printer.DefaultOptions()
opts.Format = printer.FormatJSON
opts.ShowValues = true

h.PrintKey(os.Stdout, "Software\\MyApp", opts)
```

**Output:**
```json
{
  "name": "MyApp",
  "subkeys": 2,
  "values": 5,
  "value_data": {
    "Version": {
      "name": "Version",
      "type": "REG_SZ",
      "data": "1.0.0"
    },
    "Timeout": {
      "name": "Timeout",
      "type": "REG_DWORD",
      "data": 30
    },
    "Config": {
      "name": "Config",
      "type": "REG_BINARY",
      "data": "0102030405060708"
    }
  }
}
```

### Windows .reg File Format

Standard Windows Registry Editor format for import/export.

```go
opts := printer.DefaultOptions()
opts.Format = printer.FormatReg
opts.ShowValues = true

h.PrintKey(os.Stdout, "Software\\MyApp", opts)
```

**Output:**
```
Windows Registry Editor Version 5.00

[\]

[\Software\MyApp]
"Version"=hex(1):31,00,2e,00,30,00,2e,00,30,00,00,00
"Timeout"=dword:0000001e
"MaxSize"=hex(b):00,00,00,00,01,00,00,00
"Config"=hex:01,01,02,03,04,05,06,07,08
```

## Common Use Cases

### Export Entire Subtree to .reg File

```go
h, _ := hive.Open("/path/to/SOFTWARE")
defer h.Close()

// Create output file
f, _ := os.Create("export.reg")
defer f.Close()

// Configure for full export
opts := printer.DefaultOptions()
opts.Format = printer.FormatReg
opts.ShowValues = true
opts.MaxDepth = 0 // unlimited depth

// Export
h.PrintTree(f, "Microsoft\\Windows\\CurrentVersion\\Uninstall", opts)
```

This creates a .reg file containing all installed applications that can be imported into another Windows system.

### Generate JSON Report

```go
h, _ := hive.Open("/path/to/SYSTEM")
defer h.Close()

var buf bytes.Buffer

opts := printer.DefaultOptions()
opts.Format = printer.FormatJSON
opts.ShowValues = true
opts.MaxDepth = 3

h.PrintTree(&buf, "ControlSet001\\Services", opts)

// Parse JSON for analysis
var data map[string]interface{}
json.Unmarshal(buf.Bytes(), &data)
```

### Print Single Value

```go
h, _ := hive.Open("/path/to/SOFTWARE")
defer h.Close()

opts := printer.DefaultOptions()
opts.ShowValueTypes = true

h.PrintValue(os.Stdout, "Microsoft\\Windows NT\\CurrentVersion", "ProductName", opts)
```

**Output:**
```
"ProductName" [REG_SZ] = "Windows 10 Pro"
```

### List All Values in a Key

```go
h, _ := hive.Open("/path/to/SOFTWARE")
defer h.Close()

opts := printer.DefaultOptions()
opts.ShowValues = true
opts.ShowValueTypes = false // Just show data
opts.PrintMetadata = true   // Show metadata counts

h.PrintKey(os.Stdout, "Microsoft\\Windows\\CurrentVersion", opts)
```

**Output:**
```
[CurrentVersion]
  Subkeys: 15, Values: 8
  "ProgramFilesDir" = "C:\Program Files"
  "CommonFilesDir" = "C:\Program Files\Common Files"
  "DevicePath" = "%SystemRoot%\inf"
  "MediaPathUnexpanded" = "%SystemRoot%\Media"
```

### Recursive Tree with Depth Limit

```go
h, _ := hive.Open("/path/to/SOFTWARE")
defer h.Close()

opts := printer.DefaultOptions()
opts.ShowValues = true
opts.PrintMetadata = true  // Show metadata counts
opts.MaxDepth = 2          // Only go 2 levels deep

h.PrintTree(os.Stdout, "Microsoft", opts)
```

**Output:**
```
[Microsoft]
  Subkeys: 5, Values: 0

[Windows]
  Subkeys: 12, Values: 1
  "AppInit_DLLs" [REG_SZ] = ""

[.NET Framework]
  Subkeys: 8, Values: 0
```

### Export with Timestamps for Forensics

```go
h, _ := hive.Open("/path/to/NTUSER.DAT")
defer h.Close()

opts := printer.DefaultOptions()
opts.Format = printer.FormatText
opts.ShowValues = true
opts.ShowTimestamps = true
opts.PrintMetadata = true  // Show metadata counts
opts.MaxDepth = 3

f, _ := os.Create("forensic_dump.txt")
defer f.Close()

h.PrintTree(f, "Software\\Microsoft\\Windows\\CurrentVersion\\Run", opts)
```

**Output:**
```
[Run]
  Last Write: 2024-01-15 14:22:30
  Subkeys: 0, Values: 3
  "OneDrive" [REG_SZ] = "C:\Users\...\OneDrive.exe /background"
  "SecurityHealth" [REG_EXPAND_SZ] = "%ProgramFiles%\...\SecurityHealth.exe"
```

## Configuration Options

```go
type Options struct {
    // Format specifies output format
    // Default: FormatText
    // Values: FormatText, FormatJSON, FormatReg
    Format Format

    // IndentSize is the number of spaces per indent level (text format only)
    // Default: 2
    IndentSize int

    // MaxDepth limits recursion depth (0 = unlimited)
    // Default: 0 (unlimited)
    // Use this to prevent deeply nested trees from consuming too much memory
    MaxDepth int

    // ShowValues includes value data in output
    // Default: true
    // Set to false to only show key structure
    ShowValues bool

    // ShowTimestamps includes last-write times
    // Default: false
    // Useful for forensics and change tracking
    ShowTimestamps bool

    // ShowValueTypes includes REG_* type names
    // Default: true
    // Set to false for cleaner output when types aren't important
    ShowValueTypes bool

    // Recursive enables recursive printing of subkeys
    // Default: false
    // Note: PrintTree() always recurses regardless of this setting
    Recursive bool

    // MaxValueBytes limits how many bytes of binary values to display
    // Longer values are truncated with a note
    // Default: 32
    // Set to 0 for no limit
    MaxValueBytes int

    // PrintMetadata includes metadata (subkey/value counts, timestamps, etc)
    // When false, shows keys/values without metadata counts (clean tree output)
    // When true, shows full metadata including counts (dump/ls output)
    // Default: false
    PrintMetadata bool
}
```

### Getting Default Options

```go
opts := printer.DefaultOptions()
// Equivalent to:
// opts := printer.Options{
//     Format:         FormatText,
//     IndentSize:     2,
//     MaxDepth:       0,
//     ShowValues:     true,
//     ShowTimestamps: false,
//     ShowValueTypes: true,
//     Recursive:      false,
//     MaxValueBytes:  32,
//     PrintMetadata:  false,
// }
```

## API Reference

### Hive Handle Methods (Ergonomic API)

These are the simplest methods to use - they handle Reader creation automatically.

```go
func (h *Hive) PrintKey(w io.Writer, path string, opts Options) error
```

Prints a single key and optionally its values.

**Parameters:**
- `w` - Output writer (os.Stdout, file, buffer, etc.)
- `path` - Registry path (case-insensitive, backslash or forward slash)
- `opts` - Formatting options

**Example:**
```go
h.PrintKey(os.Stdout, "Software\\Microsoft\\Windows\\CurrentVersion", opts)
```

---

```go
func (h *Hive) PrintValue(w io.Writer, keyPath, valueName string, opts Options) error
```

Prints a single value with automatic type decoding.

**Parameters:**
- `w` - Output writer
- `keyPath` - Path to the key containing the value
- `valueName` - Name of the value (use "" for default value)
- `opts` - Formatting options

**Example:**
```go
h.PrintValue(os.Stdout, "Microsoft\\Windows NT\\CurrentVersion", "ProductName", opts)
```

---

```go
func (h *Hive) PrintTree(w io.Writer, path string, opts Options) error
```

Recursively prints an entire subtree starting at the given path.

**Parameters:**
- `w` - Output writer
- `path` - Starting path (use "" for root)
- `opts` - Formatting options (respects MaxDepth)

**Example:**
```go
h.PrintTree(os.Stdout, "Software\\Microsoft", opts)
```

### Printer Methods (Advanced API)

Use these when you already have a Reader or need more control.

```go
func New(r types.Reader, w io.Writer, opts Options) *Printer
```

Creates a new Printer instance.

**Example:**
```go
r, _ := h.Reader()
p := printer.New(r, os.Stdout, opts)
```

---

```go
func (p *Printer) PrintKey(path string) error
func (p *Printer) PrintValue(keyPath, valueName string) error
func (p *Printer) PrintTree(path string) error
```

Same functionality as Hive handle methods but use the Printer's Reader and Writer.

## Value Type Handling

The printer automatically decodes all Windows Registry value types:

| Type | Constant | Output Format |
|------|----------|---------------|
| REG_SZ | `types.REG_SZ` | Decoded UTF-8 string in quotes |
| REG_EXPAND_SZ | `types.REG_EXPAND_SZ` | Decoded UTF-8 string (variables not expanded) |
| REG_DWORD | `types.REG_DWORD` | Hex and decimal: `0x0000001E (30)` |
| REG_DWORD_BE | `types.REG_DWORD_BE` | Big-endian hex and decimal |
| REG_QWORD | `types.REG_QWORD` | 64-bit hex and decimal |
| REG_MULTI_SZ | `types.REG_MULTI_SZ` | Array of strings |
| REG_BINARY | `types.REG_BINARY` | Hex bytes |
| REG_NONE | `types.REG_NONE` | Hex bytes or `<empty>` |
| Others | - | `<N bytes>` where N is the size |

### REG_MULTI_SZ Output

**Text Format:**
```
"SearchPath" [REG_MULTI_SZ] = [
  "C:\Windows\system32"
  "C:\Windows"
  "C:\Windows\System32\Wbem"
]
```

**JSON Format:**
```json
{
  "name": "SearchPath",
  "type": "REG_MULTI_SZ",
  "data": [
    "C:\\Windows\\system32",
    "C:\\Windows",
    "C:\\Windows\\System32\\Wbem"
  ]
}
```

**.reg Format:**
```
"SearchPath"=hex(7):43,00,3a,00,5c,00,57,00,69,00,6e,00,64,00,6f,00,77,00,73,00,5c,00,73,00,79,00,73,00,74,00,65,00,6d,00,33,00,32,00,00,00,43,00,3a,00,5c,00,57,00,69,00,6e,00,64,00,6f,00,77,00,73,00,00,00,43,00,3a,00,5c,00,57,00,69,00,6e,00,64,00,6f,00,77,00,73,00,5c,00,53,00,79,00,73,00,74,00,65,00,6d,00,33,00,32,00,5c,00,57,00,62,00,65,00,6d,00,00,00,00,00
```

## Performance Notes

- **Zero-copy** - The printer accesses memory-mapped hive data directly with no intermediate copies
- **Streaming** - Output is written incrementally, not buffered in memory
- **Large hives** - Use `MaxDepth` to limit recursion when printing large subtrees
- **Binary truncation** - Use `MaxValueBytes` to prevent huge binary blobs from consuming memory

**Example for large hive:**
```go
opts := printer.DefaultOptions()
opts.MaxDepth = 5        // Stop at 5 levels deep
opts.MaxValueBytes = 64  // Truncate binary values over 64 bytes

h.PrintTree(os.Stdout, "", opts)
```

## Error Handling

All printer methods return errors for invalid operations:

```go
h, err := hive.Open("/path/to/SOFTWARE")
if err != nil {
    return fmt.Errorf("open hive: %w", err)
}
defer h.Close()

opts := printer.DefaultOptions()

// Error if path doesn't exist
err = h.PrintKey(os.Stdout, "NonExistent\\Path", opts)
if err != nil {
    return fmt.Errorf("print key: %w", err)
}

// Error if value doesn't exist
err = h.PrintValue(os.Stdout, "Software\\MyApp", "MissingValue", opts)
if err != nil {
    return fmt.Errorf("print value: %w", err)
}
```

## Integration Examples

### Web API Endpoint

```go
func registryHandler(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Query().Get("path")

    h, err := hive.Open("/var/registry/SOFTWARE")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer h.Close()

    opts := printer.DefaultOptions()
    opts.Format = printer.FormatJSON
    opts.ShowValues = true
    opts.MaxDepth = 3

    w.Header().Set("Content-Type", "application/json")
    h.PrintKey(w, path, opts)
}
```

### CLI Tool

```go
func main() {
    hivePath := flag.String("hive", "", "Path to hive file")
    regPath := flag.String("path", "", "Registry key path")
    format := flag.String("format", "text", "Output format (text, json, reg)")
    depth := flag.Int("depth", 0, "Maximum depth (0 = unlimited)")

    flag.Parse()

    h, err := hive.Open(*hivePath)
    if err != nil {
        log.Fatal(err)
    }
    defer h.Close()

    opts := printer.DefaultOptions()
    switch *format {
    case "json":
        opts.Format = printer.FormatJSON
    case "reg":
        opts.Format = printer.FormatReg
    default:
        opts.Format = printer.FormatText
    }
    opts.ShowValues = true
    opts.MaxDepth = *depth

    if err := h.PrintTree(os.Stdout, *regPath, opts); err != nil {
        log.Fatal(err)
    }
}
```

**Usage:**
```bash
$ ./regtool -hive SOFTWARE -path "Microsoft\Windows\CurrentVersion" -format json -depth 2
```

### Forensic Analysis

```go
func analyzeUserActivity(ntUserPath string) error {
    h, err := hive.Open(ntUserPath)
    if err != nil {
        return err
    }
    defer h.Close()

    // Create forensic report
    f, err := os.Create("user_activity_report.txt")
    if err != nil {
        return err
    }
    defer f.Close()

    opts := printer.DefaultOptions()
    opts.ShowValues = true
    opts.ShowTimestamps = true  // Critical for forensics
    opts.MaxDepth = 3

    // Print various activity indicators
    paths := []string{
        "Software\\Microsoft\\Windows\\CurrentVersion\\Run",
        "Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\RecentDocs",
        "Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\TypedPaths",
        "Software\\Microsoft\\Internet Explorer\\TypedURLs",
    }

    for _, path := range paths {
        fmt.Fprintf(f, "\n=== %s ===\n\n", path)
        h.PrintTree(f, path, opts)
    }

    return nil
}
```

## Comparison with Other Tools

### vs. hivexget/hivexexport

**Advantages:**
- No C dependencies - pure Go
- Zero-copy performance
- Three output formats in one API
- Fine-grained control over output
- Type-safe Go API

**Use printer when:**
- You need JSON output
- You want to integrate with Go applications
- You need programmatic control
- You're building cross-platform tools

### vs. reg.exe export

**Advantages:**
- Works on non-Windows systems
- Can process offline hive files
- Programmatic access in Go
- Additional formats (JSON, custom text)

**Use printer when:**
- Analyzing hive files from disk images
- Building forensic tools
- Running on Linux/macOS
- Need structured data (JSON)

## Thread Safety

Printer instances are **NOT** thread-safe. Use one printer per goroutine:

```go
// Safe: Each goroutine has its own printer
for _, hivePath := range hivePaths {
    go func(path string) {
        h, _ := hive.Open(path)
        defer h.Close()

        opts := printer.DefaultOptions()
        h.PrintTree(os.Stdout, "", opts)
    }(hivePath)
}

// Unsafe: Shared printer across goroutines
p := printer.New(reader, os.Stdout, opts)
go func() { p.PrintKey("Path1") }()  // RACE!
go func() { p.PrintKey("Path2") }()  // RACE!
```

## Troubleshooting

### Binary Values Show as Truncated

By default, binary values over 32 bytes are truncated:

```
"LargeData" [REG_BINARY] = 0102030405...1F20 (truncated, 1024 total bytes)
```

**Solution:** Increase `MaxValueBytes`:
```go
opts.MaxValueBytes = 0  // No limit
```

### Tree Printing is Slow

Large subtrees can take time to traverse and print.

**Solution:** Limit depth:
```go
opts.MaxDepth = 5  // Only go 5 levels deep
```

### Out of Memory with Large Trees

Printing deeply nested trees to memory (bytes.Buffer) can consume lots of RAM.

**Solution:** Write directly to a file:
```go
f, _ := os.Create("output.txt")
defer f.Close()
h.PrintTree(f, "", opts)  // Streams to file
```

### JSON Output is Not Pretty

**Solution:** The printer uses `json.MarshalIndent` with 2-space indentation. For custom formatting, parse and re-marshal:
```go
var buf bytes.Buffer
h.PrintTree(&buf, path, opts)

var data map[string]interface{}
json.Unmarshal(buf.Bytes(), &data)

// Re-marshal with custom format
pretty, _ := json.MarshalIndent(data, "", "    ")
fmt.Println(string(pretty))
```

## License

Part of the hivekit project. See LICENSE file in repository root.
