# Hivex Validation Test Utility - Complete API Reference

## Overview

The `hivexval` package provides a unified, comprehensive test utility for validating Windows Registry hive files using multiple backends (Go bindings, gohivex reader, hivexsh CLI).

## Design Principles

1. **Dual Backend Support** - Use either Go bindings (fast) or hivexsh (authoritative)
2. **Handle Abstraction** - Methods accept `interface{}` handles that work with both backends
3. **Rich Assertions** - Testing-focused API with detailed failure messages
4. **Cross-Validation** - Compare multiple implementations to catch bugs
5. **Zero External Dependencies** - Graceful degradation if hivexsh unavailable

---

## Core Types

### Validator

Main entry point for all validation operations.

```go
type Validator struct {
    // Unexported fields
}
```

### Options

```go
type Options struct {
    UseBindings              bool  // Use Go bindings (default: true)
    UseHivexsh               bool  // Use hivexsh CLI (default: false)
    UseReader                bool  // Use gohivex reader (default: false)
    CompareAll               bool  // Compare all enabled backends (default: false)
    SkipIfHivexshUnavailable bool  // Skip vs fail if hivexsh missing (default: true)
}
```

### ValidationResult

```go
type ValidationResult struct {
    StructureValid   bool
    HivexshPassed    bool
    KeyCount         int
    ValueCount       int
    Errors           []string
    Warnings         []string
    ComparisonResult *ComparisonResult
}
```

### ComparisonResult

```go
type ComparisonResult struct {
    Match          bool
    Mismatches     []Mismatch
    NodesCompared  int
    ValuesCompared int
}
```

### Mismatch

```go
type Mismatch struct {
    Category string       // "key_count", "value_type", "value_data", etc.
    Path     string       // Registry path
    Message  string       // Human-readable description
    Expected interface{} // Expected value
    Actual   interface{} // Actual value
}
```

---

## Constructor Functions

### New

Creates a validator for a hive file.

```go
func New(path string, opts *Options) (*Validator, error)
```

**Parameters:**
- `path` - Absolute path to hive file
- `opts` - Options (nil uses DefaultOptions)

**Returns:**
- Validator instance
- Error if file doesn't exist or can't be opened

**Example:**
```go
v, err := hivexval.New("/path/to/hive", nil)
if err != nil {
    t.Fatal(err)
}
defer v.Close()
```

### NewBytes

Creates a validator from a byte buffer.

```go
func NewBytes(data []byte, opts *Options) (*Validator, error)
```

**Parameters:**
- `data` - Hive file contents
- `opts` - Options (nil uses DefaultOptions)

**Returns:**
- Validator instance
- Error if data is invalid

**Example:**
```go
data, _ := os.ReadFile("test.hive")
v, err := hivexval.NewBytes(data, nil)
```

### Must

Panics on error (for tests where failure is fatal).

```go
func Must(v *Validator, err error) *Validator
```

**Example:**
```go
v := hivexval.Must(hivexval.New(path, nil))
defer v.Close()
```

### DefaultOptions

Returns recommended default options.

```go
func DefaultOptions() *Options
```

**Returns:**
```go
&Options{
    UseBindings:              true,
    UseHivexsh:               false,
    UseReader:                false,
    CompareAll:               false,
    SkipIfHivexshUnavailable: true,
}
```

---

## Lifecycle Methods

### Close

Releases all resources.

```go
func (v *Validator) Close() error
```

**Example:**
```go
v, _ := hivexval.New(path, nil)
defer v.Close()
```

---

## High-Level Validation

### Validate

Performs comprehensive validation using all enabled backends.

```go
func (v *Validator) Validate() (*ValidationResult, error)
```

**Returns:**
- ValidationResult with all checks
- Error if validation fails

**Example:**
```go
result, err := v.Validate()
if err != nil {
    t.Fatal(err)
}
if !result.StructureValid {
    t.Errorf("Structure invalid: %v", result.Errors)
}
```

### ValidateStructure

Checks if hive can be opened and parsed.

```go
func (v *Validator) ValidateStructure() error
```

**Returns:**
- nil if structure is valid
- Error describing structural issue

### ValidateWithHivexsh

Runs `hivexsh -d` on the hive.

```go
func (v *Validator) ValidateWithHivexsh() error
```

**Returns:**
- nil if hivexsh succeeds
- Error with hivexsh output if fails

**Example:**
```go
if err := v.ValidateWithHivexsh(); err != nil {
    t.Errorf("Hivexsh validation failed: %v", err)
}
```

---

## Navigation & Counting

### Root

