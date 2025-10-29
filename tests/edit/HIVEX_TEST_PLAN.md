# Hivex Integration Test Plan

## Overview

This document outlines a comprehensive test strategy to validate gohivex against the established hivex library (libguestfs). The goal is to ensure one-to-one API compatibility and verify that gohivex produces identical outputs for identical inputs.

## Why Hivex?

- **Industry Standard**: Hivex is the most widely used open-source Windows Registry hive library
- **Battle-Tested**: Used in forensics, virtualization (libguestfs), and security tools
- **Trusted Reference**: LGPL v2.1 licensed, actively maintained since 2009
- **Multi-Language**: C library with bindings for Python, Perl, OCaml, Ruby
- **CLI Tools**: Provides command-line tools we can use for black-box testing

## API Mapping: gohivex ↔ hivex

### Read Operations

| gohivex Method | hivex Function | Description | Test Priority |
|----------------|----------------|-------------|---------------|
| `reader.Root()` | `hivex_root()` | Get root node | **CRITICAL** |
| `reader.StatKey(id)` | `hivex_node_name()` | Get key metadata | **CRITICAL** |
| `reader.Subkeys(id)` | `hivex_node_children()` | List child keys | **CRITICAL** |
| `reader.Values(id)` | `hivex_node_values()` | List values | **CRITICAL** |
| `reader.StatValue(id)` | `hivex_value_key()`, `hivex_value_type()` | Get value metadata | **CRITICAL** |
| `reader.ValueBytes(id)` | `hivex_value_value()` | Get raw value data | **CRITICAL** |
| `reader.ValueString(id)` | `hivex_value_string()` | Get string value | **HIGH** |
| `reader.ValueDWORD(id)` | `hivex_value_dword()` | Get DWORD value | **HIGH** |
| `reader.ValueQWORD(id)` | `hivex_value_qword()` | Get QWORD value | **HIGH** |
| `reader.Find(path)` | `hivex_node_get_child()` (recursive) | Find key by path | **HIGH** |
| *(not implemented)* | `hivex_node_parent()` | Get parent key | **MEDIUM** |
| *(not implemented)* | `hivex_node_timestamp()` | Get key timestamp | **MEDIUM** |

### Write Operations

| gohivex Method | hivex Function | Description | Test Priority |
|----------------|----------------|-------------|---------------|
| `tx.CreateKey(path)` | `hivex_node_add_child()` | Create new key | **CRITICAL** |
| `tx.DeleteKey(path)` | `hivex_node_delete_child()` | Delete key subtree | **HIGH** |
| `tx.SetValue(path, name, val)` | `hivex_node_set_value()` | Set single value | **CRITICAL** |
| *(not implemented)* | `hivex_node_set_values()` | Set multiple values | **MEDIUM** |
| `tx.Commit(writer)` | `hivex_commit()` | Write changes to disk | **CRITICAL** |

## Test Matrix

### Category 1: Read-Only Validation (Black-Box)

**Goal**: Verify gohivex reads hives identically to hivex

#### Test 1.1: Tree Structure Comparison
**Tools**: `hivexml`, gohivex reader
**Approach**:
1. Export each test hive to XML using `hivexml testdata/X > X.xml`
2. Read same hive with gohivex and generate equivalent XML structure
3. Compare XML outputs (node names, hierarchy, counts)

**Test Cases**:
- minimal.hive
- special.hive (special characters)
- rlenvalue_test_hive.hive
- large.hive (446KB)
- Real Windows hives (if available): SAM, SYSTEM, SOFTWARE, NTUSER.DAT

**Pass Criteria**:
- ✅ Identical node count at each level
- ✅ Identical node names (case-sensitive)
- ✅ Identical hierarchy structure
- ✅ No missing or extra nodes

#### Test 1.2: Value Data Comparison
**Tools**: `hivexsh`, gohivex reader
**Approach**:
1. Use `hivexsh` to extract all values: `lsval` at each key
2. Use gohivex to extract same values
3. Compare value names, types, sizes, and raw data byte-for-byte

**Test Cases**:
- All value types: REG_SZ, REG_EXPAND_SZ, REG_BINARY, REG_DWORD, REG_QWORD, REG_MULTI_SZ
- Edge cases: Empty values, 0-length data, inline data (≤4 bytes), large data (>16KB)
- Special characters in value names

**Pass Criteria**:
- ✅ Identical value count per key
- ✅ Identical value names (byte-for-byte)
- ✅ Identical value types
- ✅ Identical value sizes
- ✅ Identical value data (byte-for-byte comparison)

#### Test 1.3: Path Navigation Comparison
**Tools**: `hivexsh`, gohivex reader
**Approach**:
1. Navigate to various paths using `hivexsh cd` command
2. Navigate to same paths using gohivex `Find(path)`
3. Verify both arrive at same node

