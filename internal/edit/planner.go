// Package edit plans copy-on-write mutations for hive images.
package edit

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/ast"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Editor wraps a Reader and provides transaction-based editing.
type Editor struct {
	r types.Reader
}

// baseBuffer is an interface for readers that can expose their underlying buffer.
type baseBuffer interface {
	BaseBuffer() []byte
}

// getBaseBuffer extracts the base buffer from a reader for zero-copy AST building.
// Returns nil if the reader doesn't support buffer access.
func getBaseBuffer(r types.Reader) []byte {
	if bb, ok := r.(baseBuffer); ok {
		return bb.BaseBuffer()
	}
	return nil
}

// NewEditor creates an Editor backed by the given Reader.
func NewEditor(r types.Reader) *Editor {
	return &Editor{r: r}
}

// EditorOptions configures editor behavior.
type EditorOptions struct {
	// Limits defines registry constraints to enforce during operations.
	// If nil, no limits are enforced (not recommended for production).
	Limits *ast.Limits
}

// NewEditorWithOptions creates an Editor with custom options.
func NewEditorWithOptions(r types.Reader, opts EditorOptions) *Editor {
	return &Editor{r: r}
}

// Begin starts a new transaction with default limits.
func (e *Editor) Begin() types.Tx {
	defaultLimits := ast.DefaultLimits()
	return &transaction{
		editor:      e,
		createdKeys: make(map[string]*keyNode),
		deletedKeys: make(map[string]bool),
		setValues:   make(map[valueKey]valueData),
		deletedVals: make(map[valueKey]bool),
		limits:      &defaultLimits,
	}
}

// BeginWithLimits starts a new transaction with custom limits.
func (e *Editor) BeginWithLimits(limits ast.Limits) types.Tx {
	return &transaction{
		editor:      e,
		createdKeys: make(map[string]*keyNode),
		deletedKeys: make(map[string]bool),
		setValues:   make(map[valueKey]valueData),
		deletedVals: make(map[valueKey]bool),
		limits:      &limits,
	}
}

// transaction implements types.Tx and accumulates planned changes.
type transaction struct {
	editor      *Editor
	createdKeys map[string]*keyNode    // path -> node metadata
	deletedKeys map[string]bool        // path -> deleted
	setValues   map[valueKey]valueData // (path, name) -> value data
	deletedVals map[valueKey]bool      // (path, name) -> deleted
	limits      *ast.Limits            // registry limits to enforce
	committed   bool
	rolledBack  bool
	changeIdx   *changeIndex // lazy-built index for efficient change queries
}

// keyNode represents metadata for a key (new or existing).
type keyNode struct {
	exists bool            // true if key exists in base hive
	nodeID types.NodeID    // ID if exists
	parent string          // parent path
	name   string          // key name
	values map[string]bool // local value names (for conflict detection)
}

// valueKey identifies a value by path and name.
type valueKey struct {
	path string
	name string
}

// valueData holds value metadata and data.
type valueData struct {
	typ  types.RegType
	data []byte
}

// findInBase looks up a key in the base hive reader.
// Returns ErrNotFound if reader is nil (creating hive from scratch) or key doesn't exist.
func findInBase(r types.Reader, path string) (types.NodeID, error) {
	if r == nil {
		return types.NodeID(0), types.ErrNotFound
	}
	return r.Find(path)
}

// CreateKey ensures a key exists at path. Creates ancestors if opts.CreateParents is true.
func (tx *transaction) CreateKey(path string, opts types.CreateKeyOptions) error {
	if err := tx.checkState(); err != nil {
		return err
	}
	// Extract the original name before normalization to preserve case
	origName := lastSegment(path)
	path = normalizePath(path)
	if tx.deletedKeys[path] {
		return &types.Error{Kind: types.ErrKindState, Msg: fmt.Sprintf("key %q already marked for deletion", path)}
	}
	if tx.createdKeys[path] != nil {
		return nil // already created
	}

	// Check if key exists in base hive
	_, err := findInBase(tx.editor.r, path)
	if err == nil {
		// Key exists, mark as created (no-op but track it)
		tx.createdKeys[path] = &keyNode{
			exists: true,
			parent: parentPath(path),
			name:   origName,
			values: make(map[string]bool),
		}
		return nil
	}

	// Key does not exist; check parent
	parent := parentPath(path)
	if parent != "" {
		// Ensure parent exists
		_, perr := findInBase(tx.editor.r, parent)
		if perr != nil {
			// Parent doesn't exist
			if !opts.CreateParents {
				return &types.Error{Kind: types.ErrKindNotFound, Msg: fmt.Sprintf("parent key %q not found", parent)}
			}
			// Recursively create parent
			if err := tx.CreateKey(parent, opts); err != nil {
				return err
			}
		} else if tx.deletedKeys[parent] {
			// Parent exists in base but is deleted
			return &types.Error{Kind: types.ErrKindState, Msg: fmt.Sprintf("parent key %q is deleted", parent)}
		}
	}

	// Mark key as created
	tx.createdKeys[path] = &keyNode{
		exists: false,
		parent: parent,
		name:   origName,
		values: make(map[string]bool),
	}
	return nil
}

