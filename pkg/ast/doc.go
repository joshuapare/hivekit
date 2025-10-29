// Package ast provides an in-memory abstract syntax tree representation
// of Windows Registry hive files.
//
// The AST represents the hierarchical structure of a registry hive as a tree
// of nodes (keys) and values. This package is intended for tools that need
// direct access to the hive structure for analysis, transformation, or custom
// serialization beyond what the high-level hive package provides.
//
// # Core Types
//
// Tree represents a complete hive structure with a root node. Nodes correspond
// to registry keys (NK records) and contain child nodes and values. Values
// correspond to VK records and hold typed data (strings, integers, binary).
//
// # Incremental Building
//
// BuildIncremental constructs an AST from a base hive and a set of changes.
// Unchanged subtrees remain lazily loaded and reference the original hive buffer,
// avoiding unnecessary copying. Modified subtrees are materialized in memory
// with dirty flags for tracking.
//
// # Validation
//
// The Limits type enforces Windows Registry constraints during operations.
// Three presets are available: DefaultLimits matches Windows specifications,
// RelaxedLimits allows larger structures for special cases, and StrictLimits
// provides conservative bounds for resource-constrained environments.
//
// # Usage Example
//
//	// Build AST from hive and changes
//	tree, err := ast.BuildIncremental(reader, changes, baseBuffer)
//	if err != nil {
//		return err
//	}
//
//	// Validate against Windows limits
//	if err := tree.ValidateTree(ast.DefaultLimits()); err != nil {
//		return err
//	}
//
//	// Traverse and modify
//	node := tree.FindNode("Software\\MyApp")
//	node.AddValue("Setting", types.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
//
// For standard registry operations (read, write, merge), use the higher-level
// hive package instead. Use ast when you need control over the tree structure,
// custom traversal algorithms, or non-standard serialization formats.
package ast
