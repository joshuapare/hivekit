package edit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ErrKeyDeleted is returned when a key has been marked for deletion
var ErrKeyDeleted = errors.New("key is deleted")

// childMapThreshold is the minimum number of children before we create a childMap.
// For nodes with fewer children, we use linear search on the slice which is faster
// and avoids map allocation overhead for leaf-heavy trees.
const childMapThreshold = 8

// rebuildHive creates a new hive image from the transaction plan.
// The cellBuf parameter should be a pooled buffer that will be used for intermediate
// cell serialization. It will be grown if needed.
func rebuildHive(tx *transaction, cellBuf *[]byte, opts types.WriteOptions) ([]byte, error) {
	alloc := newAllocator()

	// Build the tree structure by walking the base hive and applying changes
	root, err := buildTree(tx, alloc, opts)
	if err != nil {
		return nil, err
	}

	// Pre-calculate buffer size needed
	// Pass changeIdx to enable on-demand materialization of base-ref nodes
	changeIdx := tx.getChangeIndex()
	neededSize := calculateBufferSize(root, tx, changeIdx)
	// Add 10% padding for safety margin and any minor alignment discrepancies
	// Our calculation now accounts for NK min size, DB records, and RI/LF overhead
	// so we don't need the previous 100% padding
	bufferSize := max(neededSize*110/100, 64*1024)

	// Ensure the pooled buffer is large enough
	ensureCapacity(cellBuf, bufferSize)

	// Slice to working size
	workBuf := (*cellBuf)[:bufferSize]

	// Give allocator access to cell buffer for writing free cells
	alloc.setCellBuffer(workBuf)

	// Serialize the tree into the buffer
	// Pass tx and changeIdx to enable on-demand materialization during serialization
	if err := serializeNode(root, alloc, workBuf, opts, tx, changeIdx); err != nil {
		return nil, err
	}

	// Trim buffer to actual size used
	if alloc.nextOffset > int32(len(workBuf)) {
		return nil, fmt.Errorf("buffer overflow: needed %d bytes, had %d (pre-calculated %d)",
			alloc.nextOffset, len(workBuf), neededSize)
	}
	workBuf = workBuf[:alloc.nextOffset]

	// Build final hive with header directly from the cell buffer layout
	return buildFinalHive(workBuf, root.offset, alloc)
}

// calculateBufferSize recursively calculates the total buffer size needed
// for all cells in the tree.
func calculateBufferSize(node *treeNode, tx *transaction, changeIdx *changeIndex) int {
	if node == nil {
		return 0
	}

	// Materialize base-ref nodes on-demand before calculating size
	if err := node.ensureMaterialized(tx, changeIdx); err != nil {
		// If materialization fails, skip this node (shouldn't happen in practice)
		return 0
	}

	total := 0

	// NK cell size: 4 (header) + format.NKFixedHeaderSize + name length
	// BUT: NK has minimum size requirement (80 bytes for short/empty names)
	nkContentSize := format.NKFixedHeaderSize + len(node.name)
	if nkContentSize < format.NKMinSize {
		nkContentSize = format.NKMinSize
	}
	nkSize := format.CellHeaderSize + nkContentSize
	total += alignTo8(nkSize)

	// Value list cell if there are values: 4 (header) + 4*count, aligned to 8 bytes
	if len(node.values) > 0 {
		valListSize := 4 + len(node.values)*4
		total += alignTo8(valListSize)

		// Each VK cell: 4 (header) + format.VKFixedHeaderSize + name length, aligned to 8 bytes
		for _, val := range node.values {
			vkSize := format.CellHeaderSize + format.VKFixedHeaderSize + len(val.name)
			total += alignTo8(vkSize)

			// Data cell calculation depends on size
			dataLen := len(val.data)
			if dataLen > 4 {
				const dbThreshold = 4096
				if dataLen > dbThreshold {
					// Large value uses DB (Big Data) records with multiple cells:
					// 1. Data blocks (multiple cells with headers)
					numBlocks, blockSizes := format.CalculateDBBlocks(dataLen)
					for _, blockSize := range blockSizes {
						blockCellSize := 4 + blockSize // header + data
						total += alignTo8(blockCellSize)
					}
					// 2. Blocklist cell: 4 (header) + 4*numBlocks
					blocklistSize := 4 + numBlocks*4
					total += alignTo8(blocklistSize)
					// 3. DB record cell: 4 (header) + 12 (DB record)
					dbRecordSize := 4 + format.DBMinSize
					total += alignTo8(dbRecordSize)
				} else {
					// Normal data cell: 4 (header) + data length
					dataSize := 4 + dataLen
					total += alignTo8(dataSize)
				}
			}
		}
	}

	// Subkey list calculation depends on count
	if len(node.children) > 0 {
		const maxLFEntries = 500
		count := len(node.children)

		if count <= maxLFEntries {
			// Single LF record: 4 (header) + 2 (sig) + 2 (count) + count*8 (entries)
			lfSize := 4 + 4 + count*8
			total += alignTo8(lfSize)
		} else {
			// Multiple LF records + RI record
			numLFs := (count + maxLFEntries - 1) / maxLFEntries

			// Each LF record
			for i := 0; i < numLFs; i++ {
				entriesInThisLF := maxLFEntries
				if i == numLFs-1 {
					// Last LF might have fewer entries
					entriesInThisLF = count - (i * maxLFEntries)
				}
				lfSize := 4 + 4 + entriesInThisLF*8
				total += alignTo8(lfSize)
			}

			// RI record: 4 (header) + 2 (sig) + 2 (count) + numLFs*4 (offsets)
			riSize := 4 + 4 + numLFs*4
			total += alignTo8(riSize)
		}
	}

	// Recursively calculate for all children
	for _, child := range node.children {
		total += calculateBufferSize(child, tx, changeIdx)
	}

	return total
}

// alignTo8 returns the size aligned to 8-byte boundary.
func alignTo8(size int) int {
	if size%8 == 0 {
		return size
	}
	return size + (8 - size%8)
}

// nodeKind represents the materialization state of a tree node.
type nodeKind int

const (
	// nodeMaterialized indicates the node is fully loaded with children, values, and maps.
	nodeMaterialized nodeKind = iota
	// nodeBaseRef indicates the node is a lightweight reference to base hive data.
	// Children and values are NOT loaded; maps are nil.
	nodeBaseRef
)