Returns the root node handle.

```go
func (v *Validator) Root() (interface{}, error)
```

**Returns:**
- Handle (bindings.NodeHandle or types.NodeID)
- Error if root cannot be accessed

### CountKeys

Recursively counts all keys in the hive.

```go
func (v *Validator) CountKeys() (int, error)
```

**Returns:**
- Total number of keys (including root)
- Error if traversal fails

**Example:**
```go
count, err := v.CountKeys()
require.NoError(t, err)
require.Equal(t, 42, count)
```

### CountValues

Recursively counts all values in the hive.

```go
func (v *Validator) CountValues() (int, error)
```

**Returns:**
- Total number of values across all keys
- Error if traversal fails

### CountTree

Returns both key and value counts.

```go
func (v *Validator) CountTree() (keyCount int, valueCount int, err error)
```

**Example:**
```go
keys, values, err := v.CountTree()
require.NoError(t, err)
t.Logf("Hive has %d keys, %d values", keys, values)
```

### WalkTree

Performs recursive traversal with a callback.

```go
func (v *Validator) WalkTree(fn func(path string, depth int, isValue bool) error) error
```

**Parameters:**
- `fn` - Callback invoked for each node/value
  - `path` - Full registry path (e.g., "Software\\MyApp")
  - `depth` - Tree depth (root = 0)
  - `isValue` - true for values, false for keys

**Returns:**
- Error if callback returns error or traversal fails

**Example:**
```go
err := v.WalkTree(func(path string, depth int, isValue bool) error {
    if isValue {
        t.Logf("[VALUE] %s", path)
    } else {
        t.Logf("[KEY] %s (depth: %d)", path, depth)
    }
    return nil
})
```

---

## Key Operations

### GetKey

Finds a key by path.

```go
func (v *Validator) GetKey(path []string) (interface{}, error)
```

**Parameters:**
- `path` - Registry path (e.g., []string{"Software", "MyApp"})

**Returns:**
- Key handle
- Error if key doesn't exist

**Example:**
```go
key, err := v.GetKey([]string{"Software", "MyApp"})
if err != nil {
    t.Fatal("Key not found")
}
```

### GetKeyName

Returns the name of a key.

```go
func (v *Validator) GetKeyName(key interface{}) (string, error)
```

**Parameters:**
- `key` - Key handle from Root() or GetKey()

**Returns:**
- Key name (empty for root)
- Error if invalid handle

### GetSubkeys

Lists all child keys.

```go
func (v *Validator) GetSubkeys(key interface{}) ([]interface{}, error)
```

**Returns:**
- Slice of child key handles
- Error if enumeration fails

**Example:**
```go
children, err := v.GetSubkeys(key)
for _, child := range children {
    name, _ := v.GetKeyName(child)
    t.Logf("Child: %s", name)
}
```

### GetSubkeyCount

Returns number of child keys.

```go
func (v *Validator) GetSubkeyCount(key interface{}) (int, error)
```

**Returns:**
- Count of direct children
- Error if invalid handle

### GetParent

Returns the parent key.

```go
func (v *Validator) GetParent(key interface{}) (interface{}, error)
```

**Returns:**
- Parent key handle
- Error if key is root (has no parent)

### GetKeyTimestamp

Returns last write time.

```go
func (v *Validator) GetKeyTimestamp(key interface{}) (time.Time, error)
```

**Returns:**
- Last modification time
- Error if unavailable

---

## Value Operations

### GetValues

Lists all values in a key.

```go
func (v *Validator) GetValues(key interface{}) ([]interface{}, error)
```

**Returns:**
- Slice of value handles
- Error if enumeration fails

**Example:**
```go
values, err := v.GetValues(key)
for _, val := range values {
    name, _ := v.GetValueName(val)
    t.Logf("Value: %s", name)
}
```

### GetValue

Finds a value by name (case-insensitive).

```go
func (v *Validator) GetValue(key interface{}, name string) (interface{}, error)
```

**Parameters:**
- `key` - Key handle
- `name` - Value name (case-insensitive)

**Returns:**
- Value handle
- Error if value doesn't exist

**Example:**
```go
val, err := v.GetValue(key, "Version")
if err != nil {
    t.Fatal("Value not found")
}
```

### GetValueCount

Returns number of values in a key.

```go
func (v *Validator) GetValueCount(key interface{}) (int, error)
```

### GetValueName

Returns value name.

```go
func (v *Validator) GetValueName(val interface{}) (string, error)
```

### GetValueType

Returns value type as string.

```go
func (v *Validator) GetValueType(val interface{}) (string, error)
```

