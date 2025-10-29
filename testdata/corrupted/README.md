# Corrupted Registry Hive Test Files

This directory contains registry hive files with specific, documented corruptions for comprehensive error handling testing. Each file tests gohivex's ability to:
1. Detect corruption with precise error reporting
2. Report exact byte offsets of corrupted structures
3. Continue parsing healthy cells in tolerant mode (where applicable)

## File Overview

| File | Base | Corruption Type | Critical? |
|------|------|-----------------|-----------|
| corrupt_regf_signature | minimal | Invalid REGF header signature | Yes |
| corrupt_regf_truncated | minimal | Truncated REGF header | Yes |
| corrupt_hbin_signature | minimal | Invalid HBIN signature | Yes |
| corrupt_hbin_size_zero | minimal | HBIN with zero size | Yes |
| corrupt_hbin_size_unaligned | minimal | HBIN size not 0x1000-aligned | Yes |
| corrupt_hbin_size_overflow | minimal | HBIN size larger than file | Yes |
| corrupt_cell_size_zero | minimal | Cell with zero size | Yes |
| corrupt_cell_offset_overflow | minimal | Cell offset near integer overflow | Yes |
| corrupt_nk_signature | minimal | Invalid NK record signature | Partial |
| corrupt_nk_truncated | minimal | Truncated NK record | Partial |
| corrupt_nk_subkey_list_invalid | minimal | Invalid subkey list offset | Partial |
| corrupt_vk_signature | special | Invalid VK record signature | Partial |
| corrupt_vk_truncated | special | Truncated VK record | Partial |
| corrupt_value_data_truncated | special | Value data length > cell size | Partial |
| corrupt_value_data_offset_invalid | special | Value data offset out of bounds | Partial |
| corrupt_subkey_list_bad_sig | special | Invalid subkey list signature | Partial |
| corrupt_value_list_offset | minimal | NK value list offset out of bounds | Partial |
| corrupt_big_data_block_list | special | Big Data blocklist corruption | Placeholder |

**Critical**: Must fail even in tolerant mode (structural integrity)
**Partial**: Can continue in tolerant mode for healthy cells
**Placeholder**: Not yet implemented (requires specific file structure)

---

## Detailed Corruption Documentation

### 1. corrupt_regf_signature
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x0000` (bytes 0-3)
- **Structure**: REGF header signature
- **Original value**: `72 65 67 66` ("regf")
- **Corrupted value**: `58 58 58 58` ("XXXX")
- **Expected behavior (strict)**: Open fails immediately with `ErrSignatureMismatch`
- **Expected behavior (tolerant)**: Must fail (header required for hive structure)
- **Test case**: `TestCorruption_RegfSignature`

### 2. corrupt_regf_truncated
- **Base file**: minimal (truncated to 2,048 bytes)
- **Corruption type**: File truncated before first HBIN
- **Structure**: REGF header (should be 4,096 bytes minimum)
- **Original size**: 8,192 bytes
- **Corrupted size**: 2,048 bytes
- **Expected behavior (strict)**: Open fails with `ErrTruncated`
- **Expected behavior (tolerant)**: Must fail (insufficient data for basic structure)
- **Test case**: `TestCorruption_RegfTruncated`

### 3. corrupt_hbin_signature
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1000` (4,096) - first HBIN signature
- **Structure**: HBIN header signature
- **Original value**: `68 62 69 6E` ("hbin")
- **Corrupted value**: `59 59 59 59` ("YYYY")
- **Expected behavior (strict)**: Accessing HBIN fails with `ErrSignatureMismatch`
- **Expected behavior (tolerant)**: Must fail (HBIN structure required)
- **Test case**: `TestCorruption_HbinSignature`

### 4. corrupt_hbin_size_zero
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1008` (4,104) - HBIN size field
- **Structure**: HBIN header, size field at +0x08
- **Original value**: `00 10 00 00` (4,096 bytes little-endian)
- **Corrupted value**: `00 00 00 00` (0 bytes)
- **Expected behavior (strict)**: HBIN parsing fails with "invalid size" error
- **Expected behavior (tolerant)**: Must fail (zero size invalid)
- **Test case**: `TestCorruption_HbinSizeZero`

### 5. corrupt_hbin_size_unaligned
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1008` (4,104) - HBIN size field
- **Structure**: HBIN header, size must be multiple of 0x1000
- **Original value**: `00 10 00 00` (4,096 bytes)
- **Corrupted value**: `34 12 00 00` (4,660 bytes - not 0x1000-aligned)
- **Expected behavior (strict)**: HBIN parsing fails with alignment error
- **Expected behavior (tolerant)**: Must fail (alignment required for cell addressing)
- **Test case**: `TestCorruption_HbinSizeUnaligned`

