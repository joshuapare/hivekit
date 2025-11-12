---
title: "Next Steps for Bug Fixes"
weight: 50
---

# Next Steps for Bug Fixes

## Summary of Work Completed

### Documentation Created ✅
1. **Architecture Overview** - Component interaction and design principles
2. **Transaction Protocol** - ACID guarantees and ordered flush protocol
3. **Sequence Number Management** - The separation between protocol and structural fields
4. **Debugging Test Failures** - Strategies for identifying and fixing bugs
5. **Deletion Failure Analysis** - Deep dive into the DeleteKey corruption bug

### Core Fix Completed ✅
- **Sequence Number Architecture** - Fixed transaction protocol violations
- **Test Suite**: 12 of 14 packages passing (2 pre-existing failures)
- **Bug Fixed**: Test_Session_MultipleOperations now passes (Seq1==Seq2)

## Current Focus: DeleteKey Corruption Bug

### Problem Statement

Multiple e2e tests failing with identical corruption pattern:
```
hivexsh: the block at 0xXXXFF0 size 1852400248 extends beyond the current page
```

### Evidence Collected

1. **Pattern**: Always at page boundaries (offset ends in 0xFF0 or 0xFF8)
2. **Size Value**: Consistent garbage (0x6E696E67 = ASCII "gnei" or "nieg")
3. **Trigger**: Only occurs after DeleteKey or DeleteValue operations
4. **Location**: Cell size field is corrupted with what appears to be string data

### Hypothesis

The most likely cause is **freed cells not being properly marked** or **dangling pointers writing to freed cells**:

```go
// Possible bug flow:
1. Key is deleted → NK cell is freed
2. Cell is added to free list
3. But: Some structure still has pointer to this cell
4. Later: That structure writes to the cell
5. Result: Cell size field is overwritten with string data
```

### Next Actions

#### Step 1: Complete Targeted Tests

Create proper targeted tests that can reproduce the bug. The initial test file needs:
- Proper index setup (not just dirty tracker)
- Reference to existing test infrastructure

```go
// Need to look at how merge tests set up editors:
session := merge.NewSession(h, opts)  // This handles index creation
strategy := session.strategy
strategy.DeleteKey(path, recursive)
```

#### Step 2: Add Instrumentation

Add logging to track every Free() call:

```go
func (fa *FastAllocator) Free(ref CellRef) error {
    off := int(ref) + format.HeaderSize
    data := fa.h.Bytes()

    sizeBefore := getI32(data, off)
    log.Printf("Free(0x%X): size before=%d", ref, sizeBefore)

    // ... freeing logic ...

    sizeAfter := getI32(data, off)
    log.Printf("Free(0x%X): size after=%d", ref, sizeAfter)

    return nil
}
```

#### Step 3: Examine DeleteKey Implementation

Review the deletion path in detail:

**Files to examine:**
- `hive/edit/nkedit.go` - DeleteKey implementation
- `hive/alloc/fastalloc.go` - Free() implementation
- `hive/index/*.go` - Index removal
- `hive/merge/strategy/*.go` - Strategy-level DeleteKey

**Questions to answer:**
1. Does DeleteKey properly free ALL associated cells? (NK, value list, values, subkey lists)
2. Does the index remove references before cells are freed?
3. Are free list pointers correctly updated?
4. Could there be a use-after-free scenario?

#### Step 4: Use hivexsh Debug Mode on Corrupted File

Modify e2e test to preserve the corrupted file:

```go
func Test_E2E_ComplexMerge_Debug(t *testing.T) {
    // ... test code that triggers bug ...

    // Save corrupted file
    debugPath := "/tmp/corrupted.hiv"
    exec.Command("cp", hivePath, debugPath).Run()
    t.Logf("Corrupted hive saved: %s", debugPath)

    // Run hivexsh in debug mode
    cmd := exec.Command("hivexsh", "-d", debugPath)
    output, _ := cmd.CombinedOutput()

    // Parse output to find exact corruption location
    // Look for "the block at 0xXXXXX"
}
```

Then manually analyze:
```bash
hivexsh -d /tmp/corrupted.hiv 2>&1 | grep -B10 "block at"
xxd /tmp/corrupted.hiv | grep "6e 69 6e 67"  # Find corruption pattern
```

#### Step 5: Minimal Reproduction

Once we understand the bug, create minimal reproduction:

```go
func Test_DeleteKey_MinimalRepro(t *testing.T) {
    // Absolute minimum code to trigger the bug
    // Start with empty hive
    // Add one key
    // Delete it
    // Check for corruption

    // If no corruption, add complexity:
    // - Add subkeys
    // - Add values
    // - Delete recursively
    // - Perform additional operations

    // Goal: Find the SMALLEST test case that reproduces
}
```

#### Step 6: Fix and Validate

Once bug is identified:
1. Implement fix
2. Verify minimal repro test passes
3. Verify all e2e tests pass
4. Run full test suite
5. Validate with hivexsh on generated files

## Alternative Approaches

### If Targeted Tests Are Difficult

Use the e2e tests directly with instrumentation:

```go
// In Test_E2E_ComplexMerge_Session:
session.Apply(plan)  // Add detailed logging here

// After each operation in the plan:
validateAllCellSizes(t, h)  // Walk all cells, check sizes
```

### If Bug Is Elusive

Add assertions throughout the codebase:

```go
// In Free():
func (fa *FastAllocator) Free(ref CellRef) error {
    // ... freeing logic ...

    // ASSERT: Size field is valid after free
    validateCellSize(ref)

    return nil
}

// In every major operation:
func (s *Strategy) DeleteKey(...) error {
    defer validateAllCells(s.h)  // Check after every delete
    // ... deletion logic ...
}
```

## Expected Timeline

| Phase | Estimated Time | Status |
|-------|----------------|--------|
| Complete targeted tests | 30 mins | In Progress |
| Add instrumentation | 15 mins | Pending |
| Examine DeleteKey impl | 30 mins | Pending |
| Debug with hivexsh | 15 mins | Pending |
| Identify root cause | ?? | Pending |
| Implement fix | 30 mins | Pending |
| Validate fix | 15 mins | Pending |
| **Total** | **~2.5 hours** | **30% Complete** |

## Success Criteria

✅ All targeted tests pass
✅ Test_E2E_ComplexMerge_Session passes
✅ Test_E2E_MixedOperations passes
✅ All other e2e tests pass
✅ hivexsh validates all generated hives
✅ No regressions in existing tests

## Resources

- [Deletion Failure Analysis]({{< ref "failure-analysis-deletions" >}})
- [Debugging Test Failures]({{< ref "debugging-failures" >}})
- [Windows Registry Specification](https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md)