**Returns:**
- Type string: "REG_SZ", "REG_DWORD", "REG_BINARY", etc.
- Error if invalid handle

**Example:**
```go
typ, _ := v.GetValueType(val)
require.Equal(t, "REG_SZ", typ)
```

### GetValueData

Returns raw value bytes.

```go
func (v *Validator) GetValueData(val interface{}) ([]byte, error)
```

**Returns:**
- Raw bytes (encoding depends on type)
- Error if read fails

### GetValueString

Returns value as string (REG_SZ/REG_EXPAND_SZ).

```go
func (v *Validator) GetValueString(val interface{}) (string, error)
```

**Returns:**
- Decoded string
- Error if type is not REG_SZ/REG_EXPAND_SZ

**Example:**
```go
str, err := v.GetValueString(val)
require.NoError(t, err)
require.Equal(t, "1.0.0", str)
```

### GetValueDWORD

Returns value as uint32 (REG_DWORD).

```go
func (v *Validator) GetValueDWORD(val interface{}) (uint32, error)
```

**Returns:**
- 32-bit integer
- Error if type is not REG_DWORD

### GetValueQWORD

Returns value as uint64 (REG_QWORD).

```go
func (v *Validator) GetValueQWORD(val interface{}) (uint64, error)
```

### GetValueStrings

Returns value as string slice (REG_MULTI_SZ).

```go
func (v *Validator) GetValueStrings(val interface{}) ([]string, error)
```

**Returns:**
- String array
- Error if type is not REG_MULTI_SZ

**Example:**
```go
strs, err := v.GetValueStrings(val)
require.NoError(t, err)
require.Equal(t, []string{"A", "B", "C"}, strs)
```

---

## Testing Assertions

All assertion methods take `*testing.T` and fail the test on mismatch.

### AssertKeyExists

Checks a key exists at the given path.

```go
func (v *Validator) AssertKeyExists(t *testing.T, path []string)
```

**Example:**
```go
v.AssertKeyExists(t, []string{"Software", "MyApp"})
```

### AssertKeyNotExists

Checks a key does NOT exist at the given path.

```go
func (v *Validator) AssertKeyNotExists(t *testing.T, path []string)
```

### AssertValueExists

Checks a value exists in a key.

```go
func (v *Validator) AssertValueExists(t *testing.T, keyPath []string, valueName string)
```

**Example:**
```go
v.AssertValueExists(t, []string{"Software", "MyApp"}, "Version")
```

### AssertValueNotExists

Checks a value does NOT exist.

```go
func (v *Validator) AssertValueNotExists(t *testing.T, keyPath []string, valueName string)
```

### AssertKeyCount

Checks total key count matches expected.

```go
func (v *Validator) AssertKeyCount(t *testing.T, expected int)
```

**Example:**
```go
v.AssertKeyCount(t, 42)
```

### AssertValueCount

Checks total value count matches expected.

```go
func (v *Validator) AssertValueCount(t *testing.T, expected int)
```

### AssertSubkeyCount

Checks a key has expected number of children.

```go
func (v *Validator) AssertSubkeyCount(t *testing.T, keyPath []string, expected int)
```

### AssertValueType

Checks value has expected type.

```go
func (v *Validator) AssertValueType(t *testing.T, keyPath []string, valueName string, expectedType string)
```

**Parameters:**
- `expectedType` - "REG_SZ", "REG_DWORD", "REG_BINARY", etc.

**Example:**
```go
v.AssertValueType(t, []string{"Software", "MyApp"}, "Timeout", "REG_DWORD")
```

### AssertValueData

Checks value has expected raw bytes.

```go
func (v *Validator) AssertValueData(t *testing.T, keyPath []string, valueName string, expected []byte)
```

### AssertValueString

Checks string value matches expected.

```go
func (v *Validator) AssertValueString(t *testing.T, keyPath []string, valueName string, expected string)
```

**Example:**
```go
v.AssertValueString(t, []string{"Software", "MyApp"}, "Version", "1.0.0")
```

### AssertValueDWORD

Checks DWORD value matches expected.

```go
func (v *Validator) AssertValueDWORD(t *testing.T, keyPath []string, valueName string, expected uint32)
```

**Example:**
```go
v.AssertValueDWORD(t, []string{"Software", "MyApp"}, "Timeout", 30)
```

### AssertValueQWORD

Checks QWORD value matches expected.

```go
func (v *Validator) AssertValueQWORD(t *testing.T, keyPath []string, valueName string, expected uint64)
```

### AssertValueStrings

