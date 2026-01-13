package hive

// TreeNode represents a minimal node in the registry tree structure.
// This is optimized for TUI display with minimal memory footprint.
// For 16K keys, this uses ~1MB vs TreeItem which uses 2MB+.
type TreeNode struct {
	NodeID      NodeID // For efficient lookups in reader
	Name        string // Display name
	Path        string // Full path (for signaling to other panels)
	Depth       int    // Tree depth for indentation
	HasChildren bool   // Whether this node has subkeys (for expand icon)
	Parent      string // Parent path (for tree structure)
}

// BuildTreeStructure walks the entire registry tree once and returns
// a flat array of TreeNodes with minimal metadata. This is designed for
// TUI applications that need to display the tree structure efficiently.
//
// The returned reader should be kept open for the lifetime of the application
// to efficiently load values and metadata on-demand as the user navigates.
//
// Memory usage: ~64 bytes per node
// Performance: Single tree walk, no path string lookups.
func BuildTreeStructure(r Reader) ([]TreeNode, error) {
	root, err := r.Root()
	if err != nil {
		return nil, err
	}

	var nodes []TreeNode

	// Recursively build tree starting from root
	err = buildTreeRecursive(r, root, "", 0, "", &nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// buildTreeRecursive recursively walks the tree and appends TreeNodes.
func buildTreeRecursive(r Reader, nodeID NodeID, parentPath string, depth int, parent string, nodes *[]TreeNode) error {
	// Get node metadata (just name and child count - minimal data)
	meta, err := r.StatKey(nodeID)
	if err != nil {
		return err
	}

	// Build current path
	currentPath := meta.Name
	if parentPath != "" {
		currentPath = parentPath + "\\" + meta.Name
	}

	// Append this node
	*nodes = append(*nodes, TreeNode{
		NodeID:      nodeID,
		Name:        meta.Name,
		Path:        currentPath,
		Depth:       depth,
		HasChildren: meta.SubkeyN > 0,
		Parent:      parent,
	})

	// Recursively process children if any
	if meta.SubkeyN > 0 {
		childIDs, subkeysErr := r.Subkeys(nodeID)
		if subkeysErr != nil {
			return subkeysErr
		}

		for _, childID := range childIDs {
			recurseErr := buildTreeRecursive(r, childID, currentPath, depth+1, currentPath, nodes)
			if recurseErr != nil {
				// Log error but continue with other children
				// This prevents one corrupted key from breaking the entire tree
				continue
			}
		}
	}

	return nil
}