// treeNode represents a key in the logical tree.
type treeNode struct {
	name           string
	nameLower      string // cached lowercase name for map lookups (eliminates repeated ToLower calls)
	path           string // cached canonical path with original casing
	pathLower      string // cached lowercase path for change-index lookups
	nameBytes      []byte // original encoded bytes from base hive (nil if new/modified)
	nameCompressed bool   // true if name should be stored in compressed (Windows-1252) format
	offset         int32  // cell offset (assigned during serialization)
	parent         *treeNode
	children       []*treeNode
	childMap       map[string]*treeNode // O(1) case-insensitive lookup map (keys are lowercase)
	childrenSorted bool                 // true if children slice is sorted by name
	values         []treeValue
	valueMap       map[string]*treeValue // O(1) case-insensitive lookup map (keys are lowercase)
	deleted        bool

	// Lazy materialization fields
	kind       nodeKind     // materialization state: nodeMaterialized or nodeBaseRef
	baseNodeID types.NodeID // source node ID if kind == nodeBaseRef
}

// treeValue represents a value in the tree.
type treeValue struct {
	name           string
	nameLower      string // cached lowercase name for map lookups (eliminates repeated ToLower calls)
	nameBytes      []byte // original encoded bytes from base hive (nil if new/modified)
	nameCompressed bool   // true if name should be stored in compressed (Windows-1252) format
	typ            types.RegType
	data           []byte
}

// buildTree constructs the logical tree by merging base + changes.
func buildTree(tx *transaction, alloc *allocator, _ types.WriteOptions) (*treeNode, error) {
	// Build change index for efficient subtree skipping
	changeIdx := tx.getChangeIndex()

	var root *treeNode

	// If no base hive (creating from scratch), start with empty root
	if tx.editor.r == nil {
		root = &treeNode{
			name:           "",
			nameLower:      "", // Empty root has empty lowercase name
			path:           "",
			pathLower:      "",
			nameCompressed: true,
			parent:         nil,
			children:       make([]*treeNode, 0),
			childMap:       nil, // Will be created when threshold is exceeded
			values:         make([]treeValue, 0),
			valueMap:       nil, // Will be created when threshold is exceeded
			deleted:        false,
			kind:           nodeMaterialized, // Created from scratch, already materialized
		}
	} else {
		// Start with root from base hive
		rootID, err := tx.editor.r.Root()
		if err != nil {
			return nil, err
		}

		root, err = buildNodeFromBase(tx, alloc, changeIdx, rootID, "", "", nil) // Root: both path and pathLower are ""
		if err != nil {
			return nil, err
		}
	}

	// Apply created keys (sorted by depth so parents are inserted before children)
	// Map iteration order is randomized in Go, so we need to sort
	createdPaths := make([]string, 0, len(tx.createdKeys))
	for path, node := range tx.createdKeys {
		if !node.exists {
			createdPaths = append(createdPaths, path)
		}
	}
	// Sort by number of path segments (depth), then alphabetically
	sort.Slice(createdPaths, func(i, j int) bool {
		iSegments := len(splitPath(createdPaths[i]))
		jSegments := len(splitPath(createdPaths[j]))
		if iSegments != jSegments {
			return iSegments < jSegments
		}
		return createdPaths[i] < createdPaths[j]
	})

	for _, path := range createdPaths {
		node := tx.createdKeys[path]
		if err := insertCreatedKey(root, path, node); err != nil {
			return nil, err
		}
	}

	// Apply value changes
	for vk, vd := range tx.setValues {
		if err := setValueInTree(root, vk.path, vk.name, vd.typ, vd.data, tx, changeIdx); err != nil {
			return nil, err
		}
	}

	// Apply value deletions
	for vk := range tx.deletedVals {
		if err := deleteValueInTree(root, vk.path, vk.name, tx, changeIdx); err != nil {
			return nil, err
		}
	}

	return root, nil
}