Checks MULTI_SZ value matches expected.

```go
func (v *Validator) AssertValueStrings(t *testing.T, keyPath []string, valueName string, expected []string)
```

**Example:**
```go
v.AssertValueStrings(t, []string{"Software", "MyApp"}, "Features", []string{"A", "B", "C"})
```

### AssertStructureValid

Checks hive structure is valid.

```go
func (v *Validator) AssertStructureValid(t *testing.T)
```

### AssertHivexshValid

Checks hivexsh validation passes.

```go
func (v *Validator) AssertHivexshValid(t *testing.T)
```

**Example:**
```go
v.AssertHivexshValid(t)  // Fails test if hivexsh -d fails
```

---

## Comparison & Cross-Validation

### Compare

Compares this validator with another implementation.

```go
func (v *Validator) Compare(other *Validator) (*ComparisonResult, error)
```

**Parameters:**
- `other` - Another validator (different backend or same hive opened differently)

**Returns:**
- ComparisonResult with mismatches
- Error if comparison fails

**Example:**
```go
// Compare Go bindings vs hivexsh
v1 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseBindings: true}))
v2 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseReader: true}))

result, err := v1.Compare(v2)
if !result.Match {
    for _, m := range result.Mismatches {
        t.Errorf("[%s] %s: %s", m.Category, m.Path, m.Message)
    }
}
```

### AssertMatchesValidator

Asserts this validator matches another.

```go
func (v *Validator) AssertMatchesValidator(t *testing.T, other *Validator)
```

**Fails test if:**
- Trees have different structures
- Values differ between validators

---

## Utility Functions

### IsHivexshAvailable

Checks if hivexsh command is available.

```go
func IsHivexshAvailable() bool
```

**Example:**
```go
if !hivexval.IsHivexshAvailable() {
    t.Skip("hivexsh not available")
}
```

### Backend

Returns which backend is currently active.

```go
func (v *Validator) Backend() Backend
```

**Returns:** BackendBindings, BackendReader, or BackendHivexsh

---

## Usage Patterns

### Quick Validation

```go
func TestMyHive(t *testing.T) {
    v := hivexval.Must(hivexval.New("test.hive", nil))
    defer v.Close()

    v.AssertKeyExists(t, []string{"Software", "MyApp"})
    v.AssertValueString(t, []string{"Software", "MyApp"}, "Version", "1.0.0")
    v.AssertValueDWORD(t, []string{"Software", "MyApp"}, "Timeout", 30)
}
```

### Comprehensive Validation

```go
func TestMyHive(t *testing.T) {
    v := hivexval.Must(hivexval.New("test.hive", &hivexval.Options{
        UseBindings: true,
        UseHivexsh:  true,
    }))
    defer v.Close()

    result, err := v.Validate()
    require.NoError(t, err)

    require.True(t, result.StructureValid)
    require.True(t, result.HivexshPassed)
    require.Equal(t, 42, result.KeyCount)
    require.Equal(t, 100, result.ValueCount)
}
```

### Cross-Validation

```go
func TestBindingsVsReader(t *testing.T) {
    v1 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseBindings: true}))
    defer v1.Close()

    v2 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseReader: true}))
    defer v2.Close()

    v1.AssertMatchesValidator(t, v2)
}
```

### Tree Walking

```go
func TestWalkTree(t *testing.T) {
    v := hivexval.Must(hivexval.New(path, nil))
    defer v.Close()

    keyCount := 0
    valueCount := 0

    err := v.WalkTree(func(path string, depth int, isValue bool) error {
        if isValue {
            valueCount++
        } else {
            keyCount++
        }
        return nil
    })

    require.NoError(t, err)
    t.Logf("Found %d keys, %d values", keyCount, valueCount)
}
```

---

## Error Handling

All methods return errors with detailed context. No panics except `Must()`.

**Error Categories:**
- File I/O errors (path doesn't exist, permissions)
- Parse errors (invalid hive structure)
- Hivexsh errors (command not found, validation failed)
- Type errors (accessing DWORD as string)
- Not found errors (key/value doesn't exist)

**Example:**
```go
val, err := v.GetValue(key, "NonExistent")
if err != nil {
    // Error message: "value 'NonExistent' not found in key 'Software\\MyApp'"
    t.Log(err)
}
```

---

## Implementation Notes

- Methods accept `interface{}` handles to support multiple backends
- Internal type switching based on active backend
- Case-insensitive value name matching (Windows behavior)
- Graceful degradation if optional backends unavailable
- Thread-safe for read operations
- Not safe for concurrent writes (use one validator per goroutine)
