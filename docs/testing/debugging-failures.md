---
title: "Debugging Test Failures"
weight: 30
---

# Debugging Test Failures

This document provides strategies for debugging test failures in HiveKit, with a focus on creating targeted tests to identify and fix issues.

## General Approach

1. **Isolate the failure** - Run only the failing test
2. **Examine the error** - What is the actual vs expected behavior?
3. **Check hivexsh output** - Does the generated hive parse correctly?
4. **Create targeted unit tests** - Test the specific failure in isolation
5. **Fix and validate** - Ensure the fix doesn't break other tests

## Common Failure Patterns

### Pattern 1: "Trailing Garbage" Error

**Symptom:**
```
hivexsh: failed to open hive file: the block at 0x2d5ff0 size 1852400248
extends beyond the current page, bad registry
```

**Root Cause:** File size doesn't match header data size field

**Debug Steps:**
```go
// 1. Check header data size field
dataSize := getU32(data, REGFDataSizeOffset)  // At 0x28
expectedFileSize := 0x1000 + dataSize

// 2. Check actual file size
actualFileSize := len(h.Bytes())

// 3. Compare
if actualFileSize != expectedFileSize {
    fmt.Printf("Mismatch: expected=0x%X, actual=0x%X, delta=0x%X\n",
        expectedFileSize, actualFileSize, actualFileSize-expectedFileSize)
}
```

**Targeted Test:**
```go
func Test_Header_FileSizeMatchesDataSize(t *testing.T) {
    // After any operation that grows/shrinks the hive
    data := h.Bytes()
    dataSize := getU32(data, REGFDataSizeOffset)
    actualSize := len(data)
    expectedSize := format.HeaderSize + int(dataSize)

    require.Equal(t, expectedSize, actualSize,
        "File size must match header data size field")
}
```

### Pattern 2: Cell Size Corruption

**Symptom:**
```
hivexsh: block at 0xXXXX size 1852400248 extends beyond the current page
```

**Root Cause:** Cell size field contains garbage value

**Debug Steps:**
```go
// 1. Dump cells in the problematic HBIN
func dumpCellsInHBIN(data []byte, hbinOffset int) {
    hbinSize := int(getU32(data, hbinOffset+8))
    hbinEnd := hbinOffset + hbinSize

    cellOff := hbinOffset + 32  // Skip HBIN header
    for cellOff < hbinEnd {
        rawSize := getI32(data, cellOff)
        absSize := rawSize
        if absSize < 0 {
            absSize = -absSize
        }

        fmt.Printf("Cell at 0x%X: size=%d (raw=%d), allocated=%v\n",
            cellOff, absSize, rawSize, rawSize < 0)

        if absSize <= 0 || absSize > 100*1024*1024 {
            fmt.Printf("  ⚠️  SUSPICIOUS SIZE: %d\n", absSize)
            // Dump hex around this cell
            dumpHex(data[cellOff:cellOff+16])
            break
        }

        cellOff += align8(int(absSize))
    }
}
```

**Targeted Test:**
```go
func Test_AllCells_ValidSizes(t *testing.T) {
    walker.WalkAllCells(h, func(off int, size int32, allocated bool) error {
        absSize := size
        if absSize < 0 {
            absSize = -absSize
        }

        // Cell size must be reasonable
        require.Greater(t, absSize, int32(0),
            "Cell at 0x%X has non-positive size", off)
        require.Less(t, absSize, int32(100*1024*1024),
            "Cell at 0x%X has suspiciously large size: %d", off, absSize)

        return nil
    })
}
```

### Pattern 3: Sequence Number Mismatch

**Symptom:**
```
Test failed: Expected Seq1=68, got Seq1=70
```

**Root Cause:** Multiple Begin() calls or operations incrementing sequences outside transaction

**Debug Steps:**
```go
// Add logging to track sequence changes
func logSequences(label string, h *hive.Hive) {
    data := h.Bytes()
    seq1 := getU32(data, REGFPrimarySeqOffset)
    seq2 := getU32(data, REGFSecondarySeqOffset)
    fmt.Printf("%s: Seq1=%d, Seq2=%d\n", label, seq1, seq2)
}

// Use at key points:
logSequences("Initial", h)
txMgr.Begin()
logSequences("After Begin", h)
// ... operations ...
logSequences("Before Commit", h)
txMgr.Commit()
logSequences("After Commit", h)
```

