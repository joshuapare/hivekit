# Hivex Compatibility Issues - Investigation Summary

## Overview

This document summarizes the three categories of mismatches found between hivex and gohivex through differential testing, along with unit tests created to reproduce and fix them.

## Issue 1: REG_NONE Value Handling (CRITICAL)

### Symptoms
- **Affected**: 135,000+ values across all real Windows hives
- **Pattern**: Hivex reports `REG_NONE` with 0 bytes, gohivex reports actual type and data
- **Examples**:
  ```
  Path: \moderatevalueparent\[3bytes]
  Hivex:   REG_NONE, 0 bytes
  Gohivex: REG_BINARY, 3 bytes

  Path: \another\[a]
  Hivex:   REG_NONE, 0 bytes
  Gohivex: REG_SZ, 4 bytes
  ```

### Hypothesis
There's a VK flags bit that marks values as "name-only" or "tombstone" entries. When this flag is set, the Type and DataLength fields should be ignored, and the value should be reported as REG_NONE with no data.

**Candidate flags**:
- 0x0001 is currently used for `NameIsASCII()` in vk.go
- Need to research Windows Registry VK flags to identify the correct bit

### Tests Created
- `internal/format/vk_test.go::TestDecodeVK_CompNameFlag` - Tests VK record parsing with flag 0x0001
- Currently PASSES because it only tests parsing, not semantic behavior

### Next Steps
1. Research VK flags in Windows Registry format documentation
2. Identify the correct "tombstone" or "name-only" flag bit
3. Add `VKRecord.IsCompName()` or similar method to check the flag
4. Modify `reader.value()` to return REG_NONE when flag is set
5. Re-run hivex comparison tests

### Impact
Without this fix, ~100% of real Windows hives show massive value mismatches.

---

## Issue 2: UTF-16LE Node Name Encoding (Priority 2)

### Symptoms
- **Affected**: 4 nodes total (special hive + windows-2003-server-software)
- **Pattern**: Non-ASCII UTF-16LE node names produce mojibake
- **Example**:
  ```
  Hivex:   "abcd_äöüß"
  Gohivex: "abcd_����"
  ```

### Investigation
The UTF-16LE decoding logic in `internal/reader/utf16_opt.go` appears correct:
- Fast path for ASCII detection (lines 15-36)
- Slow path uses standard `utf16.Decode` (lines 38-45)
- Handles surrogate pairs correctly

**Current code** (`key.go` lines 14-22):
```go
data := nk.NameRaw
if nk.NameIsCompressed() {
    return string(data), nil  // ASCII/CP-1252
}
// ... validate even length ...
return decodeUTF16LE(data), nil  // UTF-16LE
```

This logic seems correct. The issue may be:
1. The NameIsCompressed() flag is set incorrectly in the hive file?
2. There's a different encoding issue (CP-1252 vs UTF-16LE detection)?
3. The actual bytes in the hive need investigation?

### Tests Created
- `internal/format/nk_test.go::TestDecodeNK_UTF16Name` - Tests UTF-16LE name "abcd_äöüß"
- `internal/format/nk_test.go::TestDecodeNK_CompressedVsUTF16` - Tests flag interpretation
- Both PASS at the format level

### Next Steps
1. Examine the actual bytes of "abcd_äöüß" node in the special hive
2. Check if the compressed flag is set correctly
3. Verify decodeUTF16LE with the exact bytes from the hive
4. May need to handle CP-1252 encoding for compressed names instead of treating as ASCII

### Impact
Low impact (only 4 nodes), but causes structural mismatches (missing/extra children).

---

## Issue 3: Large Value / Multi-Cell Data (Priority 3)

### Symptoms
- **Affected**: 2 values in XP hives
- **Pattern**: "value data truncated: corrupt hive structure" errors
- **Examples**:
  ```
  windows-xp-system: \controlset001\control\productoptions\productpolicy
  windows-xp-software: \controlset001\control\session manager\appcompatibility\appcompatcache
  ```

### Investigation
Current code in `reader.go` lines 455-460:
```go
if len(dataCell.Data) < length {
    if r.opts.Tolerant {
        length = len(dataCell.Data)
    } else {
        return format.VKRecord{}, nil, &hive.Error{
            Kind: hive.ErrKindCorrupt,
            Msg:  "value data truncated",
            Err:  hive.ErrCorrupt,
        }
    }
}
```

**Hypothesis**: These values use "db" (database) records for multi-cell data storage, which is a different storage format for large values. Gohivex doesn't support this format yet.

### Tests Created
- No specific unit test created yet (requires complex hive structure)
- Can be reproduced with integration tests on XP hives

### Next Steps
1. Research Windows Registry "db" record format for large values
2. Check if these specific values use db records
3. Implement db record support if needed
4. Or document as known limitation if db support is out of scope

### Impact
Low impact (only 2 specific values), but prevents full hive traversal on affected hives.

---

## Test Summary

### Tests Created
1. `internal/format/vk_test.go::TestDecodeVK_CompNameFlag` ✅ PASS
2. `internal/format/nk_test.go::TestDecodeNK_UTF16Name` ✅ PASS
3. `internal/format/nk_test.go::TestDecodeNK_CompressedVsUTF16` ✅ PASS

### Test Results
All format-level tests PASS, which means:
- VK and NK record parsing is correct
- The bugs are in the **semantic interpretation** or **reader layer**, not parsing
- Need to investigate actual hive file contents to understand the issues

### Next Actions
1. **Priority 1**: Investigate VK flags to fix REG_NONE handling (affects 135K+ values)
2. **Priority 2**: Examine special hive node names to fix UTF-16 issue (affects 4 nodes)
3. **Priority 3**: Research db records for large value support (affects 2 values)

---

## Running Hivex Comparison Tests

```bash
# Run all hivex compatibility tests
make test-hivex-compat

# Run just the minimal hive (quick check)
make test-hivex-compat-minimal

# Run specific hive
go test -tags=hivex -v ./tests/integration/hivex -run TestHivexDirectComparison/special
```

## References

- Hivex library: https://libguestfs.org/hivex.3.html
- Windows Registry format (unofficial): https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md
- Test hives: `testdata/special`, `testdata/large`, `testdata/suite/*`
