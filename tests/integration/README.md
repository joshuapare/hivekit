# Integration Tests

This directory contains integration tests for gohivex, organized by purpose.

## Directory Structure

```
tests/integration/
├── hivex/                      # Hivex bindings comparison tests
│   ├── compat_test.go          # Direct differential testing vs hivex
│   ├── wrapper.go              # Wrapper for hivex bindings
│   └── comparator.go           # Tree comparison logic
├── benchmarks/                 # Performance benchmarks
│   ├── read_test.go            # Read performance benchmarks
│   └── merge_test.go           # Merge performance benchmarks
├── navigation_test.go          # Navigation API tests
├── reg_compat_test.go          # .reg file compatibility tests
├── structure_test.go           # Hive structure validation
└── debug_test.go               # Debug utilities
```

## Test Categories

### Hivex Compatibility Tests (`hivex/`)

**Purpose**: Differential testing against the reference libhivex implementation

**Files**:
- `compat_test.go` - Compares gohivex output directly against hivex (CGO bindings)
- `wrapper.go` - Go-idiomatic wrapper around generated hivex bindings
- `comparator.go` - Recursive tree comparison logic

**Requirements**:
- Build tag: `-tags=hivex`
- libhivex installed (`brew install hivex` on macOS)
- CGO enabled

**Run**:
```bash
go test -tags=hivex -v ./tests/integration/hivex -run TestHivexDirectComparison
```

**What it tests**:
- ✅ Exact match of node names between hivex and gohivex
- ✅ Exact match of value names, types, and data
- ✅ Exact match of tree structure (parent/child relationships)
- ✅ Child and value counts

### Performance Benchmarks (`benchmarks/`)

**Purpose**: Measure and compare performance characteristics

**Files**:
- `read_test.go` - Benchmarks for reading operations (tree traversal, value reading)
- `merge_test.go` - Benchmarks for merge/rewriting operations

**Run**:
```bash
go test -v ./tests/integration/benchmarks -bench=. -benchmem
```

### Registry File Compatibility (`reg_compat_test.go`)

**Purpose**: Validate gohivex against .reg file golden reference data

**What it tests**:
- Key count matches between hive and .reg file
- Value count matches
- Uses hivexregedit-generated .reg files as reference

**Run**:
```bash
go test -v ./tests/integration -run TestRegFileCompatibility
```

### Navigation Tests (`navigation_test.go`)

**Purpose**: Test navigation API (Parent, Children, etc.)

**Run**:
```bash
go test -v ./tests/integration -run TestParent
go test -v ./tests/integration -run TestNavigate
```

### Structure Tests (`structure_test.go`)

**Purpose**: Validate hive internal structure and integrity

### Debug Tests (`debug_test.go`)

**Purpose**: Debug utilities and helpers

## Test Data

Test hives are located in `../../testdata/`:
- `minimal` - Minimal test hive
- `special` - Special characters and Unicode
- `rlenvalue_test_hive` - Value length tests
- `large` - Large realistic hive
- `suite/` - Real-world Windows hives (XP, 2003, 2008, 2012)

## Removed Legacy Tests

The following files were removed during cleanup:
- ❌ `benchmark_hivex_test.go` - Replaced by bindings-based tests
- ❌ `hivex_compat_test.go` - Replaced by direct bindings comparison

## Running All Tests

```bash
# All integration tests (no hivex)
go test -v ./tests/integration

# All integration tests (with hivex comparison)
go test -tags=hivex -v ./tests/integration/...

# Just hivex comparison
go test -tags=hivex -v ./tests/integration/hivex

# Just benchmarks
go test -v ./tests/integration/benchmarks -bench=. -benchmem
```
