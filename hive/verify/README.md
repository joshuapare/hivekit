# verify Package

The `verify` package provides validation functions for Windows Registry hive structures. These helpers are designed to be used in tests to ensure hive invariants are maintained throughout operations.

## Overview

This package validates the core invariants required for valid Windows Registry hive files:

- **REGF Header** - Structure and field validation
- **HBIN Blocks** - Structure, contiguity, and cell validation
- **File Size** - Consistency between header and actual file size
- **Sequence Numbers** - Transaction consistency (Seq1 == Seq2 for clean hives)
- **Checksum** - XOR checksum of header fields

## Quick Start

### Validate All Invariants

```go
import "github.com/joshuapare/hivekit/hive/verify"

func TestMyHiveOperation(t *testing.T) {
    h, _ := hive.Open("test.hiv")

    // Perform some operations...

    // Validate all invariants
    err := verify.AllInvariants(h.Bytes())
    require.NoError(t, err, "Hive should maintain all invariants")
}
```

### Validate Individual Aspects

```go
// Validate just the REGF header
err := verify.REGFHeader(data)

// Validate HBIN structure
err := verify.HBINStructure(data)

// Validate file size matches header
err := verify.FileSize(data)

// Check sequence numbers are consistent (clean hive)
err := verify.SequenceNumbers(data)

// Validate checksum
err := verify.Checksum(data)
```

## Validation Functions

### `AllInvariants(data []byte) error`

Validates all hive invariants in one call. Returns the first error encountered, or `nil` if all checks pass.

**Checks performed:**
1. REGF header structure
2. HBIN structure and contiguity
3. File size consistency

**Example:**
```go
err := verify.AllInvariants(h.Bytes())
if err != nil {
    t.Errorf("Hive validation failed: %v", err)
}
```

### `REGFHeader(data []byte) error`

Validates the REGF header structure and fields.

**Checks:**
- File is large enough (≥4096 bytes)
- Signature is "regf"
- Major version is 1
- Minor version is in valid range (3-6)
- Data size is 4KB-aligned

**Example:**
```go
err := verify.REGFHeader(data)
require.NoError(t, err, "REGF header should be valid")
```

### `HBINStructure(data []byte) error`

Validates all HBIN blocks are valid and contiguous.

**Checks:**
- Each HBIN has valid signature "hbin"
- HBIN offset fields match actual positions
- HBIN sizes are valid and 4KB-aligned
- HBINs don't extend beyond file
- Cells within HBINs don't cross boundaries
- Cell sizes are 8-byte aligned

**Example:**
```go
err := verify.HBINStructure(data)
require.NoError(t, err, "HBIN structure should be valid")
```

### `FileSize(data []byte) error`

Validates that file size matches the header's data size field.

**Formula:** `file_size == REGF_header (4096) + data_size`

**Example:**
```go
err := verify.FileSize(data)
require.NoError(t, err, "File size should match header")
```

### `SequenceNumbers(data []byte) error`

Checks that sequence numbers are consistent (Seq1 == Seq2).

**Note:** When Seq1 != Seq2, the hive is considered "dirty" and may need recovery. This is not necessarily an error in all contexts, but indicates the hive was in the middle of a transaction.

**Example:**
```go
err := verify.SequenceNumbers(data)
if err != nil {
    t.Logf("Hive is dirty: %v", err)
}
```

### `Checksum(data []byte) error`

Validates the REGF header checksum.

The checksum is computed as the XOR of all 508 dwords (first 0x1FC bytes) before the checksum field at offset 0x1FC.

**Example:**
```go
err := verify.Checksum(data)
require.NoError(t, err, "Checksum should be valid")
```

## Error Handling

All validation functions return a `*ValidationError` on failure:

```go
type ValidationError struct {
    Type    string                 // Type of validation that failed
    Message string                 // Human-readable error message
    Offset  int                    // Byte offset where error occurred (-1 if N/A)
    Details map[string]interface{} // Additional context
}
```

**Example error handling:**
```go
err := verify.AllInvariants(data)
if err != nil {
    if verr, ok := err.(*verify.ValidationError); ok {
        t.Logf("Validation failed: %s at offset 0x%X", verr.Type, verr.Offset)
        t.Logf("Details: %v", verr.Details)
    }
    t.Fail()
}
```

## Testing Patterns

### Pattern 1: Post-Operation Validation

Validate hive after performing operations:

```go
func TestGrowMaintainsInvariants(t *testing.T) {
    h, _ := hive.Open("test.hiv")
    fa, _ := alloc.NewFast(h, nil)

    // Perform operation
    err := fa.Grow(4096)
    require.NoError(t, err)

    // Validate invariants maintained
    err = verify.AllInvariants(h.Bytes())
    require.NoError(t, err, "Grow() should maintain hive invariants")
}
```

### Pattern 2: Corruption Detection

Test that corrupted hives are properly detected:

```go
func TestDetectCorruption(t *testing.T) {
    data := createValidHive(t)

    // Intentionally corrupt
    copy(data[0:4], []byte("XXXX"))

    // Should detect corruption
    err := verify.REGFHeader(data)
    require.Error(t, err)
    require.Contains(t, err.Error(), "invalid signature")
}
```

### Pattern 3: Loop Validation

Validate after each iteration in a loop:

```go
func TestRepeatedOperations(t *testing.T) {
    h, _ := hive.Open("test.hiv")
    fa, _ := alloc.NewFast(h, nil)

    for i := 0; i < 100; i++ {
        ref, _, err := fa.Alloc(256, alloc.ClassNK)
        require.NoError(t, err)

        err = fa.Free(ref)
        require.NoError(t, err)

        // Validate after each cycle
        err = verify.AllInvariants(h.Bytes())
        require.NoError(t, err, "Iteration %d failed validation", i)
    }
}
```

## Integration with Existing Tests

The verify package can be easily integrated into existing tests by adding validation calls after operations. See Phase 6 of the TDD plan for systematic integration.

## Performance Considerations

- `AllInvariants()` performs a full scan of the hive and may be slow for large files
- For performance-critical tests, consider validating specific aspects only
- Use `testing.Short()` to skip validation in short test mode:

```go
if !testing.Short() {
    err := verify.AllInvariants(h.Bytes())
    require.NoError(t, err)
}
```

## See Also

- [REGF Specification](https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md)
- [Google Project Zero REGF Analysis](https://googleprojectzero.blogspot.com/2024/12/the-windows-registry-adventure-5-regf.html)
