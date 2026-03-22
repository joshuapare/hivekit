# Clean-Room Merge Engine (v2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a phase-separated v2 merge engine targeting sub-5ms for 1000 patches on large hives, coexisting alongside v1 for head-to-head benchmarking.

**Architecture:** Five sequential phases (Parse → Walk → Plan → Write → Flush), each in its own sub-package under `hive/merge/v2/`. The PatchTrie replaces sorted ops arrays. Phase separation eliminates mmap invalidation issues. Bump allocation is the default. Reuses existing `MatchByHash`, `Hash`, `ReadRaw`, and allocator from PR #15.

**Tech Stack:** Go 1.25.3, module `github.com/joshuapare/hivekit`. All paths relative to `main/` worktree.

**Spec:** `docs/2026-03-22-clean-room-merge-engine-design.md`

---

## File Structure

### New Files

```
hive/merge/v2/
  doc.go                    # Package documentation
  merge.go                  # Coordinator: Merge() and MergeRegText() public API
  merge_test.go             # Integration tests: v1 vs v2 semantic equivalence
  result.go                 # Result, PhaseTiming, Options types

hive/merge/v2/trie/
  node.go                   # Node, ValueOp types
  build.go                  # Build(ops) → *Node
  build_test.go             # Trie construction tests
  walk.go                   # DFS iterator
  walk_test.go              # Iterator tests

hive/merge/v2/walk/
  annotate.go               # Annotate(h, root) — read-only hive walk
  annotate_test.go          # Walk phase tests
  cursor.go                 # CursorEntry, CursorStack
  cursor_test.go            # Cursor stack unit tests

hive/merge/v2/plan/
  estimate.go               # Estimate(root) → SpacePlan
  estimate_test.go          # Space estimation tests
  types.go                  # SpacePlan, AllocEntry, CellKind

hive/merge/v2/write/
  execute.go                # Execute(h, root, plan, alloc) → updates, stats
  execute_test.go           # Write phase tests
  cells.go                  # NK/VK/data/list cell writers
  cells_test.go             # Cell writer unit tests
  subkeymerge.go            # Two-pointer sorted merge for subkey lists
  subkeymerge_test.go       # Merge algorithm tests

hive/merge/v2/flush/
  apply.go                  # Apply(h, updates, alloc) — ordered flush
  apply_test.go             # Flush phase tests
  checksum.go               # Delta checksum computation
  checksum_test.go          # Checksum tests

tests/benchmark/
  merge_v2_e2e_bench_test.go # V2 E2E benchmarks (mirrors v1 matrix)
```

### Key API Dependencies (from existing codebase)

| API | Package | Signature |
|-----|---------|-----------|
| `hive.Open` | `hive/loader_unix.go:15` | `Open(path string) (*Hive, error)` |
| `hive.ParseNK` | `hive/nk.go:18` | `ParseNK(payload []byte) (NK, error)` |
| `NK.SubkeyListOffsetRel` | `hive/nk.go:65` | `() uint32` |
| `NK.ValueListOffsetRel` | `hive/nk.go:80` | `() uint32` |
| `NK.SecurityOffsetRel` | `hive/nk.go:85` | `() uint32` |
| `subkeys.MatchByHash` | `hive/subkeys/match_by_hash.go:52` | `(h, listRef, targets map[uint32][]string) ([]MatchedEntry, error)` |
| `subkeys.AddHashTarget` | `hive/subkeys/match_by_hash.go:22` | `(targets map[uint32][]string, name string)` |
| `subkeys.ReadRaw` | `hive/subkeys/reader.go:117` | `(h, listRef) ([]RawEntry, error)` |
| `subkeys.Hash` | `hive/subkeys/hash.go:18` | `(name string) uint32` |
| `alloc.NewFast` | `hive/alloc/fastalloc.go:245` | `(h, dt, config) (*FastAllocator, error)` |
| `alloc.EnableBumpMode` | `hive/alloc/bump.go:36` | `(totalNeeded int32) error` |
| `dirty.NewTracker` | `hive/dirty/dirty.go:68` | `(h *hive.Hive) *Tracker` |
| `tx.NewManager` | `hive/tx/tx.go:44` | `(h, dt, mode) *Manager` |
| `merge.Op` | `hive/merge/ops.go:57` | struct with Type, KeyPath, PathHashes, NormalizedPath |
| `merge.convertEditOpToMergeOp` | `hive/merge/merge_prefix.go:153` | `(editOp types.EditOp) (*Op, error)` — **unexported**; v2 must either export it, duplicate the logic, or extract to a shared internal package |
| `regtext.ParseReg` | `internal/regtext/parser.go:16` | `(data []byte, opts) ([]types.EditOp, error)` |
| `format.InvalidOffset` | `internal/format/consts.go:87` | `0xFFFFFFFF` |
| `format.CellHeaderSize` | `internal/format/consts.go:51` | `4` |
| `format.HBINHeaderSize` | `internal/format/consts.go:47` | `0x20` (32) |

---

## Task 0: V2 Benchmark Scaffolding

**Files:**
- Create: `tests/benchmark/merge_v2_e2e_bench_test.go`