// buildNodeFromBase recursively builds a treeNode from the base types.
func buildNodeFromBase(
	tx *transaction,
	alloc *allocator,
	changeIdx *changeIndex,
	id types.NodeID,
	path string,
	pathLower string, // Pre-normalized path to avoid repeated ToLower calls
	parent *treeNode,
) (*treeNode, error) {
	// Check if this key is deleted (use ChangeIndex for O(1) lookup)
	if changeIdx.HasDeleted(pathLower) {
		return nil, ErrKeyDeleted
	}

	meta, err := tx.editor.r.StatKey(id)
	if err != nil {
		return nil, err
	}

	// Lazy materialization: if this subtree has no changes, create a lightweight base-ref node
	// This avoids allocating maps/slices for unchanged subtrees (88% memory reduction for sparse changes)
	// Use pre-normalized pathLower to avoid calling ToLower
	normalizedPath := pathLower
	if !changeIdx.HasSubtree(normalizedPath) && !changeIdx.HasExact(normalizedPath) {
		return &treeNode{
			kind:           nodeBaseRef,
			baseNodeID:     id,
			name:           meta.Name,
			nameLower:      meta.NameLower, // Use pre-computed lowercase from reader
			path:           path,
			pathLower:      normalizedPath,
			nameBytes:      meta.NameRaw,
			nameCompressed: meta.NameCompressed,
			parent:         parent,
			// NO allocations: children, childMap, values, valueMap are all nil
			// They will be loaded on-demand by ensureMaterialized() during size/serialize passes
		}, nil
	}

	node := &treeNode{
		kind:           nodeMaterialized, // This node has changes, so materialize it
		name:           meta.Name,
		nameLower:      meta.NameLower, // Use pre-computed lowercase from reader // Cache once for all map operations
		path:           path,
		pathLower:      normalizedPath,
		nameBytes:      meta.NameRaw, // Store original bytes for zero-copy serialization
		nameCompressed: meta.NameCompressed,
		parent:         parent,
		children:       make([]*treeNode, 0),
		childMap:       nil, // Lazy-init: only allocate when first child is added
		values:         make([]treeValue, 0),
		valueMap:       nil, // Lazy-init: only allocate when first value is added
	}

	// Load values - optimize based on whether this path has value changes
	// ONLY if there are changes (if no changes, we need to load all values for round-trip)
	valueIDs, err := tx.editor.r.Values(id)
	if err == nil {
		hasAnyChanges := changeIdx.ChangeCount() > 0
		if hasAnyChanges && changeIdx.HasValueChanges(normalizedPath) {
			// Path has value changes - need to check for modifications/deletions
			for _, vid := range valueIDs {
				vmeta, err := tx.editor.r.StatValue(vid)
				if err != nil {
					continue
				}
				vk := valueKey{path: normalizedPath, name: vmeta.Name}
				if tx.deletedVals[vk] {
					continue // value is deleted
				}
				// Check if value is overridden
				if vd, ok := tx.setValues[vk]; ok {
					node.values = append(node.values, treeValue{
						name:           vmeta.Name,
						nameLower:      vmeta.NameLower, // Use pre-computed lowercase from reader
						nameBytes:      nil,             // Modified value, no zero-copy
						nameCompressed: vmeta.NameCompressed,
						typ:            vd.typ,
						data:           vd.data,
					})
					// Lazy-init valueMap on first value
					if node.valueMap == nil {
						node.valueMap = make(map[string]*treeValue, 4) // Pre-allocate small capacity
					}
					node.valueMap[vmeta.NameLower] = &node.values[len(node.values)-1]
				} else {
					// Use original value
					data, err := tx.editor.r.ValueBytes(vid, types.ReadOptions{})
					if err != nil {
						continue
					}
					node.values = append(node.values, treeValue{
						name:           vmeta.Name,
						nameLower:      vmeta.NameLower, // Use pre-computed lowercase from reader
						nameBytes:      vmeta.NameRaw,   // Store original bytes for zero-copy
						nameCompressed: vmeta.NameCompressed,
						typ:            vmeta.Type,
						data:           data,
					})
					// Lazy-init valueMap on first value
					if node.valueMap == nil {
						node.valueMap = make(map[string]*treeValue, 4) // Pre-allocate small capacity
					}
					node.valueMap[vmeta.NameLower] = &node.values[len(node.values)-1]
				}
			}
		} else {
			// Fast path: no value changes - load all values directly without checking maps
			for _, vid := range valueIDs {
				vmeta, err := tx.editor.r.StatValue(vid)
				if err != nil {
					continue
				}
				data, err := tx.editor.r.ValueBytes(vid, types.ReadOptions{})
				if err != nil {
					continue
				}
				node.values = append(node.values, treeValue{
					name:           vmeta.Name,
					nameLower:      vmeta.NameLower, // Use pre-computed lowercase from reader
					nameBytes:      vmeta.NameRaw,   // Store original bytes for zero-copy
					nameCompressed: vmeta.NameCompressed,
					typ:            vmeta.Type,
					data:           data,
				})
				// Lazy-init valueMap on first value
				if node.valueMap == nil {
					node.valueMap = make(map[string]*treeValue, 4) // Pre-allocate small capacity
				}
				node.valueMap[vmeta.NameLower] = &node.values[len(node.values)-1]
			}
		}
	}

	// Load subkeys - optimization: skip recursing into unchanged subtrees
	// ONLY if there are changes (if no changes, we need to build full tree)
	subkeys, err := tx.editor.r.Subkeys(id)
	if err == nil {
		for _, sid := range subkeys {
			smeta, err := tx.editor.r.StatKey(sid)
			if err != nil {
				continue
			}
			// Build child path efficiently (avoid per-iteration allocation)
			childPath := concatPath(path, smeta.Name)
			childPathLower := concatPathLower(pathLower, smeta.NameLower)

			child, err := buildNodeFromBase(tx, alloc, changeIdx, sid, childPath, childPathLower, node)
			if err != nil {
				// Skip deleted keys (they return ErrKeyDeleted)
				if errors.Is(err, ErrKeyDeleted) {
					continue
				}
				return nil, err
			}
			node.children = append(node.children, child)
		}
		// Children from base hive are already sorted (registry format guarantees this)
		if len(node.children) > 0 {
			node.childrenSorted = true
			// Only create childMap if we exceed the threshold
			// For small child counts, linear search is faster and uses less memory
			if len(node.children) > childMapThreshold {
				node.childMap = make(map[string]*treeNode, len(node.children))
				for _, child := range node.children {
					node.childMap[child.nameLower] = child
				}
			}
		}
	}

	return node, nil
}

// ensureMaterialized converts a base-ref node to a materialized node by loading
// children and values from the base hive. If the node is already materialized, this is a no-op.
func (n *treeNode) ensureMaterialized(tx *transaction, changeIdx *changeIndex) error {
	// Already materialized? Nothing to do
	if n.kind == nodeMaterialized {
		return nil
	}

	// Get reader from transaction (not stored on node to avoid interface overhead)
	r := tx.editor.r
	if r == nil {
		return fmt.Errorf("cannot materialize node: no base reader available")
	}

	parentPath := n.path
	parentPathLower := n.pathLower

	// Load children from base hive
	subkeys, err := r.Subkeys(n.baseNodeID)
	if err == nil && len(subkeys) > 0 {
		// Allocate children slice
		n.children = make([]*treeNode, 0, len(subkeys))

		for _, sid := range subkeys {
			smeta, err := r.StatKey(sid)
			if err != nil {
				continue
			}

			// Build child path efficiently without recursion
			childPath := concatPath(parentPath, smeta.Name)
			childPathLower := concatPathLower(parentPathLower, smeta.NameLower)

			// Create child as base-ref (lazy materialization continues recursively)
			// The child will be materialized when/if it's accessed during size/serialize
			child := &treeNode{
				kind:           nodeBaseRef,
				baseNodeID:     sid,
				name:           smeta.Name,
				nameLower:      smeta.NameLower, // Use pre-computed lowercase from reader
				path:           childPath,
				pathLower:      childPathLower,
				nameBytes:      smeta.NameRaw,
				nameCompressed: smeta.NameCompressed,
				parent:         n,
				// Children and values remain nil (lazy)
			}
			n.children = append(n.children, child)
		}

		// Children from base hive are already sorted
		n.childrenSorted = true

		// Only create childMap if we exceed the threshold
		if len(n.children) > childMapThreshold {
			n.childMap = make(map[string]*treeNode, len(n.children))
			for _, child := range n.children {
				n.childMap[child.nameLower] = child
			}
		}
	}

	// Load values from base hive
	valueIDs, err := r.Values(n.baseNodeID)
	if err == nil && len(valueIDs) > 0 {
		// Allocate values slice
		n.values = make([]treeValue, 0, len(valueIDs))

		for _, vid := range valueIDs {
			vmeta, err := r.StatValue(vid)
			if err != nil {
				continue
			}

			data, err := r.ValueBytes(vid, types.ReadOptions{})
			if err != nil {
				continue
			}

			n.values = append(n.values, treeValue{
				name:           vmeta.Name,
				nameLower:      vmeta.NameLower, // Use pre-computed lowercase from reader
				nameBytes:      vmeta.NameRaw,
				nameCompressed: vmeta.NameCompressed,
				typ:            vmeta.Type,
				data:           data,
			})

			// Lazy-init valueMap on first value
			if n.valueMap == nil {
				n.valueMap = make(map[string]*treeValue, 4)
			}
			n.valueMap[vmeta.NameLower] = &n.values[len(n.values)-1]
		}
	}

	// Mark as materialized
	n.kind = nodeMaterialized

	return nil
}