**Test Cases**:
- Root paths: `\`
- Single-level: `\Microsoft`
- Deep paths: `\Microsoft\Windows NT\CurrentVersion`
- Paths with special chars: `\abcd_äöüß`
- Case-insensitive matching (hivex is case-insensitive)
- Non-existent paths (error handling)

**Pass Criteria**:
- ✅ Same node found for valid paths
- ✅ Same error for invalid paths
- ✅ Case-insensitive matching works

### Category 2: Round-Trip Validation

**Goal**: Verify gohivex writes hives that hivex can read correctly

#### Test 2.1: Read → Write → Read with Hivex
**Tools**: hivex CLI tools, gohivex
**Approach**:
1. Read hive with gohivex
2. Write to new file with gohivex (no modifications)
3. Read new file with `hivexsh` and `hivexml`
4. Compare structure/data with original

**Test Cases**:
- All test hives (minimal, special, rlenvalue, large)

**Pass Criteria**:
- ✅ `hivexml` successfully parses gohivex-written hive
- ✅ `hivexsh` can navigate all keys
- ✅ All values readable with correct data

#### Test 2.2: Hivex → Gohivex → Hivex Chain
**Tools**: hivex CLI tools, gohivex
**Approach**:
1. Original hive (created outside gohivex)
2. Read with gohivex, write to file A
3. Read A with hivex, verify identical to original
4. Read A with gohivex, write to file B
5. Verify B identical to A (stable output)

**Pass Criteria**:
- ✅ Hivex can read gohivex output
- ✅ Data integrity maintained
- ✅ Output is stable (A == B)

### Category 3: Write Operations Validation

**Goal**: Verify gohivex modifications match hivex behavior

#### Test 3.1: CreateKey Comparison
**Approach**:
1. Create key with gohivex: `tx.CreateKey("\\Test\\NewKey")`
2. Create same key with hivex: `hivex_node_add_child()`
3. Compare resulting hive structures

**Test Cases**:
- Single key creation
- Nested key creation with CreateParents
- Key name with special characters
- Very long key names (>255 chars)

**Pass Criteria**:
- ✅ Identical NK cell structure
- ✅ Identical subkey list format
- ✅ Identical offsets and sizes

#### Test 3.2: SetValue Comparison
**Approach**:
1. Set value with gohivex
2. Set same value with hivex
3. Compare VK cells and data cells

**Test Cases**:
- All value types (DWORD, QWORD, SZ, BINARY, MULTI_SZ)
- Inline data (≤4 bytes)
- External data (>4 bytes)
- Large data (>16KB)
- Unicode strings
- Empty/null values

**Pass Criteria**:
- ✅ Identical VK cell structure
- ✅ Identical data encoding
- ✅ Identical inline vs external data decisions

#### Test 3.3: DeleteKey Comparison
**Approach**:
1. Delete subtree with gohivex
2. Delete same subtree with hivex
3. Compare resulting hives

**Test Cases**:
- Delete leaf key
- Delete key with children
- Delete key with values
- Delete non-existent key (error case)

**Pass Criteria**:
- ✅ Identical removal of all nodes
- ✅ No orphaned cells
- ✅ Proper free space handling

### Category 4: Binary Format Validation

**Goal**: Verify binary-level compatibility

#### Test 4.1: REGF Header Validation
**Approach**:
1. Write hive with gohivex
2. Parse REGF header manually and with hivex
3. Verify all fields match spec

**Fields to Check**:
- Signature: "regf"
- Sequence numbers
- Timestamp
- Major/minor version
- Root offset
- Total size
- Checksum (if used)

#### Test 4.2: HBIN Structure Validation
**Approach**:
1. Write hive with gohivex
2. Verify HBIN headers with hivex
3. Check cell packing

**Checks**:
- ✅ HBIN signature "hbin"
- ✅ HBIN offsets correct
- ✅ HBIN sizes align to 4KB
- ✅ No cells split across HBIN boundaries
- ✅ Proper cell alignment (8-byte)

#### Test 4.3: Cell Structure Validation
**Approach**:
1. Compare NK, VK, LF cell formats
2. Use `hivex_node_struct_length()` to verify sizes
3. Check offset calculations

**Cell Types**:
- NK (key node)
- VK (value)
- LF/LH (subkey list)
- Data cells

### Category 5: Error Handling & Edge Cases

#### Test 5.1: Corrupted Hive Handling
**Approach**:
1. Create intentionally corrupted hives
2. Compare error messages from hivex vs gohivex
3. Verify both fail gracefully

**Corruption Types**:
- Invalid signature
- Truncated file
- Invalid offsets
- Circular references
- Bad cell sizes

#### Test 5.2: Large Hive Stress Test
**Approach**:
1. Create hive with 10,000+ keys
2. Process with both tools
3. Verify performance and correctness

#### Test 5.3: Special Character Handling
**Approach**:
1. Test UTF-16 encoding edge cases
2. Null bytes in names
3. Very long names
4. Emoji and unusual Unicode

## Test Harness Design

### Proposed Test Structure

```
tests/integration/
├── hivex_compat_test.go       # Main test file
├── testdata/
│   ├── minimal.hive           # Existing test hives
│   ├── special.hive
│   ├── large.hive
│   └── real_windows/          # Optional: Real Windows hives
│       ├── SAM
│       ├── SYSTEM
│       └── SOFTWARE
├── scripts/
│   ├── hivex_export.sh        # Generate hivex reference outputs
│   ├── compare_xml.py         # Compare XML structures
│   └── compare_binary.sh      # Binary diff tools
└── golden/                     # Reference outputs from hivex
    ├── minimal.xml
    ├── minimal.tree.txt
    ├── special.xml
    └── ...
