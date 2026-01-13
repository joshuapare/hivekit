---
title: "Deletion Failure Analysis"
weight: 40
---

# Deletion Failure Analysis

## Overview

Multiple e2e tests are failing with the same corruption pattern after delete operations. This document provides deep analysis and targeted test strategies.

## The Pattern

### Failure Signature

```
hivexsh: the block at 0xXXXXX0 size 1852400248 extends beyond the current page
```

### Key Observations

1. **Always at page boundary** - Failures occur at offsets ending in 0xFF0 or 0xFF8
2. **Consistent garbage size** - 1852400248 (0x6E696E67) or 1852400240 (0x6E696E60)
3. **After deletion operations** - All failing tests involve DeleteKey or DeleteValue

### Evidence

| Test | Failure Offset | Size Value | ASCII Interpretation |
|------|----------------|------------|---------------------|
| Test_E2E_ComplexMerge_Session | 0x2d5ff0 | 1852400248 | `gnie` (LE: `nieg`) |
| Test_E2E_MixedOperations | 0x1e5ff8 | 1852400240 | `gni`\` (LE: `` `nig`) |
| Test_E2E_LargeDataDeletion | (similar) | (similar) | (similar) |

## Root Cause Hypothesis

### Theory 1: Freed Cell Not Properly Marked

When a cell is freed, its size field should become **positive** (free cells have positive size, allocated cells have negative size).

**Possible Bug:**
```go
// In Free() operation
func Free(ref CellRef) error {
    off := int(ref) + HeaderSize

    // Bug: Maybe we're not reading the old size correctly?
    oldSize := getI32(data, off)

    // Bug: Maybe we're writing garbage instead of making it positive?
    newSize := abs(oldSize)
    putI32(data, off, newSize)  // Should be positive for free cells
}
```

### Theory 2: Cell Size Overwritten by Stale Data

The ASCII interpretation `nieg` suggests this might be part of a string like "Engineering" or similar.

**Possible Bug:**
```go
// When freeing a cell that contained a value:
// 1. Cell is marked free
// 2. But cell CONTENT is not zeroed
// 3. Later code reads cell size from wrong offset
// 4. Reads value data as size field
```

### Theory 3: Free List Corruption

When cells are freed, they're added to free lists. The free list uses the cell's data area to store next/prev pointers.

**Possible Bug:**
```go
// Free list insertion might corrupt the size field
func insertFreeCell(off int, size int) {
    // Size field at off+0
    putI32(data, off, size)

    // Next pointer at off+4
    putI32(data, off+4, fa.freeLists[bucket].head)  // Bug: might overflow?
}
```

## Targeted Testing Strategy

### Test 1: Simple Delete-Then-Validate

```go
func Test_Delete_CellSizeRemainValid(t *testing.T) {
    // Create a key with a value
    strategy.EnsureKey([]string{"Test", "ToDelete"})
    strategy.SetValue([]string{"Test", "ToDelete"}, "Value1", REG_SZ, []byte("test\x00"))

    // Read the cell offset before deletion
    nkRef, _ := index.GetKey([]string{"Test", "ToDelete"})
    cellOff := int(nkRef) + format.HeaderSize

    // Delete the key
    strategy.DeleteKey([]string{"Test", "ToDelete"}, true)

    // Verify the cell size field is still valid
    data := h.Bytes()
    cellSize := getI32(data, cellOff)

    absSize := cellSize
    if absSize < 0 {
        absSize = -absSize
    }

    // Cell size must be reasonable
    require.Greater(t, absSize, int32(0), "Cell size must be positive after free")
    require.Less(t, absSize, int32(100*1024), "Cell size must be reasonable")

    // Cell size should be positive (free cell)
    require.Greater(t, cellSize, int32(0), "Freed cell must have positive size")
}
```

### Test 2: Walk All Cells After Deletion

```go
func Test_Delete_AllCellsValid(t *testing.T) {
    // Perform delete operations
    strategy.DeleteKey(path, true)

    // Walk ALL cells in the hive
    data := h.Bytes()
    pos := format.HeaderSize

    for pos < len(data) {
        // Check HBIN header
        if pos+32 > len(data) {
            break
        }

        sig := string(data[pos : pos+4])
        if sig != "hbin" {
            break
        }

        hbinSize := int(getU32(data, pos+8))
        hbinEnd := pos + hbinSize

        // Walk cells in this HBIN
        cellOff := pos + 32
        for cellOff < hbinEnd {
            if cellOff+4 > hbinEnd {
                break
            }

            rawSize := getI32(data, cellOff)
            if rawSize == 0 {
                break // End of cells
            }

            absSize := rawSize
            if absSize < 0 {
                absSize = -absSize
            }

            // CRITICAL ASSERTION
            require.Greater(t, absSize, int32(0),
                "Cell at 0x%X has invalid size: %d", cellOff, rawSize)
            require.Less(t, absSize, int32(100*1024*1024),
                "Cell at 0x%X has suspiciously large size: %d (0x%X)",
                cellOff, absSize, absSize)

            // Check for the specific corruption pattern
            if absSize > 1000*1024*1024 {
                // Dump hex around this cell
                t.Errorf("CORRUPTION at 0x%X:", cellOff)
                t.Logf("Hex dump:")
                for i := 0; i < 32; i += 4 {
                    if cellOff+i+4 <= len(data) {
                        val := getU32(data, cellOff+i)
                        t.Logf("  +0x%X: 0x%08X (%d)", i, val, int32(val))
                    }
                }
            }

            cellOff += align8(int(absSize))
        }

        pos += hbinSize
    }
}
```

### Test 3: Free List Integrity After Deletion

```go
func Test_Delete_FreeListIntegrity(t *testing.T) {
    // Get initial free list state
    initialFreeCount := countFreeCells(fa)

    // Create and delete a key with value
    strategy.EnsureKey([]string{"Test", "ToDelete"})
    strategy.SetValue([]string{"Test", "ToDelete"}, "Val", REG_SZ, []byte("data\x00"))

    strategy.DeleteKey([]string{"Test", "ToDelete"}, true)

    // Free list should have more entries
    finalFreeCount := countFreeCells(fa)
    require.Greater(t, finalFreeCount, initialFreeCount,
        "Free list should grow after deletion")

    // Walk free list and validate each entry
    for bucket := 0; bucket < len(fa.freeLists); bucket++ {
        list := &fa.freeLists[bucket]
        ptr := list.head

        seen := make(map[int]bool)
        for ptr != 0 {
            // Check for cycles
            require.False(t, seen[ptr], "Free list cycle detected at 0x%X", ptr)
            seen[ptr] = true

            // Validate this free cell
            off := ptr - format.HeaderSize
            data := h.Bytes()

            cellSize := getI32(data, off)
            require.Greater(t, cellSize, int32(0),
                "Free cell at 0x%X must have positive size, got %d", off, cellSize)

            // Read next pointer
            nextPtr := getI32(data, off+4)
            ptr = int(nextPtr)

            // Prevent infinite loops
            if len(seen) > 10000 {
                t.Fatal("Free list too long, possible corruption")
            }
        }
    }
}
```

### Test 4: Specific Corruption Pattern Detection

```go
func Test_Delete_NoAsciiInSizeField(t *testing.T) {
    // After deletion, scan for the specific corruption pattern

    strategy.DeleteKey(path, true)

    data := h.Bytes()

    // Scan every 4-byte aligned offset
    for off := format.HeaderSize; off < len(data)-4; off += 4 {
        val := getU32(data, off)

        // Check if this looks like ASCII text (common corruption pattern)
        // ASCII printable range: 0x20-0x7E
        b0 := byte(val & 0xFF)
        b1 := byte((val >> 8) & 0xFF)
        b2 := byte((val >> 16) & 0xFF)
        b3 := byte((val >> 24) & 0xFF)

        allPrintable := (b0 >= 0x20 && b0 <= 0x7E) &&
                        (b1 >= 0x20 && b1 <= 0x7E) &&
                        (b2 >= 0x20 && b2 <= 0x7E) &&
                        (b3 >= 0x20 && b3 <= 0x7E)

        if allPrintable {
            // This might be a cell size field that got corrupted with ASCII
            // Check if it's actually a cell size field

            // Is this at the start of a cell?
            // (Cell offsets are relative to HBIN start + 32)
            relativeOff := off - format.HeaderSize

            // Check if this could be a cell size field
            // by looking at the value
            sizeVal := int32(val)
            absSize := sizeVal
            if absSize < 0 {
                absSize = -absSize
            }

            if absSize > 1024*1024 {  // Suspiciously large
                t.Errorf("Suspicious ASCII value at 0x%X: 0x%08X ('%c%c%c%c')",
                    off, val, b0, b1, b2, b3)
                t.Errorf("  This might be a corrupted cell size field")

                // Dump context
                dumpHex(t, data[off-32:off+32])
            }
        }
    }
}
```

## Deep Dive: Examining Actual Corruption

### Step 1: Preserve Corrupted File

Modify the e2e test to save the corrupted hive:

```go
func Test_E2E_ComplexMerge_Session_Debug(t *testing.T) {
    // ... test code ...

    // Before closing, save the hive for analysis
    debugPath := "/tmp/corrupted-hive-debug.hiv"
    exec.Command("cp", hivePath, debugPath).Run()
    t.Logf("Corrupted hive saved to: %s", debugPath)

    // Don't fail on hivexsh error (we want to analyze it)
    // Just log it
    cmd := exec.Command("hivexsh", "-d", hivePath)
    output, _ := cmd.CombinedOutput()
    t.Logf("hivexsh output:\n%s", string(output))
}
```

### Step 2: Manual Analysis with xxd

```bash
# Find the corruption
xxd /tmp/corrupted-hive-debug.hiv | grep -B2 -A2 "6e69"

