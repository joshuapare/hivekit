# Offset Representation Differences: gohivex vs hivex

## Summary

**gohivex and hivex use different conventions for representing node/value offsets:**

| Implementation | Offset Type | Example | Description |
|----------------|-------------|---------|-------------|
| **gohivex** | HBIN-relative | `32` (0x20) | Offset from start of HBIN data |
| **hivex** | Absolute file | `4128` (0x1020) | Absolute offset in file |

**Conversion:**
```
hivex_offset = gohivex_offset + 0x1000
gohivex_offset = hivex_offset - 0x1000
```

The constant `0x1000` (4096 bytes) is the offset where the first HBIN starts in a registry hive file.

## Why the Difference?

### Windows Registry Hive File Structure

```
[0x0000 - 0x0FFF]  REGF Header (4096 bytes)
                   Contains RootCellOffset as HBIN-relative (e.g., 0x20)

[0x1000 - ...]     First HBIN (Hive Bin)
  [0x1000 - 0x101F]  HBIN Header (32 bytes)
  [0x1020 - ...]     Cell data starts here
                     Root NK cell typically at 0x1020 (file offset)
                     = 0x20 (HBIN-relative offset)
```

### gohivex Approach (HBIN-relative)

- **Uses offsets as stored in the REGF header**
- `NodeID = RootCellOffset = 0x20` (from header)
- Matches the Windows Registry specification
- More compact representation
- Requires adding 0x1000 when seeking in file

**Advantages:**
- ✅ Matches specification exactly
- ✅ Smaller values (uint32 sufficient)
- ✅ Direct mapping to header values

### hivex Approach (Absolute file offset)

- **Converts HBIN-relative to absolute during open**
- `NodeHandle = 0x1000 + 0x20 = 0x1020`
- Direct file offset for seeking
- Requires uint64 for handles

**Advantages:**
- ✅ Can seek directly in file
- ✅ No conversion needed for I/O

## Impact on Testing

### Direct Comparison Fails

```go
// ❌ This will ALWAYS fail:
goRoot, _ := goHive.Root()       // Returns 32
hivexRoot := hivexHive.Root()    // Returns 4128
assert.Equal(t, goRoot, hivexRoot) // 32 != 4128 ❌
```

### Correct Comparison Methods

**Method 1: Convert before comparing**
```go
// ✅ Convert gohivex offset to absolute
const hbinStart = 0x1000
goAbsolute := uint64(goRoot) + hbinStart
assert.Equal(t, goAbsolute, uint64(hivexRoot)) // 4128 == 4128 ✅
```

**Method 2: Semantic comparison**
```go
// ✅ Compare what the offsets point to, not the offsets themselves
goMeta, _ := goHive.StatKey(goRoot)
hivexName := hivexHive.NodeName(hivexRoot)
assert.Equal(t, goMeta.Name, hivexName) // Both return "$$$PROTO.HIV" ✅
```

## Implementation in Test Helpers

The `helpers.go` file provides corrected assertion functions:

```go
// assertSameNodeID - Automatically converts offsets before comparing
func assertSameNodeID(t *testing.T, goNode hive.NodeID, hivexNode bindings.NodeHandle, ...) {
    const hbinStart = 0x1000
    goAbsolute := uint64(goNode) + hbinStart

    if goAbsolute != uint64(hivexNode) {
        t.Errorf("Node IDs don't match...")
    }
}

// assertSameValueID - Same conversion for value offsets
func assertSameValueID(t *testing.T, goVal hive.ValueID, hivexVal bindings.ValueHandle, ...) {
    const hbinStart = 0x1000
    goAbsolute := uint64(goVal) + hbinStart

    if goAbsolute != uint64(hivexVal) {
        t.Errorf("Value IDs don't match...")
    }
}
```

## Verification

The pattern holds consistently across all test hives:

| Hive | gohivex Root | hivex Root | Difference |
|------|--------------|------------|------------|
| minimal | 32 (0x20) | 4128 (0x1020) | 4096 (0x1000) ✓ |
| special | 32 (0x20) | 4128 (0x1020) | 4096 (0x1000) ✓ |
| large | 32 (0x20) | 4128 (0x1020) | 4096 (0x1000) ✓ |

See `offset_analysis_test.go` for automated verification.

## Key Takeaways

1. **Both representations are valid** - they're just different conventions
2. **Conversion is simple** - just add/subtract 0x1000
3. **Test helpers handle it automatically** - use provided assertion functions
4. **Semantic equivalence matters** - nodes/values point to the same data
5. **Not a bug** - this is an intentional design difference

## References

- Windows Registry hive format specification
- `offset_analysis_test.go` - Automated verification
- `helpers.go` - Conversion implementations
- hivex documentation: https://libguestfs.org/hivex.3.html
