# V2 Follow-up Implementation Plans

> **For agentic workers:** Use superpowers:subagent-driven-development to implement each follow-up as a separate PR. Each section below is a self-contained task.

**Module:** `github.com/joshuapare/hivekit`
**Working directory:** Create a worktree per PR from `main`:
```bash
cd /path/to/hivekit.git
git worktree add -b fix/v2-sk-refcount fix/v2-sk-refcount main
git worktree add -b fix/v2-cell-freeing fix/v2-cell-freeing main
git worktree add -b fix/v2-plan-accuracy fix/v2-plan-accuracy main
```

---

## Plan 1: SK Reference Count Incrementing

**Spec:** `docs/v2-followup-specs/01-sk-refcount.md`
**Branch:** `fix/v2-sk-refcount`
**Files to modify:** `hive/merge/v2/write/execute.go`, `hive/merge/v2/write/types.go`, `hive/merge/v2/flush/apply.go`
**Files to create:** `hive/merge/v2/write/sk_refcount_test.go`

### Steps

- [ ] **Step 1:** Read the SK cell format. Find the refcount field offset in `internal/format/consts.go` (look for SK-related constants). Read `hive/edit/nkedit.go` to see how v1 handles SK refcounting during key creation.

- [ ] **Step 2:** Add a `Category UpdateCategory` field to `write.InPlaceUpdate` in `write/types.go`. Default to `CategoryNKField` (0) for backward compatibility.

- [ ] **Step 3:** Update `flush/apply.go` `categorize` to return `u.Category` instead of always returning `CategoryNKField`.

- [ ] **Step 4:** Write test `TestSKRefcount_NewKeysIncrementParentSK` in `write/sk_refcount_test.go`:
  - Create a hive with builder, read the root SK refcount
  - Create 5 new keys via v2.Merge
  - Read the root SK refcount again
  - Assert it increased by 5

- [ ] **Step 5:** In `write/execute.go`, after `createNewKey` writes the NK cell:
  - Track `skRefIncrements map[uint32]uint32` (SK cell ref → increment count) on the executor
  - For each new NK that inherits an SK cell, increment the count
  - After all nodes processed, for each SK cell with increments > 0:
    - Read current refcount via `h.ResolveCellPayload`
    - Queue InPlaceUpdate with `Category: CategorySKRefcount`

- [ ] **Step 6:** Run tests: `go test ./hive/merge/v2/... -timeout 120s`
- [ ] **Step 7:** Run full suite: `go test ./... -timeout 300s`
- [ ] **Step 8:** Commit: `fix(v2): increment SK reference count when creating new keys`

---

## Plan 2: Old Cell Freeing

**Spec:** `docs/v2-followup-specs/02-cell-freeing.md`
**Branch:** `fix/v2-cell-freeing`
**Depends on:** Plan 1 (the `Category` field on InPlaceUpdate)
**Files to modify:** `hive/merge/v2/write/execute.go`, `hive/merge/v2/flush/apply.go`
**Files to create:** `hive/merge/v2/write/cell_free_test.go`

### Steps

- [ ] **Step 1:** Understand cell freeing. Read `hive/alloc/fastalloc.go` to see how v1 frees cells (writes positive size marker). The absolute offset formula: `absoluteOffset = cellRef + format.HeaderSize` where `format.HeaderSize = 0x1000`.

- [ ] **Step 2:** Add a helper to the executor:
  ```go
  func (ex *executor) queueCellFree(cellRef uint32, cellSize int32) {
      absOff := int32(cellRef) + int32(format.HeaderSize)
      data := make([]byte, 4)
      binary.LittleEndian.PutUint32(data, uint32(cellSize)) // positive = free
      ex.updates = append(ex.updates, write.InPlaceUpdate{
          Offset:   absOff,
          Data:     data,
          Category: flush.CategoryCellFree,
      })
  }
  ```

- [ ] **Step 3:** In `rebuildSubkeyList`: before writing the new list, save `node.SubKeyListRef`. After the new list is wired in, call `queueCellFree(oldListRef, oldListCellSize)`. To get the old cell size, read the cell header at `oldListRef` (4 bytes, negative value = allocated size).

- [ ] **Step 4:** In `processValues`: when replacing a value (existing VK matched by name), free the old VK cell and its data cell before allocating new ones.

- [ ] **Step 5:** Write test `TestCellFree_SubkeyListFreed`:
  - Create hive with parent + 10 children
  - Add 3 new children via v2.Merge
  - Read the old subkey list cell offset (before merge) and verify it now has a positive size marker (freed)

- [ ] **Step 6:** Write test `TestCellFree_HiveGrowthReduced`:
  - Run the same merge via v1 and v2
  - Compare hive-growth-bytes — v2 should be closer to v1 now

- [ ] **Step 7:** Run tests + benchmarks, verify `hive-growth-bytes` decreased for delete/mixed workloads
- [ ] **Step 8:** Commit: `fix(v2): free old cells when rebuilding subkey and value lists`

---

## Plan 3: Value List Plan Accuracy

**Spec:** `docs/v2-followup-specs/03-plan-accuracy.md`
**Branch:** `fix/v2-plan-accuracy`
**Files to modify:** `hive/merge/v2/plan/estimate.go`
**Files to create:** (add test cases to existing `hive/merge/v2/plan/estimate_test.go`)

### Steps

- [ ] **Step 1:** Read `plan/estimate.go`. Find the section that estimates value list rebuilds for existing nodes. Understand why new nodes are excluded.

- [ ] **Step 2:** Write test `TestEstimate_NewKeyWithValues`:
  ```go
  // Build trie: one new key with 3 values, marked !Exists
  // Call Estimate
  // Assert Manifest contains a CellValueList entry
  // Assert TotalNewBytes includes the value list size
  ```

- [ ] **Step 3:** Run test to verify it fails (value list not in manifest).

- [ ] **Step 4:** In `estimate.go`, after the VK/data cell estimation block for `!node.Exists` nodes, add:
  ```go
  if nonDeleteCount > 0 {
      vlistSize := align8(int32(format.CellHeaderSize + valueListEntrySize*nonDeleteCount))
      sp.Manifest = append(sp.Manifest, AllocEntry{Node: node, Kind: CellValueList, Size: vlistSize})
      sp.ListRebuilds++
      totalBytes += int64(vlistSize)
  }
  ```
  Where `nonDeleteCount` is the number of non-delete value ops (already computed for VK estimation).

- [ ] **Step 5:** Run test to verify it passes.
- [ ] **Step 6:** Run full v2 test suite: `go test ./hive/merge/v2/... -timeout 120s`
- [ ] **Step 7:** Commit: `fix(v2/plan): include value list cells for new keys in space estimate`

---

## Execution Order

```
Plan 3 (plan accuracy)     ← Independent, smallest, do first
Plan 1 (SK refcount)       ← Independent of Plan 3
Plan 2 (cell freeing)      ← Depends on Plan 1 (uses Category field)
```

Plans 1 and 3 can be done in parallel. Plan 2 should come after Plan 1.
