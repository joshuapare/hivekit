# Hive Builder

High-performance, ergonomic API for building Windows Registry hive files programmatically.

## Features

- ✅ **Path-based API** - Perfect for parsing loops and data import
- ✅ **All registry value types supported** - REG_SZ, REG_DWORD, REG_BINARY, REG_MULTI_SZ, REG_QWORD, and more
- ✅ **Create from scratch or modify existing hives**
- ✅ **Progressive writes** - Constant memory usage for arbitrarily large hives
- ✅ **Type-safe helpers** - Automatic encoding for strings, integers, binary data
- ✅ **.reg file format support** - Parse full path strings with backslashes
- ✅ **ACID semantics** - Transaction safety with crash recovery
- ✅ **High performance** - ~1000 operations/second, O(1) allocation

## Quick Start

```go
import "github.com/joshuapare/hivekit/hive/builder"

// Create a new hive
b, _ := builder.New("app.hive", nil)
defer b.Close()

// Add values using the path-based API
b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
b.SetDWORD([]string{"Software", "MyApp"}, "Timeout", 30)
b.SetBinary([]string{"Software", "MyApp"}, "Config", []byte{0x01, 0x02})

// Commit changes
b.Commit()
```

## Supported Value Types

The builder supports **all** Windows Registry value types:

### Common Types (Type-Safe Helpers)

| Method | Type | Constant | Description |
|--------|------|----------|-------------|
| `SetString()` | REG_SZ (1) | `format.REGSZ` | Null-terminated string (UTF-16LE) |
| `SetExpandString()` | REG_EXPAND_SZ (2) | `format.REGExpandSZ` | String with environment variables |
| `SetBinary()` | REG_BINARY (3) | `format.REGBinary` | Raw binary data |
| `SetDWORD()` | REG_DWORD (4) | `format.REGDWORD` | 32-bit little-endian integer |
| `SetQWORD()` | REG_QWORD (11) | `format.REGQWORD` | 64-bit little-endian integer |
| `SetMultiString()` | REG_MULTI_SZ (7) | `format.REGMultiSZ` | Array of null-terminated strings |

### Advanced Types

| Method | Type | Constant | Description |
|--------|------|----------|-------------|
| `SetNone()` | REG_NONE (0) | `format.REGNone` | No value type (placeholder) |
| `SetDWORDBigEndian()` | REG_DWORD_BIG_ENDIAN (5) | `format.REGDWORDBigEndian` | 32-bit big-endian integer |
| `SetLink()` | REG_LINK (6) | `format.REGLink` | Symbolic link |
| `SetResourceList()` | REG_RESOURCE_LIST (8) | `format.REGResourceList` | Hardware resource list |
| `SetFullResourceDescriptor()` | REG_FULL_RESOURCE_DESCRIPTOR (9) | `format.REGFullResourceDescriptor` | Hardware resource descriptor |
| `SetResourceRequirementsList()` | REG_RESOURCE_REQUIREMENTS_LIST (10) | `format.REGResourceRequirementsList` | Hardware requirements list |

### Generic Setter

```go
// For custom types or when you have pre-encoded data
b.SetValue(path []string, name string, typ uint32, data []byte)
```

## Working with .reg File Format Data

The builder includes helpers for working with data in .reg file format:

### SplitPath - Convert Full Path Strings

```go
// Handles full paths with backslashes
path := builder.SplitPath("HKLM\\Software\\MyApp")
// Returns: []string{"Software", "MyApp"}

// Strips common prefixes automatically
path = builder.SplitPath("HKEY_LOCAL_MACHINE\\Software\\MyApp")
// Returns: []string{"Software", "MyApp"}

// Supported prefixes: HKLM, HKCU, HKCR, HKU, HKCC and full names
```

### ParseValueType - Convert Type Strings

```go
// Parse type string to constant
typ, _ := builder.ParseValueType("REG_SZ")     // Returns: 1
typ, _ = builder.ParseValueType("DWORD")       // Returns: 4
typ, _ = builder.ParseValueType("REG_BINARY")  // Returns: 3

// Case-insensitive
typ, _ = builder.ParseValueType("reg_sz")      // Returns: 1
```

### SetValueFromString - All-in-One Method

```go
// Perfect for parsing loops
b.SetValueFromString(
    "HKLM\\Software\\MyApp",  // Full path with backslashes
    "Version",                 // Value name
    "REG_SZ",                  // Type as string
    data,                      // Raw bytes
)
```

### Complete .reg Parsing Example

