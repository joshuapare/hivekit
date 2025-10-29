package keytree

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// LoadEntireTree loads the complete tree structure upfront for a hive
func LoadEntireTree(hivePath string) tea.Cmd {
	return func() tea.Msg {
		// Open reader
		r, err := reader.Open(hivePath, hive.OpenOptions{})
		if err != nil {
			return ErrMsg{fmt.Errorf("failed to open hive: %w", err)}
		}

		// Get root node
		root, err := r.Root()
		if err != nil {
			r.Close()
			return ErrMsg{fmt.Errorf("failed to get root: %w", err)}
		}

		// Recursively build entire tree
		var allItems []Item
		err = buildTreeRecursive(r, root, "", 0, "", &allItems, true) // true = isRoot
		if err != nil {
			r.Close()
			return ErrMsg{fmt.Errorf("failed to build tree: %w", err)}
		}

		return TreeLoadedMsg{
			Items:  allItems,
			Reader: r,
		}
	}
}

// buildTreeRecursive recursively builds the complete tree structure
// When isRoot=true, the node itself is not added (only its children are processed)
func buildTreeRecursive(
	r hive.Reader,
	nodeID hive.NodeID,
	parentPath string,
	depth int,
	parent string,
	items *[]Item,
	isRoot bool,
) error {
	// Get node metadata
	meta, err := r.StatKey(nodeID)
	if err != nil {
		return err
	}

	// Build current path
	currentPath := meta.Name
	if parentPath != "" {
		currentPath = parentPath + "\\" + meta.Name
	}

	// Don't add root node itself, only its children
	if !isRoot {
		item := Item{
			NodeID:      nodeID,
			Path:        currentPath,
			Name:        meta.Name,
			Depth:       depth,
			HasChildren: meta.SubkeyN > 0,
			SubkeyCount: meta.SubkeyN,
			ValueCount:  meta.ValueN,
			LastWrite:   meta.LastWrite,
			Expanded:    false,
			Parent:      parent,
		}
		*items = append(*items, item)
	}

	// Recursively process children
	if meta.SubkeyN > 0 {
		childIDs, err := r.Subkeys(nodeID)
		if err != nil {
			return err
		}

		for _, childID := range childIDs {
			// For root's children, they become top-level items (depth 0, no parent, empty parent path)
			childDepth := depth + 1
			childParent := currentPath
			childParentPath := currentPath
			if isRoot {
				childDepth = 0       // Root's children are top-level
				childParent = ""     // Root's children have no parent
				childParentPath = "" // Root's children don't get root prefix in path
			}

			err := buildTreeRecursive(
				r,
				childID,
				childParentPath,
				childDepth,
				childParent,
				items,
				false,
			)
			if err != nil {
				// Log error but continue with other children
				fmt.Fprintf(os.Stderr, "[WARN] Failed to load child: %v\n", err)
			}
		}
	}

	return nil
}

// LoadChildren loads children for a specific key path
func LoadChildren(hivePath string, path string) tea.Cmd {
	return func() tea.Msg {
		keys, err := hive.ListKeys(hivePath, path, false, 1)
		if err != nil {
			return ErrMsg{
				fmt.Errorf(
					"failed to load children for path %q in hive %q: %w",
					path,
					hivePath,
					err,
				),
			}
		}

		// Convert to message type
		msgKeys := make([]KeyInfo, len(keys))
		for i, k := range keys {
			msgKeys[i] = KeyInfo{
				Path:      k.Path,
				Name:      k.Name,
				SubkeyN:   k.SubkeyN,
				ValueN:    k.ValueN,
				LastWrite: k.LastWrite,
			}
		}
		return ChildKeysLoadedMsg{Parent: path, Keys: msgKeys}
	}
}
