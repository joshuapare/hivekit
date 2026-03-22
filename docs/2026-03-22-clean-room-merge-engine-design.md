# Clean-Room Merge Engine (v2) — Design Spec
## Phase-Separated Pipeline for Sub-5ms Registry Hive Merges

**Date:** 2026-03-22
**Status:** Draft
**Prerequisite:** PR #15 (merge performance optimization) — merged 2026-03-22
**Goal:** Build a new merge engine from scratch that combines all validated optimizations into a purpose-built architecture, targeting 1-5ms for 1000 patches on large hives (vs 15-21ms in v1 post-optimization).

---

## 1. Context & Motivation

PR #15 achieved 58% faster merges and 67% less memory through incremental optimizations bolted onto the existing walk-apply engine. However, the current results (15-21ms for 1000 mixed ops on large hives) are still 7-10x away from the theoretical optimum of 1-3ms identified in the zero-context analysis (`docs/merge-optimization-analysis.md`, Section 10).

The gap exists because the v1 architecture has fundamental constraints:

1. **Interleaved walk and write:** v1's `walkAndApply` traverses the hive tree and mutates cells simultaneously. Mutations can trigger `Alloc → Grow → mmap remap`, invalidating cached pointers. This forced defensive re-resolution of NK cells after every op batch and limits how much data can be safely cached.

2. **Sorted ops array instead of trie:** v1 sorts ops by normalized path and builds `childrenByParent` / `opsByPath` maps to reconstruct tree structure from a flat list. The trie makes this structure explicit, eliminating map construction and normalizePath calls.

3. **Free-list allocation as default:** v1's FastAllocator searches free lists by default, with bump mode as an opt-in fast path. The v2 engine uses bump allocation as the only path for batch merges.

4. **No phase separation:** v1 cannot pre-compute all cross-references (e.g., parent NK → new subkey list → new child NK) because cell offsets aren't known until allocation time, which is interleaved with the walk.

The v2 engine addresses all four constraints by design.

### Decision Framework (from Section 9 of the optimization spec)

> "If incremental improvements achieve < 50% of the theoretical optimum, the clean-room engine is warranted."

