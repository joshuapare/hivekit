# V2 Follow-up: Plan Phase Accuracy for Value Lists on New Keys

**Date:** 2026-03-23
**Priority:** Low
**Scope:** `hive/merge/v2/plan/estimate.go`
**Prerequisite:** V2 merge engine merged (PR #16)

---

## Problem

When a new key is created with values (e.g., `EnsureKey("Key") + SetValue("Key", "Val", ...)`), the plan phase (`Estimate`) does not include a `CellValueList` entry in the manifest for the new key's value list. The write phase (`processValues`) allocates the value list cell from the bump region anyway, but since the plan didn't account for it, `TotalNewBytes` is underestimated.

This means `EnableBumpMode(TotalNewBytes)` pre-grows the file by slightly less than actually needed. When the bump region runs out, allocations fall through to the normal free-list allocator — which is O(log N) instead of O(1). The merge still succeeds but loses the performance guarantee of pure bump allocation.

## Root Cause

In `plan/estimate.go`, value list rebuilds are only estimated for EXISTING nodes that gain new values:

```go
if node.Exists && newValueCount > 0 {
    // estimate value list
}
```

For NEW nodes (`!node.Exists`), VK and data cells are estimated but the value list cell is not. The fix is to also estimate a value list for new nodes that have values.

## Required Change

In `plan/estimate.go`, after the block that creates VK + data entries for non-delete value ops, add:

```go
// Value list for new keys with values
if !node.Exists && nonDeleteValueCount > 0 {
    totalValues := nonDeleteValueCount
    vlistSize := align8(int32(format.CellHeaderSize + valueListEntrySize*totalValues))
    sp.Manifest = append(sp.Manifest, AllocEntry{Node: node, Kind: CellValueList, Size: vlistSize})
    sp.ListRebuilds++
    totalBytes += int64(vlistSize)
}
```

Where `nonDeleteValueCount` is the count of value ops that are NOT deletes (same count used for VK cell allocation).

## Testing

1. Build a trie with one new key + 5 values, all marked `!Exists`
2. Call `Estimate` and verify `TotalNewBytes` includes the value list cell
3. Verify `Manifest` contains a `CellValueList` entry
4. Run the E2E benchmark and confirm no bump exhaustion (all allocations via bump path)

## Success Criteria

- `Estimate` returns a `TotalNewBytes` that is >= the actual bytes allocated during `write.Execute` for all test workloads
- No bump exhaustion fallback in the `create-dense-500` benchmark (all 500 key+value creates stay on the bump path)
- No regression on existing tests