This must exist before implementation begins so each phase can be validated incrementally. Initially the benchmark will fail (v2 API doesn't exist yet) — that's expected. It compiles once Task 7 provides the `v2.Merge` function.

- [ ] **Step 1: Write V2 E2E benchmark scaffold**

Create `tests/benchmark/merge_v2_e2e_bench_test.go` that mirrors the v1 `merge_e2e_bench_test.go` structure but calls `v2.Merge`. Mark the benchmark body with `b.Skip("v2 engine not yet implemented")` as a placeholder until Task 7 provides the API.

Use naming convention `BenchmarkMergeV2E2E/<fixture>/<patchset>` for benchstat compatibility.

- [ ] **Step 2: Verify it compiles and skips**

Run: `cd main && go test ./tests/benchmark/ -bench 'BenchmarkMergeV2E2E' -benchtime=1x -timeout 30s`
Expected: All benchmarks SKIP.

- [ ] **Step 3: Commit**

```bash
git add tests/benchmark/merge_v2_e2e_bench_test.go
git commit -m "feat(benchmark): scaffold V2 merge E2E benchmarks (skipped until v2 API exists)"
```

---

## Task 1: PatchTrie — Node Types and Builder

**Files:**
- Create: `hive/merge/v2/trie/node.go`
- Create: `hive/merge/v2/trie/build.go`
- Create: `hive/merge/v2/trie/build_test.go`

This task builds the PatchTrie data structure. No hive access — pure data structure work.

- [ ] **Step 1: Write trie construction test**

Create `hive/merge/v2/trie/build_test.go`:
```go
package trie

import (
    "testing"

    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/stretchr/testify/require"
)

func TestBuild_SingleKey(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software", "Test"}},
    }
    root := Build(ops)
    require.NotNil(t, root)
    require.Len(t, root.Children, 1)
    require.Equal(t, "software", root.Children[0].NameLower)
    require.Len(t, root.Children[0].Children, 1)
    require.Equal(t, "test", root.Children[0].Children[0].NameLower)
    require.True(t, root.Children[0].Children[0].EnsureKey)
}

func TestBuild_PrefixSharing(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software", "A"}},
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software", "B"}},
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software", "C"}},
    }
    root := Build(ops)
    // Single "Software" node with 3 children
    require.Len(t, root.Children, 1)
    sw := root.Children[0]
    require.Equal(t, "software", sw.NameLower)
    require.Len(t, sw.Children, 3)
    // Children should be sorted by NameLower
    require.Equal(t, "a", sw.Children[0].NameLower)
    require.Equal(t, "b", sw.Children[1].NameLower)
    require.Equal(t, "c", sw.Children[2].NameLower)
}

func TestBuild_ValueOps(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpSetValue, KeyPath: []string{"Key"}, ValueName: "Val1",
            ValueType: 4, Data: []byte{1, 0, 0, 0}},
        {Type: merge.OpDeleteValue, KeyPath: []string{"Key"}, ValueName: "Val2"},
    }
    root := Build(ops)
    require.Len(t, root.Children, 1)
    node := root.Children[0]
    require.Len(t, node.Values, 2)
    require.Equal(t, "Val1", node.Values[0].Name)
    require.False(t, node.Values[0].Delete)
    require.Equal(t, "Val2", node.Values[1].Name)
    require.True(t, node.Values[1].Delete)
}

func TestBuild_DeleteKey(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpDeleteKey, KeyPath: []string{"Software", "Old"}},
    }
    root := Build(ops)
    node := root.Children[0].Children[0]
    require.True(t, node.DeleteKey)
}

func TestBuild_HashesPreComputed(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software"}},
    }
    root := Build(ops)
    node := root.Children[0]
    require.NotZero(t, node.Hash, "LH hash should be pre-computed")
}

func TestBuild_EmptyOps(t *testing.T) {
    root := Build(nil)
    require.NotNil(t, root)
    require.Len(t, root.Children, 0)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd main && go test ./hive/merge/v2/trie/ -run TestBuild -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement Node types**

Create `hive/merge/v2/trie/node.go`:
```go
package trie

// Node represents a single key in the PatchTrie.
// Each node maps to one path component in the registry hierarchy.
type Node struct {
    Name      string   // original case component name
    NameLower string   // pre-lowercased for comparison
    Hash      uint32   // pre-computed LH hash (computed once during Build)
    Children  []*Node  // sorted by NameLower

    // Operations at this node
    Values    []ValueOp
    DeleteKey bool
    EnsureKey bool

    // Filled during walk phase (Phase 2):
    CellIdx        uint32 // NK cell offset (InvalidOffset if doesn't exist)
    Exists         bool   // true if found in hive
    SKCellIdx      uint32 // security descriptor cell offset
    SubKeyListRef  uint32 // existing subkey list cell offset
    SubKeyCount    uint32 // existing subkey count
    ValueListRef   uint32 // existing value list cell offset
    ValueCount     uint32 // existing value count
}

// ValueOp describes a value set or delete within a merge.
type ValueOp struct {
    Name   string
    Type   uint32
    Data   []byte
    Delete bool
}
```

- [ ] **Step 4: Implement Build function**

Create `hive/merge/v2/trie/build.go`:
```go
package trie

import (
    "slices"
    "strings"

    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/joshuapare/hivekit/hive/subkeys"
    "github.com/joshuapare/hivekit/internal/format"
)

// Build constructs a PatchTrie from a list of merge operations.
// The returned root node represents the hive root — its children are
// the top-level keys referenced by the ops.
//
// Children at each level are sorted by NameLower to match hive subkey
// list sort order. LH hashes are pre-computed on every node.
func Build(ops []merge.Op) *Node {
    root := &Node{
        Name:    "",
        CellIdx: format.InvalidOffset,
    }

    for i := range ops {
        op := &ops[i]
        node := root

        // Walk/create trie path for each component
        for _, component := range op.KeyPath {
            lower := strings.ToLower(component)
            child := findChild(node, lower)
            if child == nil {
                child = &Node{
                    Name:      component,
                    NameLower: lower,
                    Hash:      subkeys.Hash(lower),
                    CellIdx:   format.InvalidOffset,
                }
                node.Children = insertSorted(node.Children, child)
            }
            node = child
        }

        // Attach operation to the leaf node
        switch op.Type {
        case merge.OpEnsureKey:
            node.EnsureKey = true
        case merge.OpDeleteKey:
            node.DeleteKey = true
        case merge.OpSetValue:
            node.Values = append(node.Values, ValueOp{
                Name: op.ValueName,
                Type: op.ValueType,
                Data: op.Data,
            })
        case merge.OpDeleteValue:
            node.Values = append(node.Values, ValueOp{
                Name:   op.ValueName,
                Delete: true,
            })
        }
    }

    return root
}

func findChild(parent *Node, nameLower string) *Node {
    for _, c := range parent.Children {
        if c.NameLower == nameLower {
            return c
        }
    }
    return nil
}

func insertSorted(children []*Node, child *Node) []*Node {
    idx, _ := slices.BinarySearchFunc(children, child, func(a, b *Node) int {
        return strings.Compare(a.NameLower, b.NameLower)
    })
    return slices.Insert(children, idx, child)
}
```

- [ ] **Step 5: Run tests**

Run: `cd main && go test ./hive/merge/v2/trie/ -run TestBuild -v`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add hive/merge/v2/trie/
git commit -m "feat(v2): add PatchTrie node types and Build constructor"
```

---

## Task 2: PatchTrie — DFS Iterator

**Files:**
- Create: `hive/merge/v2/trie/walk.go`
- Create: `hive/merge/v2/trie/walk_test.go`

- [ ] **Step 1: Write DFS iterator test**

Create `hive/merge/v2/trie/walk_test.go`:
```go
package trie

import (
    "testing"

    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/stretchr/testify/require"
)

func TestWalk_VisitsInDFSOrder(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"A", "B"}},
        {Type: merge.OpEnsureKey, KeyPath: []string{"A", "C"}},
        {Type: merge.OpEnsureKey, KeyPath: []string{"D"}},
    }
    root := Build(ops)

    var visited []string
    err := Walk(root, func(node *Node, depth int) error {
        visited = append(visited, node.NameLower)
        return nil
    })
    require.NoError(t, err)
    // DFS: A, B, C, D (children sorted, depth-first)
    require.Equal(t, []string{"a", "b", "c", "d"}, visited)
}

func TestWalk_ReportsDepth(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"L1", "L2", "L3"}},
    }
    root := Build(ops)

    var depths []int
    Walk(root, func(node *Node, depth int) error {
        depths = append(depths, depth)
        return nil
    })
    require.Equal(t, []int{1, 2, 3}, depths)
}

func TestWalk_EmptyTrie(t *testing.T) {
    root := Build(nil)
    var count int
    Walk(root, func(node *Node, depth int) error {
        count++
        return nil
    })
    require.Equal(t, 0, count)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd main && go test ./hive/merge/v2/trie/ -run TestWalk -v`
Expected: FAIL — `Walk` not defined.

- [ ] **Step 3: Implement DFS iterator**

Create `hive/merge/v2/trie/walk.go`:
```go
package trie

// Walk performs a depth-first traversal of the trie, calling fn for each
// non-root node. Depth starts at 1 for root's direct children.
// If fn returns an error, the walk stops and returns that error.
func Walk(root *Node, fn func(node *Node, depth int) error) error {
    return walkRecursive(root, 0, fn)
}

func walkRecursive(node *Node, depth int, fn func(*Node, int) error) error {
    // Don't call fn for the root (depth 0) — it's a synthetic container
    if depth > 0 {
        if err := fn(node, depth); err != nil {
            return err
        }
    }
    for _, child := range node.Children {
        if err := walkRecursive(child, depth+1, fn); err != nil {
            return err
        }
    }
    return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd main && go test ./hive/merge/v2/trie/ -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add hive/merge/v2/trie/
git commit -m "feat(v2/trie): add DFS Walk iterator"
```

---

## Task 3: Walk Phase — CursorStack and Annotate

**Files:**
- Create: `hive/merge/v2/walk/cursor.go`
- Create: `hive/merge/v2/walk/cursor_test.go`
- Create: `hive/merge/v2/walk/annotate.go`
- Create: `hive/merge/v2/walk/annotate_test.go`

- [ ] **Step 1: Write cursor stack test**

Create `hive/merge/v2/walk/cursor_test.go` with push/pop/peek tests (same pattern as v1's cursor_test.go but in the v2 package).

- [ ] **Step 2: Implement cursor stack**

Create `hive/merge/v2/walk/cursor.go`:
```go
package walk

// CursorEntry caches parsed hive data at one tree level during the read-only walk.
// All []byte fields are valid for the entire walk (hive is not modified during Phase 2).
type CursorEntry struct {
    NKCellIdx   uint32
    NKPayload   []byte
    ListRef     uint32
    ListPayload []byte
    SubkeyCount uint32
    SKCellIdx   uint32
}

type CursorStack struct {
    entries []CursorEntry
    top     int
}

func NewCursorStack(maxDepth int) *CursorStack {
    return &CursorStack{entries: make([]CursorEntry, maxDepth)}
}

func (s *CursorStack) Push(e CursorEntry) { /* ... */ }
func (s *CursorStack) Pop() CursorEntry   { /* ... */ }
func (s *CursorStack) Peek() *CursorEntry { /* ... */ }
func (s *CursorStack) Depth() int         { return s.top }
```

- [ ] **Step 3: Run cursor tests**

Run: `cd main && go test ./hive/merge/v2/walk/ -run TestCursor -v`
Expected: All PASS.

- [ ] **Step 4: Write annotate test**

Create `hive/merge/v2/walk/annotate_test.go`:
```go
package walk

import (
    "testing"

    "github.com/joshuapare/hivekit/hive"
    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/joshuapare/hivekit/hive/merge/v2/trie"
    "github.com/joshuapare/hivekit/internal/testutil"
    "github.com/stretchr/testify/require"
)

func TestAnnotate_FindsExistingKeys(t *testing.T) {
    hivePath := testutil.RequireSuiteHive(t, "windows-2003-server-system")
    // Alternatively use testutil.SetupTestHive if it works with testing.TB
    h, err := hive.Open(hivePath)
    require.NoError(t, err)
    defer h.Close()

    // Build trie targeting keys known to exist in the test hive
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"Software"}},
    }
    root := trie.Build(ops)

    err = Annotate(h, root)
    require.NoError(t, err)

    // "Software" should be found
    sw := root.Children[0]
    require.True(t, sw.Exists, "Software should exist in hive")
    require.NotEqual(t, uint32(0xFFFFFFFF), sw.CellIdx)
}

func TestAnnotate_MarksNewKeysAsNotExisting(t *testing.T) {
    h, cleanup := testutil.SetupTestHive(t)
    defer cleanup()

    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"CompletelyNewKey12345"}},
    }
    root := trie.Build(ops)

    err := Annotate(h, root)
    require.NoError(t, err)

    node := root.Children[0]
    require.False(t, node.Exists)
}
```

Note: The implementer should check whether `testutil.SetupTestHive` accepts `*testing.T` and adapt accordingly. The test hive from `testutil` is the minimal hive which may not have a "Software" key — use `RequireSuiteHive` for the first test if a real hive is needed, or create a hive via builder for predictable structure.

- [ ] **Step 5: Implement Annotate**

Create `hive/merge/v2/walk/annotate.go`:
```go
package walk

import (
    "fmt"

    "github.com/joshuapare/hivekit/hive"
    "github.com/joshuapare/hivekit/hive/merge/v2/trie"
    "github.com/joshuapare/hivekit/hive/subkeys"
    "github.com/joshuapare/hivekit/internal/format"
)

// Annotate walks the hive following the trie structure and annotates each
// trie node with its hive cell index. This is a READ-ONLY phase — the
// hive is never modified, so all cached []byte slices remain valid.
func Annotate(h *hive.Hive, root *trie.Node) error {
    // Annotate root
    rootRef := h.RootCellOffset()
    rootPayload, err := h.ResolveCellPayload(rootRef)
    if err != nil {
        return fmt.Errorf("resolve root NK: %w", err)
    }
    rootNK, err := hive.ParseNK(rootPayload)
    if err != nil {
        return fmt.Errorf("parse root NK: %w", err)
    }

    root.CellIdx = rootRef
    root.Exists = true
    root.SKCellIdx = rootNK.SecurityOffsetRel()
    root.SubKeyListRef = rootNK.SubkeyListOffsetRel()
    root.SubKeyCount = rootNK.SubkeyCount()

    cursor := NewCursorStack(32)
    cursor.Push(CursorEntry{
        NKCellIdx:   rootRef,
        NKPayload:   rootPayload,
        ListRef:     rootNK.SubkeyListOffsetRel(),
        SubkeyCount: rootNK.SubkeyCount(),
        SKCellIdx:   rootNK.SecurityOffsetRel(),
    })

    return annotateChildren(h, root, cursor)
}

func annotateChildren(h *hive.Hive, parent *trie.Node, cursor *CursorStack) error {
    if len(parent.Children) == 0 {
        return nil
    }

    parentEntry := cursor.Peek()
    if parentEntry == nil || parentEntry.ListRef == format.InvalidOffset {
        // Parent has no subkey list — all children are new
        markSubtreeAsNew(parent)
        return nil
    }

    // Build hash targets for ALL siblings at this level (single scan)
    targets := make(map[uint32][]string, len(parent.Children))
    for _, child := range parent.Children {
        subkeys.AddHashTarget(targets, child.NameLower)
    }

    // Single MatchByHash call finds all needed children
    matches, err := subkeys.MatchByHash(h, parentEntry.ListRef, targets)
    if err != nil {
        return fmt.Errorf("match children of %s: %w", parent.NameLower, err)
    }

    // Build lookup: nameLower → MatchedEntry
    matchMap := make(map[string]subkeys.MatchedEntry, len(matches))
    for _, m := range matches {
        matchMap[m.NameLower] = m
    }

    // Annotate each child
    for _, child := range parent.Children {
        match, found := matchMap[child.NameLower]
        if !found {
            child.Exists = false
            markSubtreeAsNew(child)
            continue
        }

        // Found in hive — read NK and annotate
        child.CellIdx = match.NKRef
        child.Exists = true

        nkPayload, err := h.ResolveCellPayload(match.NKRef)
        if err != nil {
            return fmt.Errorf("resolve NK for %s: %w", child.NameLower, err)
        }
        nk, err := hive.ParseNK(nkPayload)
        if err != nil {
            return fmt.Errorf("parse NK for %s: %w", child.NameLower, err)
        }

        child.SKCellIdx = nk.SecurityOffsetRel()
        child.SubKeyListRef = nk.SubkeyListOffsetRel()
        child.SubKeyCount = nk.SubkeyCount()
        child.ValueListRef = nk.ValueListOffsetRel()
        child.ValueCount = nk.ValueCount()

        // Inherit SK from parent if this node is the ancestor for new children
        if child.SKCellIdx == format.InvalidOffset {
            child.SKCellIdx = parentEntry.SKCellIdx
        }

        // Recurse into children if this trie node has any
        if len(child.Children) > 0 && child.SubKeyListRef != format.InvalidOffset {
            cursor.Push(CursorEntry{
                NKCellIdx:   match.NKRef,
                NKPayload:   nkPayload,
                ListRef:     nk.SubkeyListOffsetRel(),
                SubkeyCount: nk.SubkeyCount(),
                SKCellIdx:   child.SKCellIdx,
            })
            if err := annotateChildren(h, child, cursor); err != nil {
                return err
            }
            cursor.Pop()
        }
    }

    return nil
}

// markSubtreeAsNew marks a node and all its descendants as not existing in the hive.
func markSubtreeAsNew(node *trie.Node) {
    for _, child := range node.Children {
        child.Exists = false
        child.CellIdx = format.InvalidOffset
        markSubtreeAsNew(child)
    }
}
```

- [ ] **Step 6: Run tests**

Run: `cd main && go test ./hive/merge/v2/walk/ -v -timeout 60s`
Expected: All PASS.

- [ ] **Step 7: Commit**

```bash
git add hive/merge/v2/walk/
git commit -m "feat(v2/walk): add CursorStack and Annotate for read-only hive walk"
```

---

## Task 4: Plan Phase — Space Estimation

**Files:**
- Create: `hive/merge/v2/plan/types.go`
- Create: `hive/merge/v2/plan/estimate.go`
- Create: `hive/merge/v2/plan/estimate_test.go`

- [ ] **Step 1: Write space estimation test**

Create `hive/merge/v2/plan/estimate_test.go`:
```go
package plan

import (
    "testing"

    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/joshuapare/hivekit/hive/merge/v2/trie"
    "github.com/joshuapare/hivekit/internal/format"
    "github.com/stretchr/testify/require"
)

func TestEstimate_NewKeysOnly(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"NewKey"}},
    }
    root := trie.Build(ops)
    // Mark as not existing (simulating walk result)
    root.Children[0].Exists = false
    root.Children[0].CellIdx = format.InvalidOffset

    plan, err := Estimate(root)
    require.NoError(t, err)
    require.Greater(t, plan.TotalNewBytes, int32(0))
    require.Equal(t, 1, plan.NewNKCount)
    require.Greater(t, len(plan.Manifest), 0)
}

func TestEstimate_ExistingKeysNoAllocation(t *testing.T) {
    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"Existing"}},
    }
    root := trie.Build(ops)
    // Mark as existing
    root.Children[0].Exists = true
    root.Children[0].CellIdx = 0x100

    plan, err := Estimate(root)
    require.NoError(t, err)
    // No new NK cells needed
    require.Equal(t, 0, plan.NewNKCount)
}

func TestEstimate_EmptyTrie(t *testing.T) {
    root := trie.Build(nil)
    plan, err := Estimate(root)
    require.NoError(t, err)
    require.Equal(t, int32(0), plan.TotalNewBytes)
}
```

- [ ] **Step 2: Implement types**

Create `hive/merge/v2/plan/types.go`:
```go
package plan

import "github.com/joshuapare/hivekit/hive/merge/v2/trie"

// CellKind identifies what type of cell an allocation entry represents.
type CellKind uint8

const (
    CellNK         CellKind = iota
    CellVK
    CellData
    CellSubkeyList
    CellValueList
)

// SpacePlan describes the total space needed for new cells and the
// ordered manifest of allocations.
type SpacePlan struct {
    TotalNewBytes  int32        // total bytes for EnableBumpMode (validated <= MaxInt32)
    NewNKCount     int
    NewVKCount     int
    NewDataCount   int
    ListRebuilds   int
    InPlaceUpdates int
    Manifest       []AllocEntry // ordered for sequential bump writes
}

// AllocEntry is one allocation in the manifest.
type AllocEntry struct {
    Node *trie.Node
    Kind CellKind
    Size int32 // aligned byte size including cell header
}
```

- [ ] **Step 3: Implement Estimate**

Create `hive/merge/v2/plan/estimate.go`:
```go
package plan

import (
    "fmt"
    "math"

    "github.com/joshuapare/hivekit/hive/merge/v2/trie"
    "github.com/joshuapare/hivekit/internal/format"
)

// Estimate walks the annotated trie and computes the total space needed
// for new cells. Returns a SpacePlan with an ordered manifest.
func Estimate(root *trie.Node) (*SpacePlan, error) {
    sp := &SpacePlan{}
    var totalBytes int64 // accumulate in int64, check before narrowing

    if err := trie.Walk(root, func(node *trie.Node, depth int) error {
        if !node.Exists && (node.EnsureKey || node.DeleteKey || len(node.Values) > 0) {
            // New NK cell needed
            nameLen := len(node.Name)
            size := align8(int32(format.CellHeaderSize + 76 + nameLen))
            sp.Manifest = append(sp.Manifest, AllocEntry{Node: node, Kind: CellNK, Size: size})
            sp.NewNKCount++
            totalBytes += int64(size)
        }

        // New VK + data cells for set-value ops on new keys
        for _, v := range node.Values {
            if v.Delete {
                continue
            }
            // VK cell
            vkSize := align8(int32(format.CellHeaderSize + 20 + len(v.Name)))
            sp.Manifest = append(sp.Manifest, AllocEntry{Node: node, Kind: CellVK, Size: vkSize})
            sp.NewVKCount++
            totalBytes += int64(vkSize)

            // Data cell (skip for inline DWORDs: data <= 4 bytes)
            if len(v.Data) > 4 {
                dataSize := align8(int32(format.CellHeaderSize) + int32(len(v.Data)))
                sp.Manifest = append(sp.Manifest, AllocEntry{Node: node, Kind: CellData, Size: dataSize})
                sp.NewDataCount++
                totalBytes += int64(dataSize)
            }
        }

        return nil
    }); err != nil {
        return nil, fmt.Errorf("walk trie: %w", err)
    }

    // Validate no int32 overflow
    if totalBytes > math.MaxInt32 {
        return nil, fmt.Errorf("plan requires %d bytes, exceeds int32 max", totalBytes)
    }
    sp.TotalNewBytes = int32(totalBytes)

    return sp, nil
}

func align8(n int32) int32 {
    return (n + 7) &^ 7
}
```

**Extension required:** This simplified estimator handles new NK + VK + data cells only. The implementer MUST extend it to also estimate:

1. **Subkey list rebuilds:** For each parent node whose children changed (new children added or deleted), allocate a new LH list cell: `align8(4 + 4 + 8 * (existingCount + newCount - deletedCount))`. Walk the trie and for each node with `Exists == true` that has children where `Exists == false`, add a list rebuild entry to the manifest.

2. **Value list rebuilds:** For each existing node with new value ops, allocate a new value list cell: `align8(4 + 4 * (existingValueCount + newValueCount - deletedValueCount))`.

Add corresponding tests: `TestEstimate_SubkeyListRebuild` and `TestEstimate_ValueListRebuild`.

- [ ] **Step 4: Run tests**

Run: `cd main && go test ./hive/merge/v2/plan/ -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add hive/merge/v2/plan/
git commit -m "feat(v2/plan): add space estimation and allocation manifest"
```

---

## Task 5: Write Phase — Cell Writers and Subkey Merge

**Files:**
- Create: `hive/merge/v2/write/cells.go`
- Create: `hive/merge/v2/write/cells_test.go`
- Create: `hive/merge/v2/write/subkeymerge.go`
- Create: `hive/merge/v2/write/subkeymerge_test.go`
- Create: `hive/merge/v2/write/execute.go`
- Create: `hive/merge/v2/write/execute_test.go`

This is the largest task. The implementer should read the spec Section 6 thoroughly before starting. Key sub-components:

1. **Cell writers** — functions that write NK, VK, data, LH list, and value list cell bytes into a `[]byte` payload. These are pure functions (no hive access) and are independently testable.

2. **Two-pointer subkey merge** — merges sorted old `[]RawEntry` with sorted new entries, excluding deleted refs. Also a pure function.

3. **Execute coordinator** — wires cell writers with the annotated trie and bump allocator.

- [ ] **Step 1: Write subkey merge test**

```go
package write

import (
    "testing"

    "github.com/joshuapare/hivekit/hive/subkeys"
    "github.com/stretchr/testify/require"
)

func TestMergeSorted_InsertOnly(t *testing.T) {
    old := []subkeys.RawEntry{
        {NKRef: 100, Hash: 10},
        {NKRef: 200, Hash: 20},
        {NKRef: 300, Hash: 30},
    }
    new := []subkeys.RawEntry{
        {NKRef: 150, Hash: 15}, // insert between 100 and 200
    }
    merged := MergeSortedEntries(old, new, nil)
    require.Len(t, merged, 4)
    require.Equal(t, uint32(100), merged[0].NKRef)
    require.Equal(t, uint32(150), merged[1].NKRef)
    require.Equal(t, uint32(200), merged[2].NKRef)
    require.Equal(t, uint32(300), merged[3].NKRef)
}
```

- [ ] **Step 2: Implement subkey merge**

Create `hive/merge/v2/write/subkeymerge.go` — two-pointer merge of sorted `[]RawEntry` slices, with a delete set for exclusion.

- [ ] **Step 3: Write cell writer tests and implementations**

Pure functions that write REGF cell bytes into `[]byte` payloads. Test each independently.

- [ ] **Step 4: Implement Execute coordinator**

Create `hive/merge/v2/write/execute.go` — walks the trie, calls cell writers via bump allocator, queues in-place updates.

- [ ] **Step 5: Run all write phase tests**

Run: `cd main && go test ./hive/merge/v2/write/ -v -timeout 60s`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add hive/merge/v2/write/
git commit -m "feat(v2/write): add cell writers, subkey merge, and Execute coordinator"
```

---

## Task 6: Flush Phase — Ordered Apply and Delta Checksum

**Files:**
- Create: `hive/merge/v2/flush/apply.go`
- Create: `hive/merge/v2/flush/apply_test.go`
- Create: `hive/merge/v2/flush/checksum.go`
- Create: `hive/merge/v2/flush/checksum_test.go`

- [ ] **Step 1: Write delta checksum test**

```go
package flush

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestDeltaChecksum(t *testing.T) {
    // Compute a reference XOR checksum over 127 DWORDs
    header := make([]byte, 508)
    // Fill with known pattern
    for i := range 127 {
        header[i*4] = byte(i)
    }
    fullXOR := computeFullChecksum(header)

    // Change one field and verify delta matches full recompute
    oldVal := readU32(header, 4)
    newVal := uint32(0xDEADBEEF)
    writeU32(header, 4, newVal)
    fullAfter := computeFullChecksum(header)

    delta := DeltaChecksum(fullXOR, 4, oldVal, newVal)
    require.Equal(t, fullAfter, delta)
}
```

- [ ] **Step 2: Implement delta checksum**

Create `hive/merge/v2/flush/checksum.go`:
```go
package flush

// DeltaChecksum updates a base block XOR checksum by XOR-ing out the old
// value and XOR-ing in the new value for a single 4-byte field.
// Call once per changed field (Sequence1, Sequence2, TimeStamp, Length).
func DeltaChecksum(currentChecksum uint32, fieldOffset int, oldValue, newValue uint32) uint32 {
    if fieldOffset%4 != 0 || fieldOffset >= 508 {
        return currentChecksum // field outside checksum range
    }
    return currentChecksum ^ oldValue ^ newValue
}
```

- [ ] **Step 3: Implement Apply**

Create `hive/merge/v2/flush/apply.go` — groups updates by safety category, sorts within each group by offset, applies in order, finalizes header with delta checksum.

- [ ] **Step 4: Run tests**

Run: `cd main && go test ./hive/merge/v2/flush/ -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add hive/merge/v2/flush/
git commit -m "feat(v2/flush): add ordered flush with delta checksum"
```

---

## Task 7: Pipeline Coordinator — Public API

**Files:**
- Create: `hive/merge/v2/doc.go`
- Create: `hive/merge/v2/result.go`
- Create: `hive/merge/v2/merge.go`
- Create: `hive/merge/v2/merge_test.go`

- [ ] **Step 1: Create types and doc**

Create `hive/merge/v2/doc.go`, `hive/merge/v2/result.go` with `Result`, `PhaseTiming`, `Options` types.

- [ ] **Step 2: Write semantic equivalence test**

Create `hive/merge/v2/merge_test.go`:
```go
package v2

import (
    "context"
    "testing"

    "github.com/joshuapare/hivekit/hive"
    "github.com/joshuapare/hivekit/hive/merge"
    "github.com/joshuapare/hivekit/internal/testutil"
    "github.com/stretchr/testify/require"
)

func TestMerge_SemanticEquivalence_CreateKeys(t *testing.T) {
    // Setup: two copies of the same hive
    h1, cleanup1 := testutil.SetupTestHive(t)
    defer cleanup1()
    h2, cleanup2 := testutil.SetupTestHive(t)
    defer cleanup2()

    ops := []merge.Op{
        {Type: merge.OpEnsureKey, KeyPath: []string{"TestV2", "Child1"}},
        {Type: merge.OpSetValue, KeyPath: []string{"TestV2", "Child1"},
            ValueName: "Val", ValueType: 4, Data: []byte{1, 0, 0, 0}},
    }

    // Apply via v1
    plan := merge.NewPlan()
    plan.Ops = ops
    session, err := merge.NewSessionForPlan(context.Background(), h1, plan, merge.Options{})
    require.NoError(t, err)
    _, err = session.ApplyWithTx(context.Background(), plan)
    require.NoError(t, err)

    // Apply via v2
    _, err = Merge(context.Background(), h2, ops, Options{})
    require.NoError(t, err)

    // Compare: both hives should have the same keys and values.
    // Use hive/walker to walk both hives and collect all key paths + values,
    // then compare the collected trees.
    assertSemanticallyEqual(t, h1, h2)
}

// assertSemanticallyEqual walks both hives from root and verifies they have
// identical key trees and values. Cell offsets may differ (v2 bump allocation
// produces different layout than v1), but the logical content must match.
//
// Implementation approach:
// 1. Walk h1 from root using hive/walker, collect map[string][]ValueEntry
//    where key is the full normalized path and values are {name, type, data}
// 2. Walk h2 identically
// 3. Compare the two maps — same keys, same values
//
// This helper should be implemented as part of this task. Use the existing
// walker.WalkKeys or equivalent to enumerate all keys, and walker.WalkValues
// to enumerate values per key. Compare by building sorted slices and
// using require.Equal.
func assertSemanticallyEqual(t *testing.T, h1, h2 *hive.Hive) {
    t.Helper()
    tree1 := collectHiveTree(t, h1)
    tree2 := collectHiveTree(t, h2)
    require.Equal(t, tree1, tree2, "hives should be semantically equal")
}
```

- [ ] **Step 3: Implement Merge coordinator**

Create `hive/merge/v2/merge.go` — wires all 5 phases together as described in the spec Section 8.

- [ ] **Step 4: Run tests**

Run: `cd main && go test ./hive/merge/v2/ -v -timeout 120s`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add hive/merge/v2/
git commit -m "feat(v2): add Merge pipeline coordinator with semantic equivalence tests"
```

---

## Task 8: Activate V2 Benchmarks

The benchmark scaffold was created in Task 0 with `b.Skip`. Now that the v2 API exists (Task 7), remove the skip and connect to `v2.Merge`.

**Files:**
- Modify: `tests/benchmark/merge_v2_e2e_bench_test.go`

- [ ] **Step 1: Remove skip and connect to v2.Merge**

Update the benchmark to call `v2.Merge(ctx, h, ops, v2.Options{})` instead of skipping. Follow the same pattern as the v1 benchmark for timer management and metrics.

- [ ] **Step 2: Run smoke test**

Run: `cd main && go test ./tests/benchmark/ -bench 'BenchmarkMergeV2E2E/small-flat/create-sparse' -benchtime=1x -timeout 120s`
Expected: Benchmark runs and produces output with ns/op, B/op, allocs/op.

- [ ] **Step 3: Run head-to-head comparison**

```bash
# V1 benchmarks
go test ./tests/benchmark/ -bench 'BenchmarkMergeE2E' -benchmem -count=6 -timeout=3600s > /tmp/v1.txt

# V2 benchmarks
go test ./tests/benchmark/ -bench 'BenchmarkMergeV2E2E' -benchmem -count=6 -timeout=3600s > /tmp/v2.txt

# Compare (manually align benchmark names for benchstat)
benchstat /tmp/v1.txt /tmp/v2.txt
```

- [ ] **Step 4: Commit**

```bash
git add tests/benchmark/merge_v2_e2e_bench_test.go
git commit -m "feat(benchmark): add V2 merge engine E2E benchmarks for head-to-head comparison"
```

---

## Task 9: Full Validation

- [ ] **Step 1: Run full test suite**

Run: `cd main && go test ./... -timeout 300s`
Expected: All packages pass (v1 and v2).

- [ ] **Step 2: Run head-to-head benchmarks**

Run the full benchmark matrix for both v1 and v2. Compare with benchstat. Document results.

- [ ] **Step 3: Document results**

Create `docs/v2-merge-engine-benchmark-results.md` with:
- benchstat output (v1 vs v2)
- Per-phase timing breakdown from `PhaseTiming`
- Comparison against theoretical optimum
- Recommendations for further optimization

- [ ] **Step 4: Commit results**

```bash
git add docs/v2-merge-engine-benchmark-results.md
git commit -m "docs: add v2 merge engine benchmark results"
```

---

## Parallelism Guide

```
Task 0: Benchmark scaffold            ← First (skip-marked until Task 7)
Task 1-2: PatchTrie (trie/)           ← No dependencies, start after Task 0
Task 3: Walk phase (walk/)            ← Depends on trie/
Task 4: Plan phase (plan/)            ← Depends on trie/ (can start after Task 2)
Task 5: Write phase (write/)          ← Depends on trie/, plan/, alloc/bump.go (existing)
Task 6: Flush phase (flush/)          ← Independent (pure functions)
Task 7: Coordinator (v2/)             ← Depends on all above
Task 8: Activate V2 benchmarks        ← Depends on Task 7 (removes skip from Task 0)
Task 9: Full Validation               ← Depends on all above
```

Tasks 1-2 and Task 6 can be developed in parallel. Tasks 3 and 4 can also be parallelized since they depend only on the trie types (not each other).