**Targeted Test:**
```go
func Test_Transaction_SingleBeginIncrement(t *testing.T) {
    initial := getSeq1(h)

    txMgr.Begin()
    afterBegin := getSeq1(h)

    // Should increment exactly once
    require.Equal(t, initial+1, afterBegin)

    // Multiple operations should not change it
    for i := 0; i < 10; i++ {
        strategy.SetValue(path, fmt.Sprintf("val%d", i), REG_SZ, data)
    }

    beforeCommit := getSeq1(h)
    require.Equal(t, afterBegin, beforeCommit,
        "Seq1 should not change during operations")

    txMgr.Commit()
    afterCommit := getSeq1(h)
    require.Equal(t, afterBegin, afterCommit,
        "Seq1 should not change during Commit")
}
```

## Debugging Tools

### 1. hivexsh Debug Mode

```bash
hivexsh -d test.hiv 2>&1 | tee hivexsh-debug.txt
```

Look for:
- "sequence nos X Y" - should match
- "end of last page" - should match file size
- "trailing garbage" - indicates size mismatch
- "bad registry" - indicates corruption

### 2. Hex Dump Analysis

```go
func dumpHex(data []byte) {
    for i := 0; i < len(data); i += 16 {
        end := i + 16
        if end > len(data) {
            end = len(data)
        }

        // Offset
        fmt.Printf("%08X: ", i)

        // Hex bytes
        for j := i; j < end; j++ {
            fmt.Printf("%02X ", data[j])
        }

        // Padding
        for j := end; j < i+16; j++ {
            fmt.Printf("   ")
        }

        // ASCII
        fmt.Printf(" |")
        for j := i; j < end; j++ {
            b := data[j]
            if b >= 32 && b <= 126 {
                fmt.Printf("%c", b)
            } else {
                fmt.Printf(".")
            }
        }
        fmt.Printf("|\n")
    }
}
```

### 3. Structure Validation

```go
func validateHiveStructure(t *testing.T, h *hive.Hive) error {
    data := h.Bytes()

    // 1. Check REGF signature
    if string(data[0:4]) != "regf" {
        return fmt.Errorf("invalid REGF signature")
    }

    // 2. Check header checksum
    expectedChecksum := calculateHeaderChecksum(data)
    actualChecksum := getU32(data, REGFCheckSumOffset)
    if expectedChecksum != actualChecksum {
        return fmt.Errorf("checksum mismatch: expected=0x%X, actual=0x%X",
            expectedChecksum, actualChecksum)
    }

    // 3. Check file size vs header
    dataSize := getU32(data, REGFDataSizeOffset)
    expectedSize := format.HeaderSize + int(dataSize)
    actualSize := len(data)
    if expectedSize != actualSize {
        return fmt.Errorf("file size mismatch")
    }

    // 4. Walk all HBINs
    pos := format.HeaderSize
    for pos < len(data) {
        if pos+32 > len(data) {
            break
        }

        sig := string(data[pos : pos+4])
        if sig != "hbin" {
            return fmt.Errorf("invalid HBIN signature at 0x%X: %s", pos, sig)
        }

        hbinSize := int(getU32(data, pos+8))
        if hbinSize%4096 != 0 {
            return fmt.Errorf("HBIN size not multiple of 4KB: %d", hbinSize)
        }

        pos += hbinSize
    }

    return nil
}
```

## Creating Targeted Tests

### Step 1: Reproduce Minimally

```go
func Test_Minimal_Reproduction(t *testing.T) {
    // Start with smallest possible test case
    h, _ := createMinimalHive(t, "test.hiv", 4096)
    dt := dirty.NewTracker(h)
    txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

    // Single operation that triggers the bug
    txMgr.Begin()
    // ... minimal operation ...
    txMgr.Commit()

    // What specifically fails?
    // Add assertion for the specific failure condition
}
```

### Step 2: Test Invariants

```go
func Test_Invariant_AfterEveryGrow(t *testing.T) {
    // Test that invariants hold after each operation

    for i := 0; i < 10; i++ {
        err := fa.GrowByPages(1)
        require.NoError(t, err)

        // Check ALL invariants after each grow
        validateStructure(t, h)
        validateHBINs(t, h)
        validateChecksum(t, h)
        validateFileSize(t, h)
    }
}
```

### Step 3: Test Edge Cases

```go
func Test_EdgeCase_ZeroSizeValue(t *testing.T) {
    err := strategy.SetValue(path, "empty", REG_BINARY, []byte{})
    require.NoError(t, err)

    // Can we read it back?
    val, err := ReadValue(h, path, "empty")
    require.NoError(t, err)
    require.Equal(t, 0, len(val.Data))
}
```

## Next Steps for Current Failures

Based on the current test failures, we need to investigate:

1. **DeleteKey Operation** - What exactly is being corrupted?
2. **Cell Accounting** - Are cells being properly tracked after deletion?
3. **Index Consistency** - Does the index stay consistent with actual hive structure?

Let's create a deep-dive analysis document for these specific failures.
