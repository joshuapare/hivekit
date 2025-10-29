# Hivex Direct Comparison Tests

This directory contains integration tests that perform **direct differential testing** between:
- **hivex** (reference C implementation via CGO bindings)
- **gohivex** (native Go implementation)

## Overview

These tests open the same hive file with both implementations and recursively compare:
- ✅ Key names and paths
- ✅ Child key counts
- ✅ Value counts
- ✅ Value names
- ✅ Value types (REG_SZ, REG_DWORD, etc.)
- ✅ Value data content (byte-by-byte)

**No golden reference files needed** - just direct comparison between implementations.

## Prerequisites

### 1. Install libhivex

**macOS:**
```bash
brew install hivex
```

**Linux (Debian/Ubuntu):**
```bash
sudo apt-get install libhivex-dev libhivex-bin
```

**Verify installation:**
```bash
pkg-config --libs hivex
# Should output: -lhivex
```

### 2. Install Go hivex bindings

```bash
go get github.com/gabriel-samfira/go-hivex
```

This provides CGO bindings to libhivex.

## Running Tests

### Quick Start

```bash
# Run all direct comparison tests
make test-hivex-direct
```

### Manual Invocation

```bash
# Run all test hives
go test -tags=hivex -v ./tests/integration -run TestHivexDirectComparison

# Run specific hive
go test -tags=hivex -v ./tests/integration -run TestHivexDirectComparison/minimal

# Run single node test (debugging)
go test -tags=hivex -v ./tests/integration -run TestHivexDirectComparison_SingleNode

# Run wrapper tests
go test -tags=hivex -v ./tests/integration -run TestHivexWrapper
```

### Test Output

**Success (perfect match):**
```
=== RUN   TestHivexDirectComparison/minimal
    Opening minimal with hivex (CGO)
    Opening minimal with gohivex (native Go)
    Comparing trees recursively...
    ✓ Perfect match: 42 nodes, 128 values compared, 0 differences
    ✓ Perfect match: 42 nodes compared, 128 values compared
--- PASS: TestHivexDirectComparison/minimal (0.15s)
```

**Failure (differences found):**
```
=== RUN   TestHivexDirectComparison/minimal
    Found 3 mismatches between hivex and gohivex:
      [value_type] \Software\Test\[Version]: Value type mismatch for "Version"
        hivex:   REG_SZ
        gohivex: REG_EXPAND_SZ
      [value_data] \Software\Test\[Count]: Value data mismatch for "Count" (hivex: 4 bytes, gohivex: 8 bytes)
        hivex:   4 bytes
        gohivex: 8 bytes
      ... and 1 more mismatches
--- FAIL: TestHivexDirectComparison/minimal (0.15s)
```

## Architecture

### Components

1. **hivex_wrapper.go** (`// +build hivex`)
   - Safe wrappers around gabriel-samfira/go-hivex
   - Provides: `OpenHivex()`, `NodeName()`, `NodeChildren()`, `ValueValue()`, etc.
   - Handles CGO memory management

2. **tree_comparator.go** (`// +build hivex`)
   - `CompareTreesRecursively()` - Main comparison function
   - `compareNodesRecursive()` - Recursive tree walker
   - `compareValues()` - Value comparison
   - Returns `ComparisonResult` with all mismatches

3. **hivex_direct_compat_test.go** (`// +build hivex`)
   - Table-driven tests for all hives in `testHives` slice
   - `TestHivexDirectComparison` - Full tree comparison
   - `TestHivexDirectComparison_SingleNode` - Single node debugging
   - `TestHivexWrapper_BasicOperations` - Wrapper unit tests

### Test Flow