// DeleteKey removes a key. If opts.Recursive is true, removes the subtree.
func (tx *transaction) DeleteKey(path string, opts types.DeleteKeyOptions) error {
	if err := tx.checkState(); err != nil {
		return err
	}
	path = normalizePath(path)
	if tx.deletedKeys[path] {
		return nil // already deleted
	}

	// Check if key exists (either in base or created)
	created := tx.createdKeys[path]
	if created != nil && !created.exists {
		// Key was created in this tx but doesn't exist in base; just remove it
		delete(tx.createdKeys, path)
		return nil
	}

	id, err := findInBase(tx.editor.r, path)
	if err != nil {
		// Key doesn't exist
		return &types.Error{Kind: types.ErrKindNotFound, Msg: fmt.Sprintf("key %q not found", path)}
	}

	// If not recursive, ensure no subkeys
	if !opts.Recursive && tx.editor.r != nil {
		subkeys, err := tx.editor.r.Subkeys(id)
		if err != nil {
			return err
		}
		if len(subkeys) > 0 {
			return &types.Error{Kind: types.ErrKindState, Msg: fmt.Sprintf("key %q has subkeys; use Recursive=true", path)}
		}
	}

	// Mark key (and subtree if recursive) as deleted
	tx.deletedKeys[path] = true

	if opts.Recursive {
		// Mark all descendants as deleted
		if err := tx.markDescendantsDeleted(path); err != nil {
			return err
		}
	}

	return nil
}

// markDescendantsDeleted recursively marks all descendants of path as deleted.
func (tx *transaction) markDescendantsDeleted(path string) error {
	if tx.editor.r == nil {
		return nil // No base hive, nothing to recursively delete
	}

	id, err := tx.editor.r.Find(path)
	if err != nil {
		// path doesn't exist in base hive, nothing to delete
		// This is not an error condition for deletion
		return nil
	}
	subkeys, subkeysErr := tx.editor.r.Subkeys(id)
	if subkeysErr != nil {
		return subkeysErr
	}
	for _, sid := range subkeys {
		meta, err := tx.editor.r.StatKey(sid)
		if err != nil {
			continue
		}
		childPath := path + "\\" + meta.Name
		tx.deletedKeys[childPath] = true
		if err := tx.markDescendantsDeleted(childPath); err != nil {
			return err
		}
	}
	return nil
}

// SetValue sets or replaces a named value at path with the given type/data.
func (tx *transaction) SetValue(path, name string, t types.RegType, data []byte) error {
	if err := tx.checkState(); err != nil {
		return err
	}
	path = normalizePath(path)

	// Ensure key exists or is created
	if tx.deletedKeys[path] {
		return &types.Error{Kind: types.ErrKindState, Msg: fmt.Sprintf("key %q is deleted", path)}
	}

	// Check if key exists (root path "" always exists)
	keyExists := path == ""
	if !keyExists {
		_, err := findInBase(tx.editor.r, path)
		keyExists = err == nil
		if !keyExists && tx.createdKeys[path] == nil {
			// Auto-create the key and its parents (matches Windows RegCreateKeyEx behavior)
			// This allows .reg files to set values without explicitly creating keys first
			if err := tx.CreateKey(path, types.CreateKeyOptions{CreateParents: true}); err != nil {
				return fmt.Errorf("failed to auto-create parent key %q: %w", path, err)
			}
		}
	}

	// Record the set operation
	vk := valueKey{path: path, name: name}
	tx.setValues[vk] = valueData{typ: t, data: data}
	delete(tx.deletedVals, vk) // undelete if previously deleted
	return nil
}

// DeleteValue removes a named value at path.
func (tx *transaction) DeleteValue(path, name string) error {
	if err := tx.checkState(); err != nil {
		return err
	}
	path = normalizePath(path)

	// Ensure key exists
	if tx.deletedKeys[path] {
		return &types.Error{Kind: types.ErrKindState, Msg: fmt.Sprintf("key %q is deleted", path)}
	}

	vk := valueKey{path: path, name: name}
	tx.deletedVals[vk] = true
	delete(tx.setValues, vk) // remove if previously set
	return nil
}

