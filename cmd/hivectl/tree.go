package main

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	treeDepth   int
	treeValues  bool
	treeCompact bool
	treeASCII   bool
)

func init() {
	cmd := newTreeCmd()
	cmd.Flags().IntVar(&treeDepth, "depth", 3, "Maximum depth")
	cmd.Flags().BoolVar(&treeValues, "values", false, "Show values too")
	cmd.Flags().BoolVar(&treeCompact, "compact", false, "Compact output")
	cmd.Flags().BoolVar(&treeASCII, "ascii", false, "ASCII-only characters")
	rootCmd.AddCommand(cmd)
}

func newTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree <hive> [path]",
		Short: "Display tree structure",
		Long: `The tree command displays a hierarchical tree view of registry keys.

Example:
  hivectl tree system.hive
  hivectl tree system.hive "ControlSet001\\Services" --depth 2
  hivectl tree system.hive --values --depth 1`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTree(args)
		},
	}
	return cmd
}

func runTree(args []string) error {
	hivePath := args[0]
	var keyPath string
	if len(args) > 1 {
		keyPath = args[1]
	}

	printVerbose("Opening hive: %s\n", hivePath)

	// List keys recursively using public API
	keys, err := hive.ListKeys(hivePath, keyPath, true, treeDepth)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	// Build tree structure
	tree := buildTree(keys, keyPath)

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":  hivePath,
			"path":  keyPath,
			"tree":  tree,
			"depth": treeDepth,
		}
		return printJSON(result)
	}

	// Print root
	if keyPath != "" {
		printInfo("\n%s\n", keyPath)
	} else {
		printInfo("\n(root)\n")
	}

	// Print tree
	printTreeNode(tree, "", true, treeASCII)

	return nil
}

// TreeNode represents a node in the tree
type TreeNode struct {
	Name     string
	Children []*TreeNode
	Values   []string
	IsLast   bool
}

// buildTree constructs a tree from flat key list
func buildTree(keys []hive.KeyInfo, rootPath string) *TreeNode {
	root := &TreeNode{Name: rootPath}

	// Group keys by parent
	childMap := make(map[string][]*TreeNode)

	for _, key := range keys {
		path := key.Path

		// Get parent path and node name
		var parentPath, nodeName string
		lastSep := strings.LastIndex(path, "\\")
		if lastSep == -1 {
			parentPath = ""
			nodeName = path
		} else {
			parentPath = path[:lastSep]
			nodeName = path[lastSep+1:]
		}

		// Create node
		node := &TreeNode{
			Name: nodeName,
		}

		// Add to parent's children
		childMap[parentPath] = append(childMap[parentPath], node)
	}

	// Populate root children
	if rootPath == "" {
		root.Children = childMap[""]
	} else {
		root.Children = childMap[rootPath]
	}

	// Recursively populate children
	var populateChildren func(node *TreeNode, fullPath string)
	populateChildren = func(node *TreeNode, fullPath string) {
		if fullPath == "" {
			fullPath = node.Name
		} else {
			fullPath = fullPath + "\\" + node.Name
		}
		node.Children = childMap[fullPath]
		for _, child := range node.Children {
			populateChildren(child, fullPath)
		}
	}

	for _, child := range root.Children {
		populateChildren(child, rootPath)
	}

	return root
}

// printTreeNode recursively prints tree structure
func printTreeNode(node *TreeNode, prefix string, isLast bool, ascii bool) {
	if node.Name == "" {
		// Root node - print children
		for i, child := range node.Children {
			isChildLast := i == len(node.Children)-1
			printTreeNode(child, "", isChildLast, ascii)
		}
		return
	}

	// Choose tree characters
	var branch, connector, vertical string
	if ascii {
		branch = "+-- "
		connector = "`-- "
		vertical = "|   "
	} else {
		branch = "├── "
		connector = "└── "
		vertical = "│   "
	}

	// Print current node
	if isLast {
		printInfo("%s%s%s\n", prefix, connector, node.Name)
		prefix += "    "
	} else {
		printInfo("%s%s%s\n", prefix, branch, node.Name)
		prefix += vertical
	}

	// Print values if requested
	if treeValues && len(node.Values) > 0 {
		for _, val := range node.Values {
			printInfo("%s    %s\n", prefix, val)
		}
	}

	// Print children
	if !treeCompact || len(node.Children) > 0 {
		for i, child := range node.Children {
			isChildLast := i == len(node.Children)-1
			printTreeNode(child, prefix, isChildLast, ascii)
		}
	}
}