Theoretical optimum: ~2ms for 1000 mixed ops on a large hive.
Current v1 (post PR #15): ~16-21ms.
Achievement: ~15-20% of theoretical optimum.
**Verdict: Clean-room engine is warranted.**

---

## 2. Architecture: Phase-Separated Pipeline

The v2 engine processes merges in 5 sequential phases. Each phase completes fully before the next begins.

```
Phase 1: Parse      .reg text / []Op → PatchTrie (with pre-computed hashes)
Phase 2: Walk       PatchTrie + mmap'd hive → Annotated trie (cellIdx per node)
Phase 3: Plan       Annotated trie → Space estimate + allocation manifest
Phase 4: Write      Manifest + BumpAllocator → New cells written + in-place update list
Phase 5: Flush      In-place updates applied, header finalized, sync to disk
```

### Why Phase Separation

**v1 problem:** Walk reads an NK cell, applies value ops (which may allocate → grow → remap), then re-reads the NK cell to get the subkey list. The re-read exists solely because the write may have invalidated the pointer.

**v2 solution:** Phase 2 (Walk) reads the entire hive without writing anything. The mmap is stable throughout. All cached `[]byte` slices remain valid. No generation counter, no re-resolution, no defensive double-reads. Phase 4 (Write) then does all mutations in a single sequential pass.

### Package Structure

```
hive/merge/v2/              # Pipeline coordinator, public Merge() API
hive/merge/v2/trie/         # PatchTrie: construction from ops, DFS iteration
hive/merge/v2/walk/         # Hive walker: CursorStack, MatchByHash, cellIdx annotation
hive/merge/v2/plan/         # Space estimator: cell sizing, bump region calculation
hive/merge/v2/write/        # Cell writer: NK/VK/data/list cells, subkey list rebuilder
hive/merge/v2/flush/        # Dirty accumulator: sorted in-place updates, header finalization
```

### Relationship to v1

The v2 engine **coexists** with v1 as a separate package. The v1 engine (`hive/merge/`) remains fully functional and is the default. v2 is opt-in. This allows:
- Head-to-head benchmarking on identical workloads
- Gradual migration: v2 can be selected per-call via an option
- Safe rollback if v2 has edge-case bugs
- v1 retirement only after v2 proves correct and faster across the full test matrix

### Reused from Existing Codebase (Not Reimplemented)

| Component | Package | What v2 uses |
|-----------|---------|-------------|
| LH hash filtering | `hive/subkeys/match_by_hash.go` | `MatchByHash` with `map[uint32][]string` |
| ASCII hash | `hive/subkeys/hash.go` | `Hash()` with fast-path |
| Bump allocator | `hive/alloc/bump.go` | `EnableBumpMode` / `FinalizeBumpMode` |
| Binary format | `internal/format/` | All constants, parsers, cell layout |
| .reg parser | `internal/regtext/` | `ParseReg()` for .reg file input |
| Dirty tracker | `hive/dirty/` | Page-level dirty tracking |
| Transaction mgmt | `hive/tx/` | REGF sequence numbers |
| Hive I/O | `hive/hive.go` | `Open`, `ResolveCellPayload`, mmap access |
| Raw subkey reader | `hive/subkeys/reader.go` | `ReadRaw()` for `{NKRef, Hash}` pairs |

---

## 3. Phase 1: Parse — PatchTrie (`hive/merge/v2/trie/`)

### PatchTrie Node

```go
type Node struct {
    Name      string     // original case component name
    NameLower string     // pre-lowercased for comparison
    Hash      uint32     // pre-computed LH hash (never recomputed)
    Children  []*Node    // sorted by NameLower (matches hive subkey sort order)

    // Operations at this node
    Values    []ValueOp  // set/delete value operations
    DeleteKey bool       // true = delete this entire subtree
    EnsureKey bool       // true = create this key if it doesn't exist

    // Filled during Phase 2 (Walk):
    CellIdx        uint32  // NK cell offset in hive (0xFFFFFFFF if doesn't exist)
    Exists         bool    // true if found in hive during walk
    SKCellIdx      uint32  // security descriptor cell (inherited from parent if new)
    SubKeyListRef  uint32  // existing subkey list cell offset (for rebuilds in Phase 4)
    SubKeyCount    uint32  // existing subkey count
    ValueListRef   uint32  // existing value list cell offset (for value ops in Phase 4)
    ValueCount     uint32  // existing value count
}

type ValueOp struct {
    Name   string
    Type   uint32
    Data   []byte
    Delete bool
}
```

### Construction

`Build(ops []merge.Op) *Node` takes the existing `[]Op` from v1's plan format and builds the trie:

1. For each op, split `KeyPath` into components
2. Walk/create trie nodes from root, one component per level
3. At each node, compute `Hash = subkeys.Hash(NameLower)` once
4. Attach the operation (EnsureKey, DeleteKey, ValueOp) to the leaf node
5. Keep `Children` sorted by `NameLower` at each level

**Properties:**
- Children sorted at each level = matches hive's subkey list sort order = enables two-pointer merge during subkey list rebuilds
- Compact: 1000 ops with shared prefixes produce ~1200-1500 nodes
- Construction cost: O(P x D) where P = patches, D = average depth. For 1000 patches of depth 6: ~100us

### DFS Iterator

`Walk(root *Node, fn func(node *Node, depth int) error)` provides depth-first traversal. Used by the walk phase to drive its descent into the hive.

---

## 4. Phase 2: Walk — Hive Annotation (`hive/merge/v2/walk/`)

The walk phase descends into the hive following the trie structure. It annotates each trie node with its hive cell index or marks it as non-existent. **This phase is read-only — it never modifies the hive.**

### CursorStack

```go
type CursorEntry struct {
    NKCellIdx    uint32
    NKPayload    []byte   // raw NK cell bytes — valid for entire walk (hive is read-only)
    ListRef      uint32   // subkey list cell offset
    ListPayload  []byte   // raw subkey list bytes — cached for all sibling lookups
    SubkeyCount  uint32
    SKCellIdx    uint32   // security descriptor for inheritance by new children
}
```

**Why caching is safe in v2:** The walk phase completes before any writes. The mmap is stable throughout. All cached `[]byte` slices point into valid, immutable memory. No generation counter needed, no re-resolution, no defensive double-reads.

### Walk Algorithm

```
func Annotate(h *hive.Hive, root *trie.Node) error:
    cursorStack := new stack
    push CursorEntry for root NK

    DFS over root.Children:
        for each trieNode:
            parentEntry := cursorStack.peek()

            // Build hash targets for ALL siblings at this level (single scan)
            targets := map[uint32][]string{}
            for each sibling at this level:
                subkeys.AddHashTarget(targets, sibling.NameLower)

            // Single MatchByHash call finds all needed children
            matches := subkeys.MatchByHash(h, parentEntry.ListRef, targets)

            // Annotate found nodes
            for each match:
                correspondingTrieNode.CellIdx = match.NKRef
                correspondingTrieNode.Exists = true
                correspondingTrieNode.SKCellIdx = parentEntry.SKCellIdx

            // Nodes not in matches → don't exist in hive
            for each non-matched sibling:
                sibling.Exists = false
                markSubtreeAsNew(sibling)  // skip ALL descendant lookups

            // Recurse into found children
            for each found child:
                read child NK, read child subkey list
                push CursorEntry
                recurse into child's trie children
                pop CursorEntry
```

### markSubtreeAsNew

When a path component doesn't exist in the hive, all descendants are also new. The walk sets `Exists = false` on the entire subtree without any further hive reads. For a merge creating `A\B\C\D\E` where `A` doesn't exist, this saves 8 random reads (4 NK + 4 subkey list resolutions).

### Sibling Batching

When a parent has multiple trie children, the walker builds the hash targets map once for all siblings, not per-sibling. `MatchByHash` scans the parent's subkey list once and finds all needed children in a single pass.

---

## 5. Phase 3: Plan — Space Estimation (`hive/merge/v2/plan/`)

The plan phase walks the annotated trie and computes exactly how many bytes of new cells are needed. Pure arithmetic — no hive access.

### Space Calculations

All sizes include the 4-byte cell size header and are rounded to 8-byte alignment:

| Cell Type | Size Formula |
|-----------|-------------|
| New NK | `align8(4 + 76 + nameLen)` |
| New VK | `align8(4 + 20 + nameLen)` |
| New data (non-inline, <= 16344 bytes) | `align8(4 + dataLen)` |
| Subkey list rebuild | `align8(4 + 4 + 8 * count)` — header + sig/count + entries |
| Value list rebuild | `align8(4 + 4 * count)` — header + VK refs |

**Big data (DB cells):** Values exceeding ~16 KB require chunked storage via DB header cells, blocklists, and data block chains (see `hive/bigdata/`). The initial v2 implementation handles standard data cells only (values <= 16344 bytes). Big data support is deferred as a follow-up — the vast majority of .reg merge values are well under this limit. When big data support is added, the plan phase must account for the additional DB header + blocklist + block cells in the space estimate.

### Allocation Manifest

```go
type SpacePlan struct {
    TotalNewBytes   int32    // int32 to match EnableBumpMode(int32) signature.
                             // REGF cell offsets are 32-bit; a single merge producing
                             // >2GB of new cells is unrealistic. Plan phase validates
                             // this does not overflow during accumulation (sum in int64,
                             // checked before narrowing).
    NewNKCount      int
    NewVKCount      int
    NewDataCount    int
    ListRebuilds    int
    InPlaceUpdates  int
    Manifest        []AllocEntry  // ordered for sequential bump writes
}

type AllocEntry struct {
    TrieNode  *trie.Node
    Kind      CellKind    // NK, VK, Data, SubkeyList, ValueList
    Size      int32       // aligned byte size including cell header
}
```

**Why a manifest:** The bump allocator is deterministic — cell N starts at offset `sum(sizes[0:N])`. By computing the manifest before any allocation, the write phase knows every cell's offset before writing any of them. This enables:
1. Sequential writes (optimal I/O pattern)
2. Pre-computed cross-references (parent NK → child list → child NK) without a second pass
3. Fail-fast on insufficient space

### Integration with BumpAllocator

After computing the manifest, the plan phase calls `fa.EnableBumpMode(plan.TotalNewBytes)` to pre-grow the hive with a single `ftruncate`.

**Delete-only plans:** If the plan contains only deletes and no creates, `TotalNewBytes` may be zero (only subkey list rebuilds excluding deleted entries). If `TotalNewBytes == 0`, skip bump mode entirely — the write phase only queues in-place updates (free cells, update NK fields) with no new cell allocation. Subkey list rebuilds that shrink (removing deleted entries) still need new list cells — account for these even in delete-heavy plans.

---

## 6. Phase 4: Write — Cell Creation (`hive/merge/v2/write/`)

The write phase takes the annotated trie and space plan, then creates all cells. Two sub-phases.

### Sub-phase A: Write New Cells (Sequential Bump)

Walk the manifest in order. Each entry gets a bump allocation and its cell bytes are written sequentially into the new HBIN region.

```
For each AllocEntry in manifest:
    cellRef, payload := bumpAlloc(entry.Size)
    switch entry.Kind:
        NK:  writeNKCell(payload, node.Name, parentSKCell, timestamp)
             node.CellIdx = cellRef
        VK:  writeVKCell(payload, valueOp.Name, valueOp.Type, dataRef)
        Data: copy(payload, valueOp.Data)
        SubkeyList: writeLHList(payload, mergedEntries)
        ValueList:  writeValueListCell(payload, vkRefs)
```

### Sub-phase B: Rebuild Subkey Lists (Two-Pointer Merge)

For each parent whose children changed:

```
oldEntries := subkeys.ReadRaw(h, parent.SubKeyListRef)  // {NKRef, Hash} pairs
newEntries := collect from trie children where !Exists
deletedRefs := collect from trie children where DeleteKey

merged := twoPointerMerge(oldEntries, newEntries, excluding deletedRefs)

if len(merged) <= 1012:
    Allocate new LH list cell via bump
    Write merged entries as single LH list
else:
    Partition merged into LH leaves of <= 1012 entries each
    Allocate one LH cell per leaf + one RI cell pointing to all leaves
    Write RI → LH structure (matches Windows kernel's list partitioning)

Queue in-place update: parent NK.SubKeyLists → new list/RI cell
Queue in-place update: parent NK.SubKeyCounts → len(merged)
Queue in-place update: parent NK.LastWriteTime → now
Queue free: old list cell(s) (positive size markers)
```

**RI list handling:** The Windows kernel caps LH lists at 1012 entries. Parents with >1012 subkeys use an RI (Root Index) structure: a list of cell references to LH leaf lists. The subkey list rebuilder must detect when the merged count exceeds 1012 and produce a properly partitioned RI → LH structure. For the initial v2 implementation, this can be deferred if no test fixtures produce >1012 subkeys per parent — but the plan phase must validate this assumption and fail explicitly if violated.

### Sub-phase C: Update Existing Values

For keys that exist and have value ops:

```
For each existing key with ValueOps:
    Read current value list
    For each ValueOp:
        if Delete: free VK + data cell, exclude from rebuilt list
        if Set (new): allocate VK + data via bump, add to list
        if Set (existing, same size): queue in-place update to data
        if Set (existing, different size): free old data, allocate new via bump
    Rebuild value list if count changed
    Queue NK field updates
```

### Output

A `[]InPlaceUpdate` list — all modifications to existing cells, ready for the flush phase.

---

## 7. Phase 5: Flush — Apply & Finalize (`hive/merge/v2/flush/`)

The flush phase applies all in-place updates and finalizes the hive header.

### Dirty Cell Accumulator

```go
type InPlaceUpdate struct {
    Offset int32   // absolute file offset
    Data   []byte  // bytes to write
}
```

### Flush Algorithm

```
1. Group updates by safety category:
   a. Parent NK field updates (SubKeyLists, SubKeyCounts, ValueList)
   b. SK refcount increments
   c. Free old cells (positive size markers)

2. Within each group, sort by Offset (ascending) for I/O locality

3. Apply groups in order (a → b → c):
   — Safety ordering takes priority over global offset ordering
   — Within each group, offset ordering provides sequential I/O
   — Group (a) makes new data reachable; hive is structurally valid at every step
   — Group (c) is least critical; orphaned cells waste space but don't corrupt

3. Finalize bump allocator (FinalizeBumpMode)
   — writes trailing free cell for unused space

4. Update base block header:
   — Sequence1++ (write-in-progress marker)
   — Length += new HBIN size
   — TimeStamp = now
   — Sequence2 = Sequence1 (write-complete marker)
   — CheckSum = delta XOR (O(1): XOR out 4 old values, XOR in 4 new)

5. Sync to disk (fdatasync / msync)
```

### Delta Checksum

The base block checksum is XOR of 127 DWORDs. After a merge, only 5 fields change (Sequence1, Sequence2, TimeStamp low/high, Length). Instead of recomputing the full XOR:

```go
newChecksum = oldChecksum
newChecksum ^= oldSequence1 ^ newSequence1
newChecksum ^= oldSequence2 ^ newSequence2
newChecksum ^= oldTimeStampLo ^ newTimeStampLo
newChecksum ^= oldTimeStampHi ^ newTimeStampHi
newChecksum ^= oldLength ^ newLength
```

8 XOR operations instead of 127.

### Crash Safety

The update ordering minimizes the danger window:
- After step 2a: some NKs point to new lists (valid), others to old (also valid). Hive is consistent.
- Before step 4: `Sequence1 != Sequence2` signals dirty state. Recovery can truncate to old Length.

---

## 8. Pipeline Coordinator (`hive/merge/v2/`)

### Public API

```go
package v2

// Merge applies operations to a hive using the phase-separated pipeline.
func Merge(ctx context.Context, h *hive.Hive, ops []merge.Op, opts Options) (Result, error)

// MergeRegText parses .reg text and merges into the hive.
func MergeRegText(ctx context.Context, h *hive.Hive, regText string, opts Options) (Result, error)

type Options struct{}

type Result struct {
    KeysCreated    int
    KeysDeleted    int
    ValuesSet      int
    ValuesDeleted  int
    BytesAllocated int64
    HiveGrowth     int64
    PhaseTiming    PhaseTiming
}

type PhaseTiming struct {
    Parse    time.Duration
    Walk     time.Duration
    Plan     time.Duration
    Write    time.Duration
    Flush    time.Duration
    Total    time.Duration
}
```

### Internal Pipeline

```go
func Merge(ctx context.Context, h *hive.Hive, ops []merge.Op, opts Options) (Result, error) {
    var timing PhaseTiming

    // Phase 1: Parse → PatchTrie
    t0 := time.Now()
    root, err := trie.Build(ops)
    if err != nil { return Result{}, fmt.Errorf("parse: %w", err) }
    timing.Parse = time.Since(t0)

    // Phase 2: Walk hive → annotate trie
    if err := ctx.Err(); err != nil { return Result{}, err }
    t1 := time.Now()
    if err := walk.Annotate(h, root); err != nil {
        return Result{}, fmt.Errorf("walk: %w", err)
    }
    timing.Walk = time.Since(t1)

    // Phase 3: Plan → space estimate + enable bump
    if err := ctx.Err(); err != nil { return Result{}, err }
    t2 := time.Now()
    spacePlan, err := plan.Estimate(root)
    if err != nil { return Result{}, fmt.Errorf("plan: %w", err) }
    allocator := alloc.NewFastAllocator(h, dirty.NewTracker(h.Length()))
    if spacePlan.TotalNewBytes > 0 {
        if err := allocator.EnableBumpMode(spacePlan.TotalNewBytes); err != nil {
            return Result{}, fmt.Errorf("bump: %w", err)
        }
        defer allocator.FinalizeBumpMode()
    }
    timing.Plan = time.Since(t2)

    // Phase 4: Write cells
    if err := ctx.Err(); err != nil { return Result{}, err }
    t3 := time.Now()
    dirtyUpdates, stats, err := write.Execute(h, root, spacePlan, allocator)
    if err != nil { return Result{}, fmt.Errorf("write: %w", err) }
    timing.Write = time.Since(t3)

    // Phase 5: Flush
    if err := ctx.Err(); err != nil { return Result{}, err }
    t4 := time.Now()
    if err := flush.Apply(h, dirtyUpdates, allocator); err != nil {
        return Result{}, fmt.Errorf("flush: %w", err)
    }
    timing.Flush = time.Since(t4)

    timing.Total = time.Since(t0)
    return Result{...stats, PhaseTiming: timing}, nil
}
```

### PhaseTiming for Profiling

Every phase records its wall-clock time. A future agent optimizing the engine can see exactly which phase dominates and focus there — the same methodology from the zero-context analysis.

---

## 9. Benchmarking & Success Criteria

### Head-to-Head Benchmarking

The existing benchmark harness (`tests/benchmark/`) runs the full fixture x patchset matrix. A new `BenchmarkMergeV2E2E` mirrors `BenchmarkMergeE2E` exactly — same fixtures, same patch sets, same measurements. `benchstat` comparison gives the definitive answer.

### Target Numbers

| Workload | v1 (post PR #15) | v2 target | Theoretical optimum |
|----------|-----------------|-----------|-------------------|
| medium-mixed / 1000 mixed ops | 21ms | < 5ms | ~2ms |
| large-wide / 500 creates | 6ms | < 2ms | ~1ms |
| large-wide / 1000 mixed ops | 16ms | < 5ms | ~2ms |
| small-flat / 100 creates | 1ms | < 0.5ms | ~0.3ms |

### Per-Phase Timing Budget (1000 patches, large hive)

| Phase | Budget | Dominant factor |
|-------|--------|----------------|
| Parse | < 500us | CPU: trie construction + hash computation |
| Walk | < 800us | Memory: random reads (cursor stack + hash filtering) |
| Plan | < 100us | CPU: trie traversal + arithmetic |
| Write | < 1000us | Memory: sequential writes to bump region |
| Flush | < 500us | I/O: sorted in-place updates + fdatasync |
| **Total** | **< 3ms** | |

### Correctness Criteria

1. **Semantic equivalence:** v2 must produce a structurally identical hive as v1 — same keys, same values, same types, same data, same security descriptors. Cell offsets will differ (v2's bump allocation sequences cells differently than v1), so binary comparison is not applicable. Primary test: run same ops through both engines on copies of the same hive, then walk both result hives and compare key/value trees structurally.
2. **All existing tests pass:** The full merge test suite must pass when routed through v2.
3. **Fuzz testing:** Random op sequences must produce valid hives (verified by `hive/verify/`).
4. **Benchmark E2E pass:** The `tests/benchmark/` E2E benchmarks exercise real merge operations — they must succeed through v2.

### When to Retire v1

v1 is retired when v2:
1. Passes all correctness tests (semantic equivalence with v1)
2. Benchmarks faster on every fixture x patchset combination (p < 0.05)
3. Has been used in production-equivalent testing for at least one release cycle

---

## 10. Implementation Order

```
Phase 0: Benchmark scaffolding
    Add BenchmarkMergeV2E2E to tests/benchmark/ (mirrors v1 benchmarks)

Phase 1: PatchTrie (trie/)
    Build + Walk + tests
    Independently testable — no hive access

Phase 2: Walk phase (walk/)
    CursorStack + Annotate + tests
    Depends on: trie/, subkeys/match_by_hash.go

Phase 3: Plan phase (plan/)
    SpacePlan + Manifest + tests
    Depends on: trie/ (annotated)

Phase 4: Write phase (write/)
    Cell writer + list rebuilder + tests
    Depends on: trie/, plan/, alloc/bump.go, subkeys/ReadRaw

Phase 5: Flush phase (flush/)
    DirtyAccumulator + delta checksum + tests
    Depends on: dirty/, tx/

Phase 6: Coordinator (v2/)
    Wire phases + public API + correctness tests (v1 vs v2 comparison)

Phase 7: Head-to-head benchmarking
    Run full matrix, benchstat comparison, document results
```

Phases 1-3 can be developed and tested without a fully functional pipeline. Phase 4 needs the bump allocator. Phase 5 is independent. Phase 6 wires everything together. Phase 7 validates.

---

## 11. Architectural Notes

### SubkeyListCache (from reference doc) is folded into the trie

The reference doc (`merge-optimization-analysis.md`, Section 4.3) describes a separate `SubkeyListCache` data structure keyed by parent NK cell index, with explicit `inserts`, `deletes`, and `dirty` fields. In v2, this is folded into the trie: trie children represent inserts, `DeleteKey` on children represents deletes, and the parent node's `SubKeyListRef` provides the existing list reference. A separate cache is unnecessary because the trie already provides the parent→child relationship and the walk phase annotates all needed metadata directly on the nodes.

### Thread Safety

The v2 merge engine is **not thread-safe**. A single `Merge()` call must not be concurrent with other reads or writes to the same `*hive.Hive`. This is consistent with v1 (see `hive/merge/doc.go`). Callers that need concurrent access must serialize merge calls or use separate hive instances.

### ParseReg → Op Conversion

The `MergeRegText` API parses .reg text using `internal/regtext.ParseReg()`, which returns `[]types.EditOp` (not `[]merge.Op`). The coordinator must convert `EditOp` to `merge.Op` before calling `trie.Build`. The existing conversion function `merge.convertEditOpToMergeOp` in `hive/merge/merge_prefix.go` handles this — v2 should reuse or adapt it. The conversion includes computing `PathHashes` and `NormalizedPath` on each op.

### Scope Limitations (Initial v2)

The following are explicitly deferred for follow-up work:
- **Big data (DB cells):** Values >16 KB requiring chunked storage. v2 initially handles standard data cells only.
- **RI list partitioning:** If a subkey list rebuild produces >1012 entries, v2 must fail explicitly rather than produce an invalid oversized LH list. Full RI partitioning is a follow-up.
- **Warm path cache:** Amortized index of top 3-4 tree levels across repeated merges (reference doc Section 6.1). Not needed for the initial v2.
- **Parallel phase pipeline:** Overlapping phases via goroutines (reference doc Section 6.3). v2 starts sequential; parallelism is a future optimization.
- **SIMD hash scan:** AVX2-accelerated LH hash scanning (reference doc Section 6.4). The current scalar scan is already fast enough.