### 6. corrupt_hbin_size_overflow
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1008` (4,104) - HBIN size field
- **Structure**: HBIN header, size extends beyond file
- **Original value**: `00 10 00 00` (4,096 bytes)
- **Corrupted value**: `00 00 10 00` (1,048,576 bytes - 1 MB)
- **Expected behavior (strict)**: HBIN parsing fails with `ErrTruncated`
- **Expected behavior (tolerant)**: Must fail (cannot access cells beyond file)
- **Test case**: `TestCorruption_HbinSizeOverflow`

### 7. corrupt_cell_size_zero
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1020` (4,128) - first cell size
- **Structure**: Cell header (4 bytes, signed int32)
- **Original value**: `A0 FF FF FF` (-96 allocated)
- **Corrupted value**: `00 00 00 00` (0 bytes)
- **Expected behavior (strict)**: Cell parsing fails with invalid size error
- **Expected behavior (tolerant)**: Must fail (zero-size cell invalid)
- **Test case**: `TestCorruption_CellSizeZero`

### 8. corrupt_cell_offset_overflow
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x103C` (4,156) - NK subkey list offset
- **Structure**: NK record field `NKSubkeyListOffset` (+0x1C from NK start)
- **Original value**: `00 00 00 00` (no subkeys)
- **Corrupted value**: `00 FF FF FF` (0xFFFFFF00 - near max uint32)
- **Expected behavior (strict)**: Integer overflow protection triggers, bounds error
- **Expected behavior (tolerant)**: Error reported, other fields still accessible
- **Test case**: `TestCorruption_CellOffsetOverflow`

### 9. corrupt_nk_signature
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1022` (4,130) - root NK signature
- **Structure**: NK record signature (2 bytes)
- **Original value**: `6E 6B` ("nk")
- **Corrupted value**: `58 58` ("XX")
- **Expected behavior (strict)**: Root access fails with `ErrSignatureMismatch` at offset `0x1022`
- **Expected behavior (tolerant)**: Error reported with exact offset, cannot proceed (root required)
- **Test case**: `TestCorruption_NkSignature`

### 10. corrupt_nk_truncated
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1020` (4,128) - root NK cell size
- **Structure**: Cell size field, NK requires minimum 0x50 (80) bytes
- **Original value**: `A0 FF FF FF` (-96 allocated = 96 bytes)
- **Corrupted value**: `F0 FF FF FF` (-16 allocated = 16 bytes)
- **Expected behavior (strict)**: NK decoding fails with `ErrTruncated`
- **Expected behavior (tolerant)**: Error reported, cannot access this node
- **Test case**: `TestCorruption_NkTruncated`

### 11. corrupt_nk_subkey_list_invalid
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x103C` (4,156) - NK subkey list offset
- **Structure**: NK record field `NKSubkeyListOffset`
- **Original value**: `00 00 00 00` (no subkeys)
- **Corrupted value**: `EF BE AD DE` (0xDEADBEEF - obviously invalid)
- **Expected behavior (strict)**: Enumerating subkeys fails with out-of-bounds error
- **Expected behavior (tolerant)**: Error when accessing subkeys, but NK metadata (name, timestamp) still accessible
- **Test case**: `TestCorruption_NkSubkeyListInvalid`

### 12. corrupt_vk_signature
- **Base file**: special (8,192 bytes)
- **Corruption location**: Offset `0x1382` (5,762) - VK signature
- **Structure**: VK record signature (2 bytes) at offset 0x1380 cell
- **Original value**: `76 6B` ("vk")
- **Corrupted value**: `59 59` ("YY")
- **Expected behavior (strict)**: Value access fails with `ErrSignatureMismatch` at offset `0x1382`
- **Expected behavior (tolerant)**: Error reported for this value, other values remain accessible
- **Test case**: `TestCorruption_VkSignature`

### 13. corrupt_vk_truncated
- **Base file**: special (8,192 bytes)
- **Corruption location**: Offset `0x1380` (5,760) - VK cell size
- **Structure**: Cell size field, VK requires minimum 0x14 (20) bytes
- **Original value**: `E0 FF FF FF` (-32 allocated = 32 bytes)
- **Corrupted value**: `F8 FF FF FF` (-8 allocated = 8 bytes)
- **Expected behavior (strict)**: VK decoding fails with `ErrTruncated`
- **Expected behavior (tolerant)**: Error for this value, key metadata and other values accessible
- **Test case**: `TestCorruption_VkTruncated`

### 14. corrupt_value_data_truncated
- **Base file**: special (8,192 bytes)
- **Corruption location**: Offset `0x1384` (5,764) - VK data length field
- **Structure**: VK data length (4 bytes), high bit = inline flag
- **Original value**: `04 00 00 80` (inline, 4 bytes)
- **Corrupted value**: `64 00 00 80` (inline, 100 bytes - exceeds inline space)
- **Expected behavior (strict)**: Value read fails with data size mismatch
- **Expected behavior (tolerant)**: Returns partial data or error, continues for other values
- **Test case**: `TestCorruption_ValueDataTruncated`
- **Note**: Similar to existing `TestTolerantModeAllowsTruncatedValue` test