// insertChildSorted inserts a child node while maintaining sort order if possible.
// If parent's children are sorted, inserts at the correct position; otherwise appends.
func insertChildSorted(parent, child *treeNode) {
	if !parent.childrenSorted || len(parent.children) == 0 {
		// Not sorted or empty, just append
		parent.children = append(parent.children, child)
		// Update or create map based on threshold
		if parent.childMap != nil {
			// Map already exists, update it
			parent.childMap[child.nameLower] = child
		} else if len(parent.children) > childMapThreshold {
			// Just crossed threshold, create map and populate with all children
			parent.childMap = make(map[string]*treeNode, len(parent.children))
			for _, c := range parent.children {
				parent.childMap[c.nameLower] = c
			}
		}
		if len(parent.children) == 1 {
			parent.childrenSorted = true // Single child is trivially sorted
		}
		return
	}

	// Find insertion point using binary search
	insertPos := sort.Search(len(parent.children), func(i int) bool {
		return parent.children[i].name >= child.name
	})

	// Insert at position
	parent.children = append(parent.children, nil)
	copy(parent.children[insertPos+1:], parent.children[insertPos:])
	parent.children[insertPos] = child
	// Update or create map based on threshold
	if parent.childMap != nil {
		// Map already exists, update it
		parent.childMap[child.nameLower] = child
	} else if len(parent.children) > childMapThreshold {
		// Just crossed threshold, create map and populate with all children
		parent.childMap = make(map[string]*treeNode, len(parent.children))
		for _, c := range parent.children {
			parent.childMap[c.nameLower] = c
		}
	}
	// Remains sorted
}

// insertCreatedKey inserts a created key into the tree.
func insertCreatedKey(root *treeNode, path string, node *keyNode) error {
	segments := splitPath(path)
	if len(segments) == 0 {
		return nil
	}

	current := root
	// Build path incrementally as we walk down the tree
	currentPath := current.path
	currentPathLower := current.pathLower
	for i, seg := range segments {
		segLower := strings.ToLower(seg)
		child, found := findChild(current, segLower)
		if !found {
			// For the last segment, use the original case-preserving name from keyNode
			// For intermediate segments, use the segment from the path
			nodeName := seg
			if i == len(segments)-1 {
				nodeName = node.name
			}
			nodeLower := strings.ToLower(nodeName)
			newPath := concatPath(currentPath, nodeName)
			newPathLower := concatPathLower(currentPathLower, nodeLower)
			// Create new node preserving original case
			newNode := &treeNode{
				name:           nodeName,
				nameLower:      nodeLower, // Cache lowercase for lookups
				path:           newPath,
				pathLower:      newPathLower,
				nameCompressed: true,
				parent:         current,
				children:       make([]*treeNode, 0),
				childMap:       nil,  // Lazy-init: only allocate when first child is added
				childrenSorted: true, // New node starts sorted (empty)
				values:         make([]treeValue, 0),
				valueMap:       nil,              // Lazy-init: only allocate when first value is added
				kind:           nodeMaterialized, // Created keys are always materialized
			}
			insertChildSorted(current, newNode)
			current = newNode
			currentPath = newPath
			currentPathLower = newPathLower
		} else {
			// Use the child we found in the map; reuse cached path metadata
			current = child
			currentPath = child.path
			currentPathLower = child.pathLower
		}
		// If this is the last segment, we're at the target
		if i == len(segments)-1 {
			return nil
		}
	}
	return nil
}

// setValueInTree sets a value in the tree.
// Materializes base-ref nodes along the path as needed.
func setValueInTree(root *treeNode, path, name string, typ types.RegType, data []byte, tx *transaction, changeIdx *changeIndex) error {
	node := findNodeAndMaterialize(root, path, tx, changeIdx)
	if node == nil {
		return fmt.Errorf("key not found: %s", path)
	}

	nameLower := strings.ToLower(name) // Cache lowercase once
	// Use valueMap for O(1) case-insensitive lookup
	var val *treeValue
	var found bool
	if node.valueMap != nil {
		val, found = node.valueMap[nameLower]
	}
	if found {
		// Update existing value
		val.typ = typ
		val.data = data
		// Preserve existing nameCompressed setting
		return nil
	}

	// New value - default to compressed (deterministic: prefer compression)
	node.values = append(node.values, treeValue{
		name:           name,
		nameLower:      nameLower,
		nameCompressed: true,
		typ:            typ,
		data:           data,
	})
	// Lazy-init valueMap on first value
	if node.valueMap == nil {
		node.valueMap = make(map[string]*treeValue, 4) // Pre-allocate small capacity
	}
	node.valueMap[nameLower] = &node.values[len(node.values)-1]
	return nil
}

// deleteValueInTree deletes a value from the tree.
// Materializes base-ref nodes along the path as needed.
func deleteValueInTree(root *treeNode, path, name string, tx *transaction, changeIdx *changeIndex) error {
	node := findNodeAndMaterialize(root, path, tx, changeIdx)
	if node == nil {
		return nil
	}

	// Use valueMap for O(1) case-insensitive lookup
	lowerName := strings.ToLower(name)
	if node.valueMap != nil {
		if _, found := node.valueMap[lowerName]; !found {
			return nil // Value doesn't exist, nothing to delete
		}
	} else {
		return nil // No values at all, nothing to delete
	}

	// Find and remove from slice (case-insensitive comparison)
	for i, v := range node.values {
		if strings.EqualFold(v.name, name) {
			node.values = append(node.values[:i], node.values[i+1:]...)
			break
		}
	}

	// Remove from map
	delete(node.valueMap, lowerName)

	// Note: We need to rebuild the map because slice indices changed
	// This is necessary because we store pointers to slice elements
	node.valueMap = make(map[string]*treeValue, len(node.values))
	for i := range node.values {
		node.valueMap[node.values[i].nameLower] = &node.values[i]
	}

	return nil
}

// findChild finds a child node by name (case-insensitive).
// Uses map lookup if available, otherwise linear search on the children slice.
func findChild(parent *treeNode, nameLower string) (*treeNode, bool) {
	// Try map lookup first (O(1) if map exists)
	if parent.childMap != nil {
		child, found := parent.childMap[nameLower]
		return child, found
	}

	// Fall back to linear search (acceptable for small child counts)
	for _, child := range parent.children {
		if child.nameLower == nameLower {
			return child, true
		}
	}
	return nil, false
}

// findNode finds a node by path in the tree (case-insensitive).
func findNode(root *treeNode, path string) *treeNode {
	if path == "" {
		return root
	}
	segments := splitPath(path)
	current := root
	for _, seg := range segments {
		segLower := strings.ToLower(seg)
		child, found := findChild(current, segLower)
		if !found {
			return nil
		}
		current = child
	}
	return current
}