```

### Test Execution Flow

```go
// Pseudo-code for test structure
func TestHivexCompatibility(t *testing.T) {
    // 1. Generate reference data using hivex
    GenerateHivexReferenceData()

    // 2. Run comparison tests
    t.Run("StructureMatch", TestTreeStructureMatches)
    t.Run("ValuesMatch", TestValuesMatch)
    t.Run("PathNavigation", TestPathNavigation)
    t.Run("RoundTrip", TestRoundTrip)
    t.Run("WriteOps", TestWriteOperations)
    t.Run("BinaryFormat", TestBinaryFormat)
}
```

### Comparison Strategy

1. **XML Comparison** (for structure):
   - Normalize whitespace
   - Sort attributes
   - Compare node counts, names, hierarchy

2. **Binary Comparison** (for values):
   - Byte-for-byte comparison of raw data
   - Hex dump diffs for mismatches

3. **Behavioral Comparison** (for write ops):
   - Same input → same output
   - Deterministic file generation

## Required Dependencies

### System Requirements
```bash
# Install hivex tools
apt-get install libhivex-bin  # Ubuntu/Debian
brew install hivex            # macOS
```

### Go Test Dependencies
```go
import (
    "encoding/xml"
    "os/exec"  // To shell out to hivex tools
    "github.com/google/go-cmp/cmp"  // For deep comparisons
)
```

## Success Criteria

### Must Pass (MVP)
- ✅ All Category 1 tests (Read-Only Validation)
- ✅ All Category 2 tests (Round-Trip)
- ✅ Test 3.1 & 3.2 (CreateKey, SetValue)
- ✅ Test 4.2 (HBIN Structure)

### Should Pass (Full Compatibility)
- ✅ All Category 3 tests (Write Operations)
- ✅ All Category 4 tests (Binary Format)
- ✅ Category 5.3 (Special Characters)

### Nice to Have
- ✅ Category 5.1 & 5.2 (Error handling, stress tests)
- ✅ Performance benchmarks vs hivex

## Implementation Phases

### Phase 1: Setup & Read Validation (2-3 days)
1. Install hivex locally
2. Generate reference XML for all test hives
3. Implement XML export in gohivex
4. Test 1.1: Tree Structure Comparison
5. Test 1.2: Value Data Comparison

### Phase 2: Round-Trip Validation (1-2 days)
1. Test 2.1: Gohivex write → Hivex read
2. Test 2.2: Stability test (A == B)

### Phase 3: Write Operations (2-3 days)
1. Test 3.1: CreateKey comparison
2. Test 3.2: SetValue comparison
3. Test 3.3: DeleteKey comparison

### Phase 4: Binary Format (1-2 days)
1. Test 4.1: REGF header validation
2. Test 4.2: HBIN structure validation
3. Test 4.3: Cell structure validation

### Phase 5: Edge Cases (1-2 days)
1. Special character handling
2. Error cases
3. Large hive stress test

## Estimated Timeline

**Total: 7-12 days** for comprehensive validation

- Phase 1: 2-3 days
- Phase 2: 1-2 days
- Phase 3: 2-3 days
- Phase 4: 1-2 days
- Phase 5: 1-2 days

## Next Steps

1. **Install hivex** on development machine
2. **Run hivex on test hives** to verify it works
3. **Generate reference data** for all test hives
4. **Implement first test** (Test 1.1: XML structure comparison)
5. **Iterate through test matrix**

## Questions to Resolve

1. Should we test against real Windows hives (SAM, SYSTEM, etc.)?
2. What's the priority: read validation vs write validation?
3. Do we need performance benchmarks against hivex?
4. Should we test write operations by comparing binary output or behavior?
5. How do we handle hivex-specific features not in gohivex (e.g., parent navigation)?

---

**Author**: Claude Code
**Date**: 2025-10-21
**Status**: DRAFT - Awaiting Review
