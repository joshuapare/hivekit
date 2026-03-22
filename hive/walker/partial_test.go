package walker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	hivepkg "github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
)

// setupLargeTestHive opens the large test hive directly (without testutil dependency).
// This hive has 1711 NKs and 3633 VKs with structure:
//
//	root -> A -> A giant -> A giant elephant, ...
//	root -> Another -> Another giant -> ...
//	root -> The -> The giant -> ...
func setupLargeTestHive(t *testing.T) (*hivepkg.Hive, func()) {
	t.Helper()

	// Try multiple relative paths to find the test hive
	candidates := []string{
		"testdata/large",
		"../../testdata/large",
		"../../../testdata/large",
	}

	var srcPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			srcPath = c
			break
		}
	}
	if srcPath == "" {
		t.Skip("Large test hive not found")
	}

	// Copy to temp dir so tests don't mutate the original
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read test hive: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "test-hive")
	if err := os.WriteFile(dst, src, 0644); err != nil {
		t.Fatalf("write temp hive: %v", err)
	}

	h, err := hivepkg.Open(dst)
	if err != nil {
		t.Fatalf("open hive: %v", err)
	}

	return h, func() { h.Close() }
}

// Test_BuildPartial_IndexesCorrectPaths verifies that BuildPartial indexes
// the correct paths and that those paths are queryable via index.WalkPath.
func Test_BuildPartial_IndexesCorrectPaths(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	// Use paths known to exist in the large test hive
	paths := [][]string{
		{"A"},
		{"A", "A giant"},
		{"The"},
		{"The", "The giant"},
	}

	idx, stats, err := BuildPartial(h, paths)
	if err != nil {
		t.Fatalf("BuildPartial failed: %v", err)
	}

	t.Logf("Partial build stats: NK=%d, VK=%d, Paths=%d, TrieNodes=%d",
		stats.NKIndexed, stats.VKIndexed, stats.PathsRequested, stats.TrieNodes)

	if idx == nil {
		t.Fatal("Expected non-nil index")
	}

	rootOffset := h.RootCellOffset()

	// Verify that the indexed paths are queryable
	for _, path := range paths {
		offset, ok := index.WalkPath(idx, rootOffset, path...)
		if !ok {
			t.Errorf("Path %v should be found in partial index", path)
			continue
		}
		if offset == 0 {
			t.Errorf("Path %v returned zero offset", path)
		}
	}

	// NKIndexed includes root + the 4 unique nodes from paths
	// Trie: root -> {A -> "A giant", The -> "The giant"}
	// NK indexed: root(1) + A(1) + "A giant"(1) + The(1) + "The giant"(1) = 5
	if stats.NKIndexed < 5 {
		t.Errorf("Expected at least 5 NK indexed (root + 4 path nodes), got %d", stats.NKIndexed)
	}
}

// Test_BuildPartial_FewerThanFull verifies that partial build indexes fewer
// cells than a full index build.
func Test_BuildPartial_FewerThanFull(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	// Build partial index with a few paths
	paths := [][]string{
		{"A", "A giant"},
		{"The", "The giant"},
	}

	partialIdx, partialStats, err := BuildPartial(h, paths)
	if err != nil {
		t.Fatalf("BuildPartial failed: %v", err)
	}

	// Build full index for comparison
	builder := NewIndexBuilder(h, 10000, 10000)
	fullIdx, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Full Build failed: %v", err)
	}

	fullStats := fullIdx.Stats()
	partialIdxStats := partialIdx.Stats()

	t.Logf("Full index: NK=%d, VK=%d", fullStats.NKCount, fullStats.VKCount)
	t.Logf("Partial index: NK=%d, VK=%d (from stats: NK=%d, VK=%d)",
		partialIdxStats.NKCount, partialIdxStats.VKCount,
		partialStats.NKIndexed, partialStats.VKIndexed)

	// Partial should have significantly fewer NKs than full
	if partialIdxStats.NKCount >= fullStats.NKCount {
		t.Errorf("Partial NK count (%d) should be less than full NK count (%d)",
			partialIdxStats.NKCount, fullStats.NKCount)
	}

	// Partial should be at least 50% smaller (in practice much more)
	if partialIdxStats.NKCount > fullStats.NKCount/2 {
		t.Errorf("Partial NK count (%d) should be less than half of full NK count (%d)",
			partialIdxStats.NKCount, fullStats.NKCount)
	}
}

// Test_BuildPartial_EmptyPaths verifies that BuildPartial with empty paths
// returns an index containing only the root.
func Test_BuildPartial_EmptyPaths(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	idx, stats, err := BuildPartial(h, nil)
	if err != nil {
		t.Fatalf("BuildPartial with empty paths failed: %v", err)
	}

	if idx == nil {
		t.Fatal("Expected non-nil index even for empty paths")
	}

	// Should have indexed just the root
	if stats.NKIndexed != 1 {
		t.Errorf("Expected 1 NK indexed (root only), got %d", stats.NKIndexed)
	}

	if stats.PathsRequested != 0 {
		t.Errorf("Expected 0 paths requested, got %d", stats.PathsRequested)
	}

	// Root should be findable
	rootOffset := h.RootCellOffset()
	idxStats := idx.Stats()
	if idxStats.NKCount < 1 {
		t.Errorf("Expected at least 1 NK in index, got %d", idxStats.NKCount)
	}

	// Verify root is indexed
	_, ok := idx.GetNK(rootOffset, "")
	if !ok {
		t.Error("Root NK should be indexed even with empty paths")
	}
}