# The pattern 0x6E696E67 might appear as:
# 67 6e 69 6e (little endian)
# This spells "gnei" or "nieg"
```

### Step 3: Track the Deletion Path

Add logging to deletion operations:

```go
func (s *Strategy) DeleteKey(path []string, recursive bool) error {
    log.Printf("DeleteKey: path=%v, recursive=%v", path, recursive)

    nkRef, err := s.idx.GetKey(path)
    if err != nil {
        log.Printf("  Key not found: %v", err)
        return err
    }
    log.Printf("  NK ref=0x%X", nkRef)

    // ... deletion logic ...

    // Before freeing, log what we're about to free
    off := int(nkRef) + format.HeaderSize
    data := s.h.Bytes()
    sizeBefore := getI32(data, off)
    log.Printf("  Freeing NK cell: off=0x%X, size=%d", off, sizeBefore)

    err = s.alloc.Free(nkRef)
    if err != nil {
        log.Printf("  Free failed: %v", err)
        return err
    }

    sizeAfter := getI32(data, off)
    log.Printf("  After free: size=%d (should be positive)", sizeAfter)

    return nil
}
```

## Hypothesis: Dangling Pointer in Free List

Based on the pattern, the most likely bug is:

**When deleting a key with subkeys or values:**
1. NK cell is freed
2. Value list cells are freed
3. Subkey list cells are freed
4. Cells are added to free lists
5. **BUG:** Some freed cell still has a pointer to it from another structure
6. Later, that structure writes to the "freed" cell
7. The write overwrites the cell size field with garbage

### Targeted Test for This

```go
func Test_Delete_NoDanglingPointers(t *testing.T) {
    // Create a key with subkeys and values
    strategy.EnsureKey([]string{"Parent", "Child1"})
    strategy.EnsureKey([]string{"Parent", "Child2"})
    strategy.SetValue([]string{"Parent"}, "V1", REG_SZ, []byte("data1\x00"))
    strategy.SetValue([]string{"Parent"}, "V2", REG_SZ, []byte("data2\x00"))

    // Get the NK cell offset before deletion
    parentRef, _ := index.GetKey([]string{"Parent"})
    nkOff := int(parentRef) + format.HeaderSize

    // Delete the parent (recursive)
    strategy.DeleteKey([]string{"Parent"}, true)

    // The NK cell should now be free
    data := h.Bytes()
    nkSize := getI32(data, nkOff)
    require.Greater(t, nkSize, int32(0), "NK cell should be freed (positive size)")

    // Perform MORE operations (this might trigger the dangling pointer)
    for i := 0; i < 100; i++ {
        strategy.EnsureKey([]string{"NewKey", fmt.Sprintf("K%d", i)})
    }

    // Re-check the originally freed NK cell
    // Its size field should NOT have changed to garbage
    currentSize := getI32(data, nkOff)

    absSize := currentSize
    if absSize < 0 {
        absSize = -absSize
    }

    require.Less(t, absSize, int32(100*1024),
        "Originally freed cell size changed to garbage: %d (0x%X)",
        currentSize, uint32(currentSize))
}
```

## Next Steps

1. **Run the targeted tests** - Add them to a new test file
2. **Identify exact failure point** - Which test catches the bug?
3. **Add instrumentation** - Log every Free() call and cell size changes
4. **Fix the bug** - Once we know where it happens
5. **Verify with e2e tests** - Ensure e2e tests pass after fix

## Files to Examine

- `hive/edit/nkedit.go` - DeleteKey implementation
- `hive/alloc/fastalloc.go` - Free() implementation
- `hive/index/*.go` - Index update after deletion
- `hive/merge/strategy/*.go` - DeleteKey strategy implementations