### 15. corrupt_value_data_offset_invalid
- **Base file**: special (8,192 bytes)
- **Corruption location**: Offset `0x1428` (5,160) - VK data offset field
- **Structure**: VK data offset field (4 bytes) at VK cell 0x1420
- **Original value**: `04 00 00 00` (offset relative to first HBIN)
- **Corrupted value**: `EF BE AD DE` (0xDEADBEEF - out of bounds)
- **Expected behavior (strict)**: Value data read fails with out-of-bounds error
- **Expected behavior (tolerant)**: Error for this value, other values accessible
- **Test case**: `TestCorruption_ValueDataOffsetInvalid`

### 16. corrupt_subkey_list_bad_sig
- **Base file**: special (8,192 bytes)
- **Corruption location**: Offset `0x1400` (5,120) - LH list signature
- **Structure**: Subkey list signature (2 bytes), should be "lf", "lh", "li", or "ri"
- **Original value**: `6C 68` ("lh")
- **Corrupted value**: `5A 5A` ("ZZ")
- **Expected behavior (strict)**: Enumerating subkeys fails with `ErrSignatureMismatch`
- **Expected behavior (tolerant)**: Error when listing subkeys, parent key metadata accessible
- **Test case**: `TestCorruption_SubkeyListBadSig`

### 17. corrupt_value_list_offset
- **Base file**: minimal (8,192 bytes)
- **Corruption location**: Offset `0x1048` (4,168) - NK value list offset
- **Structure**: NK record field `NKValueListOffset` (+0x28 from NK start)
- **Original value**: `FF FF FF FF` (no values, InvalidOffset)
- **Corrupted value**: `00 00 10 00` (0x100000 - beyond file)
- **Expected behavior (strict)**: Enumerating values fails with out-of-bounds error
- **Expected behavior (tolerant)**: Error when accessing values, key metadata still accessible
- **Test case**: `TestCorruption_ValueListOffset`

### 18. corrupt_big_data_block_list
- **Base file**: special (8,192 bytes)
- **Status**: **PLACEHOLDER** - neither minimal nor special contain DB (Big Data) records
- **Purpose**: Would test corruption of DB blocklist structure for large values (>16344 bytes)
- **Requirements**: Need hive file with values large enough to trigger Big Data storage
- **Test case**: `TestCorruption_BigDataBlockList` (will be skipped)
- **Note**: To properly test, would need to create or obtain a hive with large registry values

---

## Test Strategy

### Critical Corruptions (Must Fail Always)
These corruptions break fundamental hive structure and must fail in both strict and tolerant modes:
- REGF signature/truncation
- HBIN signature/size issues
- Zero/invalid cell sizes

### Recoverable Corruptions (Tolerant Mode Can Continue)
These corruptions affect specific records but allow continued access to healthy data:
- Individual NK/VK signature corruption
- Invalid offsets to sublists
- Truncated individual records
- Data length mismatches

### Test Patterns

**For critical corruptions:**
```go
func TestCorruption_Critical(t *testing.T) {
    _, err := reader.Open("testdata/corrupted/corrupt_xxx", hive.OpenOptions{})
    require.Error(t, err)
    // Assert error kind and message
}
```

**For recoverable corruptions:**
```go
func TestCorruption_Recoverable_Strict(t *testing.T) {
    r, err := reader.Open("testdata/corrupted/corrupt_xxx", hive.OpenOptions{})
    require.NoError(t, err)
    defer r.Close()

    // Operation on corrupted structure should fail
    _, err = r.SomeOperation()
    require.Error(t, err)
    // Assert error reports precise location
}

func TestCorruption_Recoverable_Tolerant(t *testing.T) {
    r, err := reader.Open("testdata/corrupted/corrupt_xxx",
        hive.OpenOptions{Tolerant: true})
    require.NoError(t, err)
    defer r.Close()

    // Should still access healthy data
    // Error on corrupted structure, but no crash
    // Verify precise error reporting
}
```

---

## Regenerating Corrupted Files

To regenerate all corrupted files:

```bash
# Generate minimal-based corruptions
./scripts/generate_corrupted_files.sh

# Generate special-based VK corruptions
./scripts/generate_corrupted_vk_files.sh
```

Both scripts are idempotent and will overwrite existing files.

---

## Notes

1. **Byte Offsets**: All offsets are absolute file positions, not relative to HBIN
2. **Cell Sizes**: Negative values indicate allocated cells (in-use), positive = free
3. **Endianness**: All multi-byte values are little-endian (Intel byte order)
4. **Alignment**: Cells are 8-byte aligned, HBINs are 0x1000-byte aligned
5. **Signatures**: 2-byte ASCII signatures are stored directly (not null-terminated)

---

## Related Files

- **Test suite**: `tests/hive/corruption_comprehensive_test.go`
- **Existing corruption test**: `tests/hive/tolerant_mode_test.go`
- **Format constants**: `internal/format/consts.go`
- **Error types**: `internal/format/errors.go` and `pkg/hive/api.go`
