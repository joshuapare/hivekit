# V2 Follow-up: Old Cell Freeing

**Date:** 2026-03-23
**Priority:** Medium
**Scope:** `hive/merge/v2/write/execute.go`, `hive/merge/v2/flush/apply.go`
**Prerequisite:** V2 merge engine merged (PR #16)

---

## Problem

When the v2 engine rebuilds a subkey list or value list, it allocates a new cell (via bump allocator) and updates the parent NK to point to the new cell. But the old cell is never freed — its size marker stays negative (allocated), making the space unrecoverable.

Similarly, when a VK cell or data cell is replaced (value update with different size), the old cells are not freed.

This causes the hive to grow monotonically across merges, even for operations that should be space-neutral (e.g., updating an existing value to the same size).

## Cells That Need Freeing

| Operation | Old cell to free |
|-----------|-----------------|
| Subkey list rebuild (insert/delete child) | Old subkey list cell (LH/LF/LI, or RI + leaves) |
| Value list rebuild (add/remove value) | Old value list cell |
| Value replace (different size) | Old VK cell + old data cell |
| Value delete | Old VK cell + old data cell |
| Key delete | All cells in the deleted subtree (NK, VK, data, lists, SK refcount decrement) |

## Required Change

### 1. Queue free markers as InPlaceUpdates

To free a cell, write a positive size value at the cell's absolute offset. The cell header is 4 bytes at the start of the cell:
- Allocated cell: size is negative (e.g., -48 means 48 bytes allocated)
- Free cell: size is positive (e.g., 48 means 48 bytes free)

```go
// To free a cell at absoluteOffset with allocatedSize:
freeMarker := write.InPlaceUpdate{
    Offset: absoluteOffset,
    Data:   make([]byte, 4),
}
binary.LittleEndian.PutUint32(freeMarker.Data, uint32(allocatedSize)) // positive = free
```

### 2. Track old cell references before overwriting

In `rebuildSubkeyList`: before calling `subkeys.WriteRaw` to create the new list, save `node.SubKeyListRef` as the old list ref. After the new list is wired in, queue a free for the old ref.

In `processValues`: before rebuilding the value list, save the old `ValueListRef`. After the new list is written, queue a free.

For replaced VK/data cells: the existing `processValues` logic needs to detect when a value already exists (matched by name in the old value list) and free the old VK + data cells before allocating new ones.

### 3. Categorize as CategoryCellFree in flush

Free markers must be applied LAST in the flush safety order — after NK field updates (which wire in the new cells) and after SK refcount updates. The `categorize` function needs to identify these updates.

Simplest approach: add a `Category` field to `InPlaceUpdate`:
```go
type InPlaceUpdate struct {
    Offset   int32
    Data     []byte
    Category UpdateCategory // new field
}
```

Then `categorize` just returns `u.Category`.

### 4. Computing absolute offsets

Cell refs in the trie are relative offsets (from HBIN data start at 0x1000). To write a free marker via `h.Bytes()`, compute: `absoluteOffset = cellRef + format.HeaderSize` (where HeaderSize = 0x1000 = 4096).

Verify this by reading `internal/format/consts.go` for the exact offset calculation.

## Testing

1. Create a hive with a parent key + 10 children
2. Merge: delete 3 children via v2
3. Verify the old subkey list cell is freed (positive size marker)
4. Verify deleted NK cells are freed
5. Compare hive size growth between v1 and v2 for the same operation — v2 should no longer grow more than v1

## Success Criteria

- Hive growth (bytes) for update/delete operations matches v1 within 10%
- Old cells show positive size markers after merge
- No regression on existing tests
- Benchmark: `hive-growth-bytes` metric in E2E benchmarks should decrease for delete/mixed workloads