```
1. Open hive with hivex (CGO)
   └─> hivex.NewHivex(path) → HivexHandle

2. Open hive with gohivex (native Go)
   └─> reader.Open(path) → hive.Reader

3. Get root nodes from both
   ├─> hivexHandle.Root() → int64
   └─> gohivexReader.Root() → hive.NodeID

4. Recursive comparison
   └─> CompareTreesRecursively()
       ├─> Compare node metadata (name, child count, value count)
       ├─> Compare all values (name, type, data)
       └─> Recurse into all matching children

5. Assert zero mismatches
   └─> Result.Mismatches should be empty
```

## Test Hives

All hives from `testHives` slice are tested:
- `minimal` - Minimal valid hive
- `special` - Special characters
- `rlenvalue_test_hive` - Large values
- `large` - Multi-HBIN hive
- `windows-xp-system` - Real Windows XP SYSTEM
- `windows-xp-software` - Real Windows XP SOFTWARE
- `windows-2003-server-system` - Windows Server 2003
- `windows-2003-server-software` - Windows Server 2003
- `windows-8-enterprise-system` - Windows 8
- `windows-8-enterprise-software` - Windows 8
- `windows-2012-system` - Windows Server 2012
- `windows-2012-software` - Windows Server 2012

## Troubleshooting

### CGO Build Errors

**Error:** `package github.com/gabriel-samfira/go-hivex: C source files not allowed when not using cgo`

**Solution:** CGO must be enabled:
```bash
export CGO_ENABLED=1
go test -tags=hivex ...
```

### Missing libhivex

**Error:** `could not determine kind of name for C.hivex_open`

**Solution:** Install libhivex development headers:
```bash
# macOS
brew install hivex

# Linux
sudo apt-get install libhivex-dev

# Verify
pkg-config --libs hivex
```

### Go Module Issues

**Error:** `module github.com/gabriel-samfira/go-hivex: not found`

**Solution:** Install the Go bindings:
```bash
go get github.com/gabriel-samfira/go-hivex
```

### Build Tags

**Important:** All hivex comparison code uses `// +build hivex` build tag.

Without the tag, these files are excluded from normal builds (to avoid CGO dependency).

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Hivex Integration Tests

on: [push, pull_request]

jobs:
  hivex-comparison:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install libhivex
        run: |
          sudo apt-get update
          sudo apt-get install -y libhivex-dev libhivex-bin

      - name: Install go-hivex bindings
        run: go get github.com/gabriel-samfira/go-hivex

      - name: Decompress test hives
        run: make decompress-hives

      - name: Run hivex direct comparison tests
        run: make test-hivex-direct
```

## Performance

Typical test times (on modern hardware):

| Hive | Size | Nodes | Values | Time |
|------|------|-------|--------|------|
| minimal | 4 KB | ~10 | ~5 | 0.01s |
| large | 100 KB | ~500 | ~200 | 0.10s |
| windows-xp-system | 10 MB | ~20k | ~50k | 5-10s |
| windows-2012-software | 50 MB | ~100k | ~200k | 30-60s |

## Comparison with Golden File Tests

### Old Approach (hivex_compat_test.go)
- ❌ Requires pre-generated golden .reg files
- ❌ Two-step process: hivex → .reg → compare
- ❌ Must know expected values upfront
- ❌ .reg file parsing complexity
- ✅ Can run without CGO

### New Approach (hivex_direct_compat_test.go)
- ✅ No golden files needed
- ✅ Direct comparison: hivex ↔ gohivex
- ✅ Differential testing (just check if they differ)
- ✅ Simpler implementation
- ❌ Requires CGO + libhivex

Both approaches are complementary and serve different purposes.

## Contributing

When adding new test hives:

1. Add to `testHives` slice in `hivex_compat_test.go`
2. Place hive file in `testdata/` or `testdata/suite/`
3. Run `make test-hivex-direct` to validate

All test hives will automatically be tested by the direct comparison framework.

## References

- **libhivex**: https://libguestfs.org/hivex.3.html
- **go-hivex bindings**: https://github.com/gabriel-samfira/go-hivex
- **Windows Registry format**: https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md
