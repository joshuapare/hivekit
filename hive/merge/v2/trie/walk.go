package trie

// Walk performs depth-first traversal of the trie, calling fn for each
// non-root node. Depth starts at 1 for root's direct children.
// If fn returns an error, the walk stops and returns that error.
func Walk(root *Node, fn func(node *Node, depth int) error) error {
	return walkNode(root, 0, fn)
}

// walkNode recurses through the trie. The root itself (depth 0) is not passed
// to fn; fn is called for every descendant starting at depth 1.
func walkNode(node *Node, depth int, fn func(node *Node, depth int) error) error {
	if depth > 0 {
		if err := fn(node, depth); err != nil {
			return err
		}
	}
	for _, child := range node.Children {
		if err := walkNode(child, depth+1, fn); err != nil {
			return err
		}
	}
	return nil
}
