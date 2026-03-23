package trie_test

import (
	"errors"
	"testing"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
)

// TestWalk_VisitsInDFSOrder builds a trie with A/B, A/C, and D, then verifies
// that Walk visits nodes in DFS order: a, b, c, d.
func TestWalk_VisitsInDFSOrder(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"A", "B"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"A", "C"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"D"}},
	}
	root := trie.Build(ops)

	var visited []string
	err := trie.Walk(root, func(node *trie.Node, depth int) error {
		visited = append(visited, node.NameLower)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b", "c", "d"}
	if len(visited) != len(expected) {
		t.Fatalf("expected %d visits, got %d: %v", len(expected), len(visited), visited)
	}
	for i, name := range expected {
		if visited[i] != name {
			t.Errorf("visit[%d]: expected %q, got %q", i, name, visited[i])
		}
	}
}

// TestWalk_ReportsDepth builds a trie with L1/L2/L3 and verifies that depths
// are reported as [1, 2, 3].
func TestWalk_ReportsDepth(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"L1", "L2", "L3"}},
	}
	root := trie.Build(ops)

	var depths []int
	err := trie.Walk(root, func(node *trie.Node, depth int) error {
		depths = append(depths, depth)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []int{1, 2, 3}
	if len(depths) != len(expected) {
		t.Fatalf("expected %d depth entries, got %d: %v", len(expected), len(depths), depths)
	}
	for i, d := range expected {
		if depths[i] != d {
			t.Errorf("depths[%d]: expected %d, got %d", i, d, depths[i])
		}
	}
}

// TestWalk_EmptyTrie verifies that Walk on a trie built from nil ops visits
// zero nodes.
func TestWalk_EmptyTrie(t *testing.T) {
	root := trie.Build(nil)

	count := 0
	err := trie.Walk(root, func(node *trie.Node, depth int) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 visits, got %d", count)
	}
}

// TestWalk_ErrorStopsWalk verifies that when fn returns an error, Walk stops
// immediately and propagates the error.
func TestWalk_ErrorStopsWalk(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"A", "B"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"A", "C"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"D"}},
	}
	root := trie.Build(ops)

	sentinel := errors.New("stop here")
	count := 0
	err := trie.Walk(root, func(node *trie.Node, depth int) error {
		count++
		if node.NameLower == "a" {
			return sentinel
		}
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if count != 1 {
		t.Errorf("expected fn to be called exactly once before stopping, got %d", count)
	}
}
