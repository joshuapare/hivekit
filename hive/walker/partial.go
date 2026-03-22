package walker

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// PartialBuildStats reports metrics from a partial index build.
type PartialBuildStats struct {
	// NKIndexed is the number of NK cells added to the index.
	NKIndexed int
	// VKIndexed is the number of VK cells added to the index.
	VKIndexed int
	// PathsRequested is the number of unique paths provided.
	PathsRequested int
	// TrieNodes is the number of trie nodes built from the paths.
	TrieNodes int
}

// trieNode is a node in a simple prefix trie built from plan paths.
// Each node represents one path component (e.g., "ControlSet001").
type trieNode struct {
	children map[string]*trieNode // lowercased name -> child node
}

// newTrieNode creates a new empty trie node.
func newTrieNode() *trieNode {
	return &trieNode{
		children: make(map[string]*trieNode),
	}
}

// buildTrie constructs a prefix trie from a list of key paths.
// Each path is a slice of component names (e.g., ["ControlSet001", "Services", "LanmanServer"]).
// Returns the root trie node and the total number of trie nodes created.
func buildTrie(paths [][]string) (*trieNode, int) {
	root := newTrieNode()
	nodeCount := 1 // count the root

	for _, path := range paths {
		current := root
		for _, component := range path {
			lower := strings.ToLower(component)
			child, exists := current.children[lower]
			if !exists {
				child = newTrieNode()
				current.children[lower] = child
				nodeCount++
			}
			current = child
		}
	}

	return root, nodeCount
}

// BuildPartial builds an index containing only the subtrees touched by the
// given paths. Instead of walking the entire hive (which takes 50-200ms for
// large hives), it builds a trie from the paths and only descends into
// branches that match trie nodes.
//
// This produces an index compatible with the merge session's use of
// index.WalkPath and index.GetNK. Paths not in the plan will return
// "not found" from the index, which is acceptable since the merge engine
// only queries paths from the plan.
//
// The paths parameter is a list of key paths, where each path is a slice of
// component names relative to the hive root (e.g., [][]string{{"ControlSet001", "Services"}}).
//
// Returns the built index, build statistics, and any error.
func BuildPartial(h *hive.Hive, paths [][]string) (index.Index, PartialBuildStats, error) {
	var stats PartialBuildStats
	stats.PathsRequested = len(paths)

	if len(paths) == 0 {
		// Return an empty index with just the root indexed
		idx := index.NewNumericIndex(16, 16)
		rootOffset := h.RootCellOffset()
		idx.AddNK(rootOffset, "", rootOffset)
		stats.NKIndexed = 1
		return idx, stats, nil
	}

	// Build trie from paths
	trie, trieNodeCount := buildTrie(paths)
	stats.TrieNodes = trieNodeCount

	// Estimate index capacity from path count
	// Each path contributes ~depth nodes, but many overlap at the top levels
	nkCap := trieNodeCount + 16
	vkCap := nkCap * 2 // rough estimate

	idx := index.NewNumericIndex(nkCap, vkCap)

	// Index the root NK
	rootOffset := h.RootCellOffset()
	idx.AddNK(rootOffset, "", rootOffset)
	stats.NKIndexed++

	// Descend from root into branches that match the trie
	if err := partialDescend(h, idx, rootOffset, trie, &stats); err != nil {
		return nil, stats, fmt.Errorf("partial index build: %w", err)
	}

	return idx, stats, nil
}

