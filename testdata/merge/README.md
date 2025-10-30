# Registry Merge Integration Tests

This directory contains comprehensive test data for .reg file parsing and merging operations.

## Directory Structure

### `valid/` - Well-formed .reg files

**Basic Operations (01-07):**
- `01_simple_string.reg` - Single string value
- `02_simple_dword.reg` - Single DWORD value  
- `03_nested_keys.reg` - Multiple nested key levels
- `04_delete_key.reg` - Key deletion with [-HKEY...]
- `05_delete_value.reg` - Value deletion with "Name"=-
- `06_default_value.reg` - Default value with @=
- `07_mixed_operations.reg` - Mix of create, set, and delete operations

**All Value Types (10):**
- `10_value_types_all.reg` - All 11 supported registry value types:
  - REG_SZ, REG_EXPAND_SZ, REG_BINARY, REG_DWORD, REG_DWORD_BE
  - REG_LINK, REG_MULTI_SZ, REG_RESOURCE_LIST
  - REG_FULL_RESOURCE_DESCRIPTOR, REG_RESOURCE_REQUIREMENTS_LIST, REG_QWORD

**Edge Cases (20-27):**
- `20_unicode_strings.reg` - Unicode characters in values
- `21_special_chars.reg` - Backslashes, quotes, escaping
- `22_line_continuation.reg` - Long hex values
- `23_long_hex_values.reg` - 256 bytes of binary data
- `24_empty_string.reg` - Empty string values
- `25_zero_dword.reg` - Zero and max DWORD values
- `26_all_hkey_prefixes.reg` - HKEY_LOCAL_MACHINE prefix variations
- `27_short_aliases.reg` - HKLM vs HKEY_LOCAL_MACHINE equivalence

### `invalid/` - Malformed .reg files for error testing

- `err_no_header.reg` - Missing "Windows Registry Editor" header
- `err_bad_header.reg` - Wrong header version (4.00 instead of 5.00)
- `err_unclosed_bracket.reg` - Key path missing closing ]
- `err_invalid_dword.reg` - Malformed DWORD with non-hex characters
- `err_invalid_hex.reg` - Invalid hex data with bad characters
- `err_truncated_hex.reg` - Incomplete hex value
- `err_bad_line_continuation.reg` - Invalid line continuation
- `err_unescaped_quotes.reg` - Unescaped quotes in strings
- `err_empty_key_name.reg` - Empty key path []
- `err_missing_value_name.reg` - Assignment without value name

## Test Coverage

The integration tests in `pkg/hive/merge_integration_test.go` provide:

- ✅ All 11+ registry value types
- ✅ Create/delete operations for keys and values
- ✅ Nested key hierarchies  
- ✅ Default values (@=)
- ✅ Unicode and special characters
- ✅ Long hex values
- ✅ HKEY prefix handling and short aliases
- ✅ Malformed files (10 error cases)
- ✅ Operation counting and statistics
- ✅ Prefix stripping (AutoPrefix option)
- ✅ End-to-end parse → merge → verify workflow

## Running Tests

```bash
# Run all integration tests
go test -v -run TestMergeIntegration ./pkg/hive/

# Run specific test suite
go test -v -run TestMergeIntegration_ValidFiles ./pkg/hive/
go test -v -run TestMergeIntegration_InvalidFiles ./pkg/hive/
go test -v -run TestMergeIntegration_Statistics ./pkg/hive/
```

## Test Results

All 16 valid files and 8 invalid files are tested with proper assertions for:
- Operation counts (total, keys created/deleted, values set/deleted)
- Error handling for malformed files
- Statistics collection accuracy
- Prefix stripping behavior