```go
// Data from a .reg file
type RegEntry struct {
    FullPath  string // "HKLM\\Software\\MyApp"
    ValueName string // "Version"
    ValueType string // "REG_SZ"
    Data      []byte // Raw bytes
}

entries := []RegEntry{
    {
        FullPath:  "HKLM\\Software\\MyApp",
        ValueName: "Version",
        ValueType: "REG_SZ",
        Data:      builder.EncodeStringHelper("1.0.0"),
    },
    {
        FullPath:  "Software\\MyApp",
        ValueName: "Timeout",
        ValueType: "DWORD",
        Data:      builder.EncodeDWORDHelper(30),
    },
}

b, _ := builder.New("output.hive", nil)
defer b.Close()

// Process each entry
for _, entry := range entries {
    b.SetValueFromString(entry.FullPath, entry.ValueName, entry.ValueType, entry.Data)
}

b.Commit()
```

### Encoding Helpers

When you need to encode data externally (before passing to `SetValueFromString`):

```go
// String encoding
data := builder.EncodeStringHelper("Hello")           // UTF-16LE with null terminator

// Integer encoding
data = builder.EncodeDWORDHelper(12345)               // 4 bytes, little-endian
data = builder.EncodeQWORDHelper(9876543210)          // 8 bytes, little-endian
data = builder.EncodeDWORDBigEndianHelper(12345)      // 4 bytes, big-endian

// Multi-string encoding
data = builder.EncodeMultiStringHelper([]string{"A", "B", "C"})  // REG_MULTI_SZ format
```

## Progressive Writes for Large Hives

For building large hives, configure progressive writes to maintain constant memory usage:

```go
opts := builder.DefaultOptions()
opts.AutoFlushThreshold = 1000    // Flush every 1000 operations
opts.PreallocPages = 100000       // Pre-allocate ~400MB

b, _ := builder.New("large.hive", opts)
defer b.Close()

// Add millions of entries - memory usage stays constant
for i := 0; i < 10_000_000; i++ {
    b.SetDWORD([]string{"Data", fmt.Sprintf("Key%d", i)}, "Value", uint32(i))
}

b.Commit()
```

## Configuration Options

```go
type Options struct {
    // Write strategy (default: StrategyHybrid)
    Strategy StrategyType  // InPlace, Append, or Hybrid

    // Pre-allocate pages (0 = grow dynamically)
    PreallocPages int

    // Progressive flush threshold (default: 1000)
    // Set to 0 to disable progressive writes
    AutoFlushThreshold int

    // Create new hive if doesn't exist (default: true)
    CreateIfNotExists bool

    // Hive version for new hives (default: Version1_3)
    HiveVersion HiveVersion  // Version1_3, Version1_4, Version1_5, Version1_6

    // Flush mode for commits (default: FlushAuto)
    FlushMode dirty.FlushMode  // FlushAuto, FlushDataOnly, FlushFull
}
```

## Examples

See `examples/builder/` directory:
- **simple.go** - Basic usage with various value types
- **large.go** - Building 10,000 keys with progressive writes
- **regfile_format.go** - Parsing .reg file format data

## Performance

- **Throughput**: ~1000 operations/second (single-threaded)
- **Memory**: Constant with progressive writes (doesn't grow with hive size)
- **Scalability**: Tested up to 1GB hives, supports up to 4GB
- **Latency**: ~50-100ns per operation (excluding I/O)

## Implementation Details

The builder is a thin ergonomic wrapper over the proven hivekit merge infrastructure:

- **100% reuse** of existing Session/Plan/Strategy/Allocator/Transaction components
- **Zero duplication** - all core logic already tested in production
- **ACID semantics** - Full transaction safety with crash recovery
- **Platform-optimized** - Uses mremap() on Linux, F_FULLFSYNC on macOS
- **O(1) operations** - Segregated free lists for allocation, hash index for lookups

## Thread Safety

Builder instances are **NOT** thread-safe. Use one builder per goroutine:

```go
// Safe: One builder per goroutine
for _, file := range files {
    go func(f string) {
        b, _ := builder.New(f, nil)
        defer b.Close()
        // ... operations ...
        b.Commit()
    }(file)
}

// Unsafe: Shared builder across goroutines (RACE!)
b, _ := builder.New("shared.hive", nil)
go func() { b.SetString(...) }()  // ❌ RACE!
go func() { b.SetDWORD(...) }()   // ❌ RACE!
```

## Error Handling

All methods return errors for invalid operations:

```go
b, err := builder.New("test.hive", nil)
if err != nil {
    return fmt.Errorf("create builder: %w", err)
}
defer b.Close()

if err := b.SetString([]string{"Software", "MyApp"}, "Version", "1.0"); err != nil {
    return fmt.Errorf("set version: %w", err)
}

if err := b.Commit(); err != nil {
    return fmt.Errorf("commit: %w", err)
}
```

## License

Part of the hivekit project. See LICENSE file in repository root.
