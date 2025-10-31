package edit

import (
	"sort"
	"strings"
)

// changeIndex provides efficient change detection for paths.
// It builds a sorted index of all changed paths to enable O(log n) lookups
// and O(log n + k) subtree queries (where k is the number of matching paths).
type changeIndex struct {
	// Sorted lists of normalized paths for efficient lookup
	createdPaths []string // paths of created keys
	deletedPaths []string // paths of deleted keys
	valuePaths   []string // paths with value changes (set or deleted)

	// Maps for exact lookups (O(1))
	createdSet map[string]bool
	deletedSet map[string]bool
	valueSet   map[string]bool

	// All changed paths (union of above), sorted for prefix matching
	allPaths []string
}

// normalizePathLower normalizes and lowercases a path for case-insensitive comparison.
func normalizePathLower(p string) string {
	return strings.ToLower(normalizePath(p))
}

// buildChangeIndex constructs an index from transaction changes.
// This should be called once during transaction commit and cached.
func buildChangeIndex(tx *transaction) *changeIndex {
	idx := &changeIndex{
		createdSet: make(map[string]bool),
		deletedSet: make(map[string]bool),
		valueSet:   make(map[string]bool),
	}

	// Collect created keys (use lowercase for case-insensitive lookups)
	for path, node := range tx.createdKeys {
		if !node.exists {
			pathLower := normalizePathLower(path)
			idx.createdSet[pathLower] = true
			idx.createdPaths = append(idx.createdPaths, pathLower)
		}
	}

	// Collect deleted keys (use lowercase for case-insensitive lookups)
	for path := range tx.deletedKeys {
		pathLower := normalizePathLower(path)
		idx.deletedSet[pathLower] = true
		idx.deletedPaths = append(idx.deletedPaths, pathLower)
	}

	// Collect paths with value changes (set or deleted) (use lowercase)
	for vk := range tx.setValues {
		pathLower := normalizePathLower(vk.path)
		if !idx.valueSet[pathLower] {
			idx.valueSet[pathLower] = true
			idx.valuePaths = append(idx.valuePaths, pathLower)
		}
	}
	for vk := range tx.deletedVals {
		pathLower := normalizePathLower(vk.path)
		if !idx.valueSet[pathLower] {
			idx.valueSet[pathLower] = true
			idx.valuePaths = append(idx.valuePaths, pathLower)
		}
	}

	// Sort all path lists for binary search
	sort.Strings(idx.createdPaths)
	sort.Strings(idx.deletedPaths)
	sort.Strings(idx.valuePaths)

	// Build unified sorted list of all changed paths
	allSet := make(map[string]bool)
	for path := range idx.createdSet {
		allSet[path] = true
	}
	for path := range idx.deletedSet {
		allSet[path] = true
	}
	for path := range idx.valueSet {
		allSet[path] = true
	}

	idx.allPaths = make([]string, 0, len(allSet))
	for path := range allSet {
		idx.allPaths = append(idx.allPaths, path)
	}
	sort.Strings(idx.allPaths)

	return idx
}

// HasExact returns true if the exact path has any changes.
// This is an O(1) operation using map lookups.
func (idx *changeIndex) HasExact(path string) bool {
	pathLower := normalizePathLower(path)
	return idx.createdSet[pathLower] || idx.deletedSet[pathLower] || idx.valueSet[pathLower]
}

// HasSubtree returns true if the path or any of its descendants have changes.
// This is an O(log n + k) operation where k is the number of matching paths.
// Uses binary search on sorted path list and prefix matching.
func (idx *changeIndex) HasSubtree(path string) bool {
	pathLower := normalizePathLower(path)

	// Check if the path itself has changes
	if idx.HasExact(path) {
		return true
	}

	// Build the prefix to search for descendants
	// For path "A\B", descendants will be "A\B\*"
	var prefix string
	if pathLower == "" {
		// Root path - all paths are descendants
		return len(idx.allPaths) > 0
	} else {
		prefix = pathLower + "\\"
	}

	// Binary search for the first path that could be a descendant
	// Find the insertion point for prefix in sorted list
	i := sort.Search(len(idx.allPaths), func(i int) bool {
		return idx.allPaths[i] >= prefix
	})

	// Check if any paths starting at position i have the prefix
	if i < len(idx.allPaths) && strings.HasPrefix(idx.allPaths[i], prefix) {
		return true
	}

	return false
}

// HasCreated returns true if the exact path was created.
func (idx *changeIndex) HasCreated(path string) bool {
	pathLower := normalizePathLower(path)
	return idx.createdSet[pathLower]
}

// HasDeleted returns true if the exact path was deleted.
func (idx *changeIndex) HasDeleted(path string) bool {
	pathLower := normalizePathLower(path)
	return idx.deletedSet[pathLower]
}

// HasValueChanges returns true if the exact path has value changes.
func (idx *changeIndex) HasValueChanges(path string) bool {
	pathLower := normalizePathLower(path)
	return idx.valueSet[pathLower]
}

// ChangeCount returns the total number of changed paths.
func (idx *changeIndex) ChangeCount() int {
	return len(idx.allPaths)
}