// Commit validates the plan, rebuilds HBINs, and writes a new hive to dst.
func (tx *transaction) Commit(dst types.Writer, opts types.WriteOptions) error {
	if err := tx.checkState(); err != nil {
		return err
	}

	// Validate limits if enabled
	if tx.limits != nil {
		if err := tx.validateLimits(); err != nil {
			return err
		}
	}

	tx.committed = true

	// Get a buffer from the pool for rebuilding
	cellBuf := getBuffer()
	defer putBuffer(cellBuf)

	// Delegate to rebuild and commit logic
	buf, err := rebuildHive(tx, cellBuf, opts)
	if err != nil {
		return err
	}
	return dst.WriteHive(buf)
}

// validateLimits validates the transaction against configured limits.
// This builds an AST and validates all nodes before committing.
func (tx *transaction) validateLimits() error {
	// When creating from scratch (nil reader), skip AST validation
	// The rebuild process will validate as it builds
	if tx.editor.r == nil {
		return nil
	}

	// Build AST to validate
	baseHive := getBaseBuffer(tx.editor.r)
	tree, err := ast.BuildIncremental(tx.editor.r, tx, baseHive)
	if err != nil {
		return fmt.Errorf("failed to build AST for validation: %w", err)
	}

	// Validate tree against limits
	if err := tree.ValidateTree(*tx.limits); err != nil {
		return ast.LimitViolation(err)
	}

	return nil
}

// Rollback discards the transaction.
func (tx *transaction) Rollback() error {
	if tx.committed {
		return &types.Error{Kind: types.ErrKindState, Msg: "transaction already committed"}
	}
	if tx.rolledBack {
		return &types.Error{Kind: types.ErrKindState, Msg: "transaction already rolled back"}
	}
	tx.rolledBack = true
	// Clear resources
	tx.createdKeys = nil
	tx.deletedKeys = nil
	tx.setValues = nil
	tx.deletedVals = nil
	return nil
}

func (tx *transaction) checkState() error {
	if tx.committed {
		return &types.Error{Kind: types.ErrKindState, Msg: "transaction already committed"}
	}
	if tx.rolledBack {
		return &types.Error{Kind: types.ErrKindState, Msg: "transaction already rolled back"}
	}
	return nil
}

// TransactionChanges interface implementation for AST building ----------------

// GetCreatedKeys returns all paths that were created.
func (tx *transaction) GetCreatedKeys() map[string]bool {
	result := make(map[string]bool)
	for path, node := range tx.createdKeys {
		if !node.exists {
			result[path] = true
		}
	}
	return result
}

// GetDeletedKeys returns all paths that were deleted.
func (tx *transaction) GetDeletedKeys() map[string]bool {
	return tx.deletedKeys
}

// GetSetValues returns all values that were set.
func (tx *transaction) GetSetValues() map[ast.ValueKey]ast.ValueData {
	result := make(map[ast.ValueKey]ast.ValueData)
	for vk, vd := range tx.setValues {
		result[ast.ValueKey{Path: vk.path, Name: vk.name}] = ast.ValueData{
			Type: vd.typ,
			Data: vd.data,
		}
	}
	return result
}

// GetDeletedValues returns all values that were deleted.
func (tx *transaction) GetDeletedValues() map[ast.ValueKey]bool {
	result := make(map[ast.ValueKey]bool)
	for vk := range tx.deletedVals {
		result[ast.ValueKey{Path: vk.path, Name: vk.name}] = true
	}
	return result
}

// HasPathChanges returns true if the path or any of its descendants have changes.
// Uses the change index for efficient O(log n) detection.
func (tx *transaction) HasPathChanges(path string) bool {
	idx := tx.getChangeIndex()
	return idx.HasSubtree(path)
}

// normalizePath normalizes a registry path and converts to lowercase.
// Windows registry paths are case-insensitive, so we store everything lowercase
// to avoid repeated ToLower() calls and string allocations.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "\\")
	p = strings.TrimSuffix(p, "\\")
	return strings.ToLower(p)
}

// parentPath returns the parent path of p, or "" if p is root.
func parentPath(p string) string {
	idx := strings.LastIndex(p, "\\")
	if idx < 0 {
		return ""
	}
	return p[:idx]
}

// lastSegment returns the last segment of a path.
func lastSegment(p string) string {
	idx := strings.LastIndex(p, "\\")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

// getChangeIndex returns the change index, building it lazily on first access.
// This index is used for efficient change detection during tree building.
func (tx *transaction) getChangeIndex() *changeIndex {
	if tx.changeIdx == nil {
		tx.changeIdx = buildChangeIndex(tx)
	}
	return tx.changeIdx
}