// findNodeAndMaterialize finds a node by path in the tree, materializing base-ref nodes along the way.
// This is needed when applying value changes during buildTree, as base-ref nodes don't have their
// children loaded yet.
func findNodeAndMaterialize(root *treeNode, path string, tx *transaction, changeIdx *changeIndex) *treeNode {
	if path == "" {
		if root.kind == nodeBaseRef {
			if err := root.ensureMaterialized(tx, changeIdx); err != nil {
				return nil
			}
		}
		return root
	}
	segments := splitPath(path)
	current := root

	for _, seg := range segments {
		// Materialize the current node (parent) if it's a base-ref
		if current.kind == nodeBaseRef {
			if err := current.ensureMaterialized(tx, changeIdx); err != nil {
				return nil
			}
		}

		// Now we can safely find the child
		segLower := strings.ToLower(seg)
		child, found := findChild(current, segLower)
		if !found {
			return nil
		}

		// Move to child
		current = child
	}
	return current
}

// concatPath efficiently joins parent path and child name with backslash separator.
// Paths are computed once per node and cached on treeNode to avoid repeated work.
func concatPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "\\" + child
}

// concatPathLower joins already-lowercase parent and child paths.
// Used for building normalized paths efficiently without repeated ToLower calls.
func concatPathLower(parentLower, childLower string) string {
	if parentLower == "" {
		return childLower
	}
	return parentLower + "\\" + childLower
}