// Test_BuildPartial_CaseInsensitive verifies that the partial index handles
// case-insensitive path lookups correctly.
func Test_BuildPartial_CaseInsensitive(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	// Use mixed case in paths
	paths := [][]string{
		{"a"},
		{"A", "A giant"},
	}

	idx, _, err := BuildPartial(h, paths)
	if err != nil {
		t.Fatalf("BuildPartial failed: %v", err)
	}

	rootOffset := h.RootCellOffset()

	// Should find the path regardless of case used in query
	_, ok := index.WalkPath(idx, rootOffset, "A")
	if !ok {
		t.Error("Should find 'A' with case-insensitive lookup")
	}

	_, ok = index.WalkPath(idx, rootOffset, "a")
	if !ok {
		t.Error("Should find 'a' with case-insensitive lookup")
	}
}

// Test_BuildPartial_OverlappingPaths verifies that overlapping paths
// (where one is a prefix of another) are handled correctly.
func Test_BuildPartial_OverlappingPaths(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	paths := [][]string{
		{"A"},
		{"A", "A giant"},
		{"A", "A giant"},   // duplicate
		{"The", "The giant"},
	}

	idx, stats, err := BuildPartial(h, paths)
	if err != nil {
		t.Fatalf("BuildPartial failed: %v", err)
	}

	rootOffset := h.RootCellOffset()

	// All unique paths should be findable
	for _, path := range [][]string{
		{"A"},
		{"A", "A giant"},
		{"The", "The giant"},
	} {
		_, ok := index.WalkPath(idx, rootOffset, path...)
		if !ok {
			t.Errorf("Path %v should be found in partial index", path)
		}
	}

	// Trie should have deduplicated:
	// root -> {a -> "a giant", the -> "the giant"}
	// 5 trie nodes: root + a + "a giant" + the + "the giant"
	if stats.TrieNodes != 5 {
		t.Errorf("Expected 5 trie nodes (root + 4 unique components), got %d", stats.TrieNodes)
	}
}

// Test_BuildPartial_NonExistentPath verifies that BuildPartial handles
// paths that don't exist in the hive gracefully.
func Test_BuildPartial_NonExistentPath(t *testing.T) {
	h, cleanup := setupLargeTestHive(t)
	defer cleanup()

	paths := [][]string{
		{"A"},              // exists
		{"NonExistent"},    // does not exist
		{"A", "A giant"},   // exists
		{"A", "NoSuchKey"}, // does not exist
	}

	idx, stats, err := BuildPartial(h, paths)
	if err != nil {
		t.Fatalf("BuildPartial failed: %v", err)
	}

	rootOffset := h.RootCellOffset()

	// Existing paths should be findable
	_, ok := index.WalkPath(idx, rootOffset, "A")
	if !ok {
		t.Error("Path ['A'] should be found")
	}
	_, ok = index.WalkPath(idx, rootOffset, "A", "A giant")
	if !ok {
		t.Error("Path ['A', 'A giant'] should be found")
	}

	// Non-existent paths should not be found
	_, ok = index.WalkPath(idx, rootOffset, "NonExistent")
	if ok {
		t.Error("Path ['NonExistent'] should NOT be found")
	}

	t.Logf("Stats: NK=%d, VK=%d", stats.NKIndexed, stats.VKIndexed)
}

// Test_buildTrie verifies the trie construction logic.
func Test_buildTrie(t *testing.T) {
	paths := [][]string{
		{"a", "b", "c"},
		{"a", "b", "d"},
		{"a", "e"},
		{"f"},
	}

	root, nodeCount := buildTrie(paths)

	// Expected trie:
	// root -> a -> b -> c
	//              b -> d
	//         a -> e
	// root -> f
	// Total: root + a + b + c + d + e + f = 7
	if nodeCount != 7 {
		t.Errorf("Expected 7 trie nodes, got %d", nodeCount)
	}

	// Verify structure
	if len(root.children) != 2 {
		t.Errorf("Root should have 2 children (a, f), got %d", len(root.children))
	}

	aNode := root.children["a"]
	if aNode == nil {
		t.Fatal("Expected 'a' child of root")
	}
	if len(aNode.children) != 2 {
		t.Errorf("'a' should have 2 children (b, e), got %d", len(aNode.children))
	}

	bNode := aNode.children["b"]
	if bNode == nil {
		t.Fatal("Expected 'b' child of 'a'")
	}
	if len(bNode.children) != 2 {
		t.Errorf("'b' should have 2 children (c, d), got %d", len(bNode.children))
	}
}

// Test_buildTrie_CaseFolding verifies that trie construction lowercases names.
func Test_buildTrie_CaseFolding(t *testing.T) {
	paths := [][]string{
		{"Software", "Microsoft"},
		{"SOFTWARE", "MICROSOFT"},
		{"software", "microsoft"},
	}

	root, nodeCount := buildTrie(paths)

	// All three paths should collapse to one: root -> software -> microsoft
	// Total: root + software + microsoft = 3
	if nodeCount != 3 {
		t.Errorf("Expected 3 trie nodes after case folding, got %d", nodeCount)
	}

	if len(root.children) != 1 {
		t.Errorf("Root should have 1 child after case folding, got %d", len(root.children))
	}

	softNode := root.children["software"]
	if softNode == nil {
		t.Fatal("Expected 'software' child of root")
	}
}
