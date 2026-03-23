# V2 Follow-up: SK Reference Count Incrementing

**Date:** 2026-03-23
**Priority:** Medium
**Scope:** `hive/merge/v2/write/execute.go`, `hive/merge/v2/flush/apply.go`
**Prerequisite:** V2 merge engine merged (PR #16)

---

## Problem

When the v2 engine creates a new NK cell, it inherits the parent's Security Descriptor (SK) cell index. The new NK correctly points to the parent's SK cell, but the SK cell's reference count is never incremented.

The REGF spec requires SK cells to maintain an accurate reference count. When a key is deleted, the refcount is decremented; if it reaches zero, the SK cell is freed. A stale (too-low) refcount means a future delete operation could free the SK cell while other keys still reference it, causing corruption.

## Current Behavior

In `write/execute.go`, `createNewKey` writes the NK cell with `skRef` set to the parent's SK cell index (inherited via `node.SKCellIdx` from the walk phase). No in-place update is queued to increment the SK cell's refcount.

## Required Change

### 1. Read the SK cell's current refcount during the write phase

After creating a new NK cell that inherits an SK cell, read the SK cell's refcount field:
- SK cell layout: refcount is at offset 0x08 from the SK cell payload start (4 bytes, uint32)
- Use `h.ResolveCellPayload(skRef)` to read the SK cell
- Read `binary.LittleEndian.Uint32(payload[8:12])` for the current refcount

### 2. Queue an in-place update to increment the refcount

```go
newRefcount := currentRefcount + skIncrementCount
update := write.InPlaceUpdate{
    Offset: skCellAbsoluteOffset + 8, // refcount field offset within SK cell
    Data:   make([]byte, 4),
}
binary.LittleEndian.PutUint32(update.Data, newRefcount)
```

Multiple new keys may share the same SK cell — accumulate the increment count per unique SK cell before queuing.

### 3. Categorize as CategorySKRefcount in flush

The `categorize` function in `flush/apply.go` currently always returns `CategoryNKField`. SK refcount updates should be categorized as `CategorySKRefcount` so they're applied in the correct safety order (after NK field updates, before cell frees).

Detection: SK refcount updates target offsets within SK cells (signature "sk" at the cell start). The simplest approach: tag the InPlaceUpdate with its category at creation time rather than inferring it from the offset.

## Testing

1. Create a hive with a parent key
2. Merge 5 new child keys under the parent via v2
3. Read the parent's SK cell refcount
4. Verify refcount increased by 5
5. Compare with v1 behavior on the same input

## Success Criteria

- SK refcount matches v1 output for identical operations
- No regression on existing v2 tests
- No regression on benchmark performance (the SK read + increment is one random read per unique SK cell, negligible)