// splitPath splits a path into segments, preserving original case.
// Optimized to use strings.Split directly instead of bytes conversion.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	// Use strings.Split directly (no string->bytes->string conversion)
	parts := strings.Split(path, `\`)
	// Filter out empty segments in-place (reuse parts slice)
	result := parts[:0]
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// serializeNode recursively serializes a tree node into the cell buffer.
// We need to serialize in an order that allows us to know child offsets
// when building parent's subkey list. So we:
// 1. Serialize NK first (with placeholder 0 for list offsets)
// 2. Serialize children (so we know their offsets)
// 3. Serialize lists using known child offsets
// 4. Update parent NK with list offsets
func serializeNode(node *treeNode, alloc *allocator, buf []byte, opts types.WriteOptions, tx *transaction, changeIdx *changeIndex) error {
	// Materialize base-ref nodes on-demand before serializing
	if err := node.ensureMaterialized(tx, changeIdx); err != nil {
		return fmt.Errorf("failed to materialize node at %q: %w", node.path, err)
	}

	// Sort children by name only if not already sorted
	if !node.childrenSorted && len(node.children) > 0 {
		sort.Slice(node.children, func(i, j int) bool {
			return node.children[i].name < node.children[j].name
		})
		node.childrenSorted = true
	}

	// Reserve space for NK and remember offset
	nkOff := serializeNKToBuf(node, 0, 0, alloc, buf, opts)
	node.offset = nkOff

	// Serialize children recursively (so we know their offsets)
	for _, child := range node.children {
		if err := serializeNode(child, alloc, buf, opts, tx, changeIdx); err != nil {
			return err
		}
	}

	// Now serialize value list (VKs and data)
	valueListOffset := int32(0)
	if len(node.values) > 0 {
		// Use stack allocation for small slices to reduce heap pressure
		const maxStackOffsets = 32
		var stackBuf [maxStackOffsets]int32
		var valueOffsets []int32

		if len(node.values) <= maxStackOffsets {
			valueOffsets = stackBuf[:len(node.values)]
		} else {
			valueOffsets = make([]int32, len(node.values))
		}

		for i, val := range node.values {
			vkOff := serializeVKToBuf(val, alloc, buf)
			// Convert cellBuf offset to HBIN-relative offset
			valueOffsets[i] = alloc.cellBufOffsetToHBINOffset(vkOff)
		}
		valListOff := serializeValueListToBuf(valueOffsets, alloc, buf)
		valueListOffset = alloc.cellBufOffsetToHBINOffset(valListOff)
	}

	// Serialize subkey list now that we know child offsets
	subkeyListOffset := int32(0)
	if len(node.children) > 0 {
		// Use stack allocation for small slices to reduce heap pressure
		const maxStackOffsets = 32
		var stackBuf [maxStackOffsets]int32
		var childOffsets []int32

		if len(node.children) <= maxStackOffsets {
			childOffsets = stackBuf[:len(node.children)]
		} else {
			childOffsets = make([]int32, len(node.children))
		}

		for i, child := range node.children {
			// Convert cellBuf offset to HBIN-relative offset
			childOffsets[i] = alloc.cellBufOffsetToHBINOffset(child.offset)
		}
		subkeyListOff := serializeSubkeyListToBuf(childOffsets, alloc, buf)
		subkeyListOffset = alloc.cellBufOffsetToHBINOffset(subkeyListOff)
	}

	// Update NK with correct list offsets
	updateNKListOffsets(buf, nkOff, valueListOffset, subkeyListOffset)

	return nil
}

// serializeNKToBuf writes an NK cell directly to the buffer.
func serializeNKToBuf(
	node *treeNode,
	valueListOff, subkeyListOff int32,
	alloc *allocator,
	buf []byte,
	opts types.WriteOptions,
) int32 {
	// Use the encoding format from the original record (deterministic)
	var nameBytes []byte
	var nameSize int
	var flags uint16
	var needsEncoding bool

	// Zero-copy optimization: Use original bytes if unchanged
	if node.nameBytes != nil {
		nameBytes = node.nameBytes
		nameSize = len(nameBytes)
		if node.nameCompressed {
			flags = format.NKFlagCompressedName
		} else {
			flags = 0x00
		}
	} else if node.nameCompressed {
		// Compressed name - encode to Windows-1252
		var err error
		nameBytes, err = reader.EncodeKeyName(node.name)
		if err != nil {
			// If encoding fails, the name has characters not in Windows-1252
			// Fall back to UTF-16LE and clear the compressed flag
			nameSize = utf16LESize(node.name)
			needsEncoding = true
			flags = 0x00
		} else {
			nameSize = len(nameBytes)
			flags = format.NKFlagCompressedName // compressed name flag
		}
	} else {
		// Uncompressed name - encode to UTF-16LE
		nameSize = utf16LESize(node.name)
		needsEncoding = true
		flags = 0x00
	}

	contentSize := format.NKFixedHeaderSize + nameSize // Fixed from 0x50 to 0x4C
	// Ensure NK payload meets minimum size requirement (80 bytes)
	// This is needed for keys with short/empty names (e.g., root)
	if contentSize < format.NKMinSize {
		contentSize = format.NKMinSize
	}
	totalSize := contentSize + 4 // +4 for cell header

	// Allocate (will be 8-byte aligned)
	offset := alloc.alloc(totalSize)

	// Calculate aligned size for cell header
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	// Ensure buffer is large enough
	if int(offset)+alignedTotalSize > len(buf) {
		panic(fmt.Sprintf("buffer too small for NK: need %d bytes at offset %d, buffer is %d bytes",
			alignedTotalSize, offset, len(buf)))
	}

	pos := int(offset)

	// Cell header (negative size = allocated, total size includes 4-byte header)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(-alignedTotalSize))
	pos += 4

	// NK signature
	copy(buf[pos:], format.NKSignature)
	pos += 2

	// Flags
	binary.LittleEndian.PutUint16(buf[pos:], flags)
	pos += 2

	// Timestamp
	ts := opts.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	binary.LittleEndian.PutUint64(buf[pos:], toFiletime(ts))
	pos += 8

	// Access bits (0x0C, Windows 8+, ignored)
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Parent offset (0x10)
	// Root node has no parent (InvalidOffset), children point to parent's NK offset
	var parentOffset uint32 = format.InvalidOffset
	if node.parent != nil {
		// Convert parent's cellBuf offset to HBIN-relative offset
		parentOffset = uint32(alloc.cellBufOffsetToHBINOffset(node.parent.offset))
	}
	binary.LittleEndian.PutUint32(buf[pos:], parentOffset)
	pos += 4

	// Subkey count (0x14)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(node.children)))
	pos += 4

	// Volatile subkey count (0x18, ignored)
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Subkey list offset (0x1C)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(subkeyListOff))
	pos += 4

	// Volatile subkey list offset (0x20, ignored)
	binary.LittleEndian.PutUint32(buf[pos:], format.InvalidOffset)
	pos += 4

	// Value count (0x24)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(node.values)))
	pos += 4

	// Value list offset (0x28)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(valueListOff))
	pos += 4

	// Security offset (placeholder)
	binary.LittleEndian.PutUint32(buf[pos:], format.InvalidOffset)
	pos += 4

	// Class offset
	binary.LittleEndian.PutUint32(buf[pos:], format.InvalidOffset)
	pos += 4

	// Max lengths fields (0x30-0x3F: 16 bytes total)
	binary.LittleEndian.PutUint32(buf[pos:], 0) // max name length
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0) // max class length
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0) // max value name length
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0) // max value data length
	pos += 4

	// Work var (0x44: 4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Name length (at 0x48 relative to payload start) - Fixed from 0x4C
	binary.LittleEndian.PutUint16(buf[pos:], uint16(nameSize))
	pos += 2

	// Class length (at 0x4A relative to payload start) - Fixed from 0x4E
	binary.LittleEndian.PutUint16(buf[pos:], 0)
	pos += 2

	// Name (at 0x4C relative to payload start) - Fixed from 0x50
	if needsEncoding {
		// Encode UTF-16LE directly to buffer (zero allocation)
		encodeUTF16LETo(buf[pos:], node.name)
	} else {
		// Copy pre-encoded bytes
		copy(buf[pos:], nameBytes)
	}

	return offset
}

// buildFinalHive assembles the final hive with REGF header + HBINs directly from the cell buffer.
func buildFinalHive(cellBuf []byte, rootOffset int32, alloc *allocator) ([]byte, error) {
	hbinInfos := alloc.getHBINs()
	if len(hbinInfos) == 0 {
		return nil, fmt.Errorf("no HBINs recorded in allocator")
	}

	// Calculate total HBIN size (including headers).
	totalHBINSize := 0
	for _, info := range hbinInfos {
		totalHBINSize += int(info.size)
	}

	// Allocate final hive buffer (header + HBIN data).
	totalSize := format.HeaderSize + totalHBINSize
	result := make([]byte, totalSize)
	header := result[:format.HeaderSize]

	// Populate REGF base block header.
	copy(header[:4], format.REGFSignature)        // 0x00: Signature "regf"
	binary.LittleEndian.PutUint32(header[4:], 1)  // 0x04: Primary sequence number
	binary.LittleEndian.PutUint32(header[8:], 1)  // 0x08: Secondary sequence number
	binary.LittleEndian.PutUint64(header[12:], 0) // 0x0C: Last written timestamp (FILETIME)
	binary.LittleEndian.PutUint32(header[20:], 1) // 0x14: Major version
	binary.LittleEndian.PutUint32(header[24:], 5) // 0x18: Minor version (5 = Windows 2000+)
	binary.LittleEndian.PutUint32(header[28:], 0) // 0x1C: File type (0 = primary file)
	binary.LittleEndian.PutUint32(header[32:], 1) // 0x20: File format (1 = direct memory load)

	// Root offset (0x24) converted from cell buffer offset to HBIN-relative offset.
	rootHBINOffset := alloc.cellBufOffsetToHBINOffset(rootOffset)
	binary.LittleEndian.PutUint32(header[36:], uint32(rootHBINOffset))

	// Hive bins data size (0x28) and clustering factor (0x2C).
	binary.LittleEndian.PutUint32(header[40:], uint32(totalHBINSize))
	binary.LittleEndian.PutUint32(header[44:], 1)

	// Emit HBINs directly into the result buffer.
	destPos := format.HeaderSize
	fileOffset := 0
	for idx, info := range hbinInfos {
		if destPos+int(info.size) > len(result) {
			return nil, fmt.Errorf("hbin %d exceeds allocated buffer", idx)
		}

		dest := result[destPos : destPos+int(info.size)]
		copy(dest[:4], format.HBINSignature)
		binary.LittleEndian.PutUint32(dest[4:], uint32(fileOffset))
		binary.LittleEndian.PutUint32(dest[8:], uint32(info.size))
		binary.LittleEndian.PutUint64(dest[12:], 0)
		binary.LittleEndian.PutUint32(dest[20:], 0)

		dataSize := int(info.size) - format.HBINHeaderSize
		srcStart := int(info.offset)
		var srcEnd int
		if idx < len(hbinInfos)-1 {
			srcEnd = int(hbinInfos[idx+1].offset)
		} else {
			srcEnd = len(cellBuf)
		}
		if srcEnd > len(cellBuf) {
			srcEnd = len(cellBuf)
		}

		toCopy := srcEnd - srcStart
		if toCopy > dataSize {
			toCopy = dataSize
		}
		if toCopy > 0 && srcStart < len(cellBuf) {
			copy(dest[format.HBINHeaderSize:], cellBuf[srcStart:srcStart+toCopy])
		}

		// Mark remaining space as a free cell if needed.
		remaining := dataSize - toCopy
		if remaining >= 4 {
			freePos := format.HBINHeaderSize + toCopy
			binary.LittleEndian.PutUint32(dest[freePos:], uint32(remaining))
		}

		destPos += int(info.size)
		fileOffset += int(info.size)
	}

	// Compute checksum (XOR of first 508 bytes as 127 DWORDs).
	var checksum uint32
	for i := 0; i < 0x1FC; i += 4 {
		checksum ^= binary.LittleEndian.Uint32(header[i : i+4])
	}
	binary.LittleEndian.PutUint32(header[0x1FC:], checksum)

	return result, nil
}

// toFiletime converts a time.Time to Windows FILETIME (100ns since 1601-01-01).
func toFiletime(t time.Time) uint64 {
	const epoch = 116444736000000000 // 100ns intervals from 1601 to 1970
	return uint64(t.Unix())*10000000 + epoch
}

// updateNKListOffsets updates an already-serialized NK record with list offsets.
func updateNKListOffsets(buf []byte, nkOff, valueListOffset, subkeyListOffset int32) {
	pos := int(nkOff) + 4 // Skip cell header

	// Skip to subkey list offset field (at +0x1C in NK payload) - Fixed from 0x18
	binary.LittleEndian.PutUint32(buf[pos+0x1C:], uint32(subkeyListOffset))

	// Skip to value list offset field (at +0x28 in NK payload) - Fixed from 0x24
	binary.LittleEndian.PutUint32(buf[pos+0x28:], uint32(valueListOffset))
}

// serializeBlocklist writes a blocklist cell (array of block offsets) for DB records.
// Returns the HBIN-relative offset of the blocklist cell.
func serializeBlocklist(blockOffsets []uint32, alloc *allocator, buf []byte) int32 {
	// Calculate size: 4 bytes per offset
	dataSize := len(blockOffsets) * 4
	totalSize := dataSize + 4 // +4 for cell header

	// Allocate cell
	offset := alloc.alloc(totalSize)

	// Calculate aligned size for cell header
	alignedSize := totalSize
	if totalSize%8 != 0 {
		alignedSize = totalSize + (8 - totalSize%8)
	}

	// Write cell header (negative = allocated)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(-alignedSize))

	// Write block offsets
	pos := int(offset) + 4
	for _, blockOffset := range blockOffsets {
		binary.LittleEndian.PutUint32(buf[pos:], blockOffset)
		pos += 4
	}

	return alloc.cellBufOffsetToHBINOffset(offset)
}

// serializeDBRecord writes a DB (Big Data) record structure for large values.
// This creates: 1) data blocks, 2) blocklist, 3) DB record cell.
// Returns the HBIN-relative offset of the DB record cell.
func serializeDBRecord(data []byte, alloc *allocator, buf []byte) int32 {
	// Calculate block chunking
	numBlocks, blockSizes := format.CalculateDBBlocks(len(data))

	// Allocate and write data blocks
	blockOffsets := make([]uint32, numBlocks)
	dataOffset := 0

	for i, blockSize := range blockSizes {
		// Allocate data block cell
		totalSize := blockSize + 4 // +4 for cell header
		blockOff := alloc.alloc(totalSize)

		// Calculate aligned size
		alignedSize := totalSize
		if totalSize%8 != 0 {
			alignedSize = totalSize + (8 - totalSize%8)
		}

		// Write cell header (negative = allocated)
		binary.LittleEndian.PutUint32(buf[blockOff:], uint32(-alignedSize))

		// Write data
		copy(buf[blockOff+4:], data[dataOffset:dataOffset+blockSize])

		// Store HBIN-relative offset
		blockOffsets[i] = uint32(alloc.cellBufOffsetToHBINOffset(blockOff))

		dataOffset += blockSize
	}

	// Write blocklist cell
	blocklistOff := serializeBlocklist(blockOffsets, alloc, buf)

	// Write DB record cell
	dbRecordSize := format.DBMinSize + 4 // 12 bytes + 4 byte header
	dbOff := alloc.alloc(dbRecordSize)

	// Calculate aligned size
	alignedSize := dbRecordSize
	if dbRecordSize%8 != 0 {
		alignedSize = dbRecordSize + (8 - dbRecordSize%8)
	}

	// Write cell header
	binary.LittleEndian.PutUint32(buf[dbOff:], uint32(-alignedSize))

	// Write DB record payload using format.EncodeDB
	dbPayload := format.EncodeDB(uint16(numBlocks), uint32(blocklistOff))
	copy(buf[dbOff+4:], dbPayload)

	// Return HBIN-relative offset
	return alloc.cellBufOffsetToHBINOffset(dbOff)
}

// serializeVKToBuf writes a VK cell (and optional data cell) directly to the buffer.
func serializeVKToBuf(val treeValue, alloc *allocator, buf []byte) int32 {
	// Use the encoding format from the original record (deterministic)
	var nameBytes []byte
	var nameSize int
	var flags uint16
	var needsEncoding bool

	// Zero-copy optimization: Use original bytes if unchanged
	if val.nameBytes != nil {
		nameBytes = val.nameBytes
		nameSize = len(nameBytes)
		if val.nameCompressed {
			flags = format.VKFlagASCIIName
		} else {
			flags = 0x00
		}
	} else if val.nameCompressed {
		// Compressed name - encode to Windows-1252
		var err error
		nameBytes, err = reader.EncodeKeyName(val.name)
		if err != nil {
			// If encoding fails, the name has characters not in Windows-1252
			// Fall back to UTF-16LE and clear the compressed flag
			nameSize = utf16LESize(val.name)
			needsEncoding = true
			flags = 0x00
		} else {
			nameSize = len(nameBytes)
			flags = format.VKFlagASCIIName // compressed/ASCII flag
		}
	} else {
		// Uncompressed name - encode to UTF-16LE
		nameSize = utf16LESize(val.name)
		needsEncoding = true
		flags = 0x00
	}
	dataLen := len(val.data)
	dataOff := int32(-1) // Placeholder for unused offset (format.InvalidOffset)

	// Determine if data should be inline (≤4 bytes)
	inline := dataLen <= 4

	if !inline && dataLen > 0 {
		// Determine if we need DB records for large values (>4KB)
		// Use conservative threshold to avoid HBIN boundary issues
		const dbThreshold = 4096

		if dataLen > dbThreshold {
			// Use DB (Big Data) record for large values
			dataOff = serializeDBRecord(val.data, alloc, buf)
		} else {
			// Write normal data cell with alignment
			dataTotalSize := dataLen + 4
			doff := alloc.alloc(dataTotalSize)

			// Calculate aligned size
			alignedDataSize := dataTotalSize
			if dataTotalSize%8 != 0 {
				alignedDataSize = dataTotalSize + (8 - dataTotalSize%8)
			}

			binary.LittleEndian.PutUint32(buf[doff:], uint32(-alignedDataSize))
			copy(buf[doff+4:], val.data)
			// Convert cellBuf offset to HBIN-relative offset
			dataOff = alloc.cellBufOffsetToHBINOffset(doff)
		}
	}

	contentSize := format.VKFixedHeaderSize + nameSize
	totalSize := contentSize + 4
	offset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	if int(offset)+alignedTotalSize > len(buf) {
		panic("buffer too small")
	}

	pos := int(offset)

	// Cell header (total size includes 4-byte header)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(-alignedTotalSize))
	pos += 4

	// VK signature
	copy(buf[pos:], format.VKSignature)
	pos += 2

	// Name length
	binary.LittleEndian.PutUint16(buf[pos:], uint16(len(nameBytes)))
	pos += 2

	// Data size (high bit set if inline)
	if inline {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(dataLen)|format.VKDataInlineBit)
	} else {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(dataLen))
	}
	pos += 4

	// Data offset or inline data
	if inline && dataLen > 0 {
		copy(buf[pos:], val.data)
	} else {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(dataOff))
	}
	pos += 4

	// Type
	binary.LittleEndian.PutUint32(buf[pos:], uint32(val.typ))
	pos += 4

	// Flags (format.VKFlagASCIIName for compressed/ASCII, 0x00 for UTF-16LE)
	binary.LittleEndian.PutUint16(buf[pos:], flags)
	pos += 2

	// Padding
	binary.LittleEndian.PutUint16(buf[pos:], 0)
	pos += 2

	// Name
	if needsEncoding {
		// Encode UTF-16LE directly to buffer (zero allocation)
		encodeUTF16LETo(buf[pos:], val.name)
	} else {
		// Copy pre-encoded bytes
		copy(buf[pos:], nameBytes)
	}

	return offset
}

// serializeValueListToBuf writes a value list cell directly to the buffer.
// With dynamic HBIN sizing, large value lists (>1000 values) can now be stored
// in larger HBINs (8KB, 12KB, or 16KB) as per the Windows Registry spec.
func serializeValueListToBuf(offsets []int32, alloc *allocator, buf []byte) int32 {
	contentSize := len(offsets) * 4
	totalSize := contentSize + 4
	offset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	binary.LittleEndian.PutUint32(buf[offset:], uint32(-alignedTotalSize))
	pos := int(offset) + 4
	for _, off := range offsets {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(off))
		pos += 4
	}
	return offset
}

// serializeSubkeyListToBuf writes a subkey list directly to the buffer.
// For large lists (>500 entries), uses RI (index root) to split into multiple LF records.
// This ensures each record fits within a single HBIN (4KB limit).
func serializeSubkeyListToBuf(offsets []int32, alloc *allocator, buf []byte) int32 {
	count := len(offsets)

	// Maximum entries per LF record that fits in 4KB HBIN:
	// HBIN data: 4064 bytes
	// LF record: 4 (cell header) + 2 (sig) + 2 (count) + n*8 (entries) + padding
	// Safe limit: ~500 entries (8 + 500*8 = 4008 bytes + alignment)
	const maxLFEntries = 500

	if count <= maxLFEntries {
		// Small list - use single LF record
		return serializeSingleLF(offsets, alloc, buf)
	}

	// Large list - split into multiple LF records and use RI to index them
	return serializeRIWithLFs(offsets, alloc, buf, maxLFEntries)
}

// serializeSingleLF writes a single LF record for a subkey list.
func serializeSingleLF(offsets []int32, alloc *allocator, buf []byte) int32 {
	count := len(offsets)
	contentSize := 4 + count*8 // signature(2) + count(2) + entries(count*8)
	totalSize := contentSize + 4
	offset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	binary.LittleEndian.PutUint32(buf[offset:], uint32(-alignedTotalSize))
	pos := int(offset) + 4

	copy(buf[pos:], format.LFSignature)
	pos += 2

	binary.LittleEndian.PutUint16(buf[pos:], uint16(count))
	pos += 2

	for _, off := range offsets {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(off))
		pos += 4
		// Hash (4 bytes) - simplified, use zeros
		binary.LittleEndian.PutUint32(buf[pos:], 0)
		pos += 4
	}

	return offset
}

// serializeRIWithLFs writes an RI record that points to multiple LF records.
func serializeRIWithLFs(offsets []int32, alloc *allocator, buf []byte, maxPerLF int) int32 {
	// Calculate number of LF records needed
	numLFs := (len(offsets) + maxPerLF - 1) / maxPerLF
	lfOffsets := make([]int32, numLFs)

	// Create each LF record
	for i := 0; i < numLFs; i++ {
		start := i * maxPerLF
		end := start + maxPerLF
		if end > len(offsets) {
			end = len(offsets)
		}
		lfOffsets[i] = serializeSingleLF(offsets[start:end], alloc, buf)
	}

	// Create RI record pointing to the LF records
	// RI structure: signature(2) + count(2) + offsets(count*4)
	contentSize := 4 + numLFs*4
	totalSize := contentSize + 4
	riOffset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	binary.LittleEndian.PutUint32(buf[riOffset:], uint32(-alignedTotalSize))
	riPos := int(riOffset) + 4

	copy(buf[riPos:], format.RISignature)
	riPos += 2

	binary.LittleEndian.PutUint16(buf[riPos:], uint16(numLFs))
	riPos += 2

	for _, lfOff := range lfOffsets {
		binary.LittleEndian.PutUint32(buf[riPos:], uint32(alloc.cellBufOffsetToHBINOffset(lfOff)))
		riPos += 4
	}

	return riOffset
}

// utf16LESize calculates the size in bytes needed for UTF-16LE encoding of s.
// Does not allocate.
func utf16LESize(s string) int {
	size := 0
	for _, r := range s {
		if r <= format.UTF16BMPMax {
			size += 2 // BMP character - single UTF-16 code unit
		} else {
			size += 4 // Supplementary character - surrogate pair
		}
	}
	return size
}

// encodeUTF16LETo encodes a UTF-8 string to UTF-16LE bytes, writing to dst.
// Returns the number of bytes written. Assumes dst has sufficient capacity.
// Does not allocate.
func encodeUTF16LETo(dst []byte, s string) int {
	pos := 0
	for _, r := range s {
		if r <= format.UTF16BMPMax {
			// BMP character - single UTF-16 code unit
			dst[pos] = byte(r)
			dst[pos+1] = byte(r >> 8)
			pos += 2
		} else {
			// Supplementary character - surrogate pair
			r -= format.UTF16SurrogateBase
			high := uint16(format.UTF16HighSurrogateStart + (r >> 10))
			low := uint16(format.UTF16LowSurrogateStart + (r & format.UTF16SurrogateMask))
			dst[pos] = byte(high)
			dst[pos+1] = byte(high >> 8)
			dst[pos+2] = byte(low)
			dst[pos+3] = byte(low >> 8)
			pos += 4
		}
	}
	return pos
}

// encodeUTF16LE encodes a UTF-8 string to UTF-16LE bytes.
// This is a convenience wrapper for tests that allocates.
// Production code should use utf16LESize + encodeUTF16LETo for zero allocation.
func encodeUTF16LE(s string) []byte {
	size := utf16LESize(s)
	result := make([]byte, size)
	encodeUTF16LETo(result, s)
	return result
}