// partialDescend walks into the children of parentOffset, only visiting
// children whose names match the trie node's children. For each match,
// it adds the child NK to the index and recurses if the trie continues deeper.
func partialDescend(h *hive.Hive, idx *index.NumericIndex, parentOffset uint32, node *trieNode, stats *PartialBuildStats) error {
	if len(node.children) == 0 {
		return nil // trie ends here, no need to descend further
	}

	// Get the parent NK to find its subkey list
	payload, err := h.ResolveCellPayload(parentOffset)
	if err != nil {
		return fmt.Errorf("resolve NK at 0x%X: %w", parentOffset, err)
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return fmt.Errorf("parse NK at 0x%X: %w", parentOffset, err)
	}

	if nk.SubkeyCount() == 0 {
		return nil // no children to search
	}

	subkeyListOffset := nk.SubkeyListOffsetRel()
	if subkeyListOffset == format.InvalidOffset {
		return nil // invalid subkey list
	}

	// Build targets map for MatchByHash: LH hash -> []lowercase names
	targets := make(map[uint32][]string, len(node.children))
	for nameLower := range node.children {
		subkeys.AddHashTarget(targets, nameLower)
	}

	// Use MatchByHash to efficiently find only the children we need
	matched, err := subkeys.MatchByHash(h, subkeyListOffset, targets)
	if err != nil {
		return fmt.Errorf("match children at 0x%X: %w", parentOffset, err)
	}

	// Index each matched child and recurse
	for _, m := range matched {
		// Add the child NK to the index under its parent
		childTrieNode := node.children[m.NameLower]
		if childTrieNode == nil {
			continue // shouldn't happen, but be safe
		}

		// Use the hash-fast path for adding to index
		fnvHash := index.Fnv32LowerBytes([]byte(m.NameLower))
		idx.AddNKHashFast(parentOffset, fnvHash, m.NKRef)
		stats.NKIndexed++

		// Index values for this NK if the trie terminates here
		// (i.e., this is a leaf in the plan's path set)
		// Also index values if the trie continues (the merge engine may
		// need to set values at intermediate nodes too)
		if err := partialIndexValues(h, idx, m.NKRef, stats); err != nil {
			// Non-fatal: skip values for this key
			continue
		}

		// Recurse into subtree if trie continues
		if len(childTrieNode.children) > 0 {
			if err := partialDescend(h, idx, m.NKRef, childTrieNode, stats); err != nil {
				return err
			}
		}
	}

	return nil
}

// partialIndexValues indexes all values under the given NK cell.
// This is needed because merge operations may set values at any node in the plan path.
func partialIndexValues(h *hive.Hive, idx *index.NumericIndex, nkOffset uint32, stats *PartialBuildStats) error {
	payload, err := h.ResolveCellPayload(nkOffset)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	valueCount := nk.ValueCount()
	if valueCount == 0 {
		return nil
	}

	valueListOffset := nk.ValueListOffsetRel()
	if valueListOffset == format.InvalidOffset {
		return nil
	}

	// Resolve value list cell
	vlPayload, err := h.ResolveCellPayload(valueListOffset)
	if err != nil {
		return err
	}

	// Value list is an array of VK offsets (4 bytes each)
	needed := int(valueCount) * format.DWORDSize
	if needed > len(vlPayload) {
		return nil // truncated, skip
	}

	for i := range valueCount {
		off := int(i) * format.DWORDSize
		vkOffset, readErr := format.CheckedReadU32(vlPayload, off)
		if readErr != nil {
			continue
		}
		if vkOffset == 0 || vkOffset == format.InvalidOffset {
			continue
		}

		// Resolve VK cell to get name
		vkPayload, resolveErr := h.ResolveCellPayload(vkOffset)
		if resolveErr != nil {
			continue
		}
		if len(vkPayload) < format.VKFixedHeaderSize {
			continue
		}

		nameLen := format.ReadU16(vkPayload, format.VKNameLenOffset)
		flags := format.ReadU16(vkPayload, format.VKFlagsOffset)

		if len(vkPayload) < format.VKFixedHeaderSize+int(nameLen) {
			continue
		}

		nameBytes := vkPayload[format.VKFixedHeaderSize : format.VKFixedHeaderSize+nameLen]

		if flags&0x0001 != 0 {
			// Compressed (ASCII) - use hash fast path
			hash := index.Fnv32LowerBytes(nameBytes)
			idx.AddVKHashFast(nkOffset, hash, vkOffset)
		} else {
			// UTF-16LE - decode and add via string path
			nameLower := decodeUTF16LELower(nameBytes)
			idx.AddVKLower(nkOffset, nameLower, vkOffset)
		}
		stats.VKIndexed++
	}

	return nil
}
