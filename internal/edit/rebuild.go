package edit

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

const (
	// bufferSizeMultiplier is the multiplier for buffer size calculation.
	// Adds 100% padding for alignment overhead, safety margin, and metadata.
	bufferSizeMultiplier = 2

	// listEntrySize is the size of a subkey list entry (offset + hash).
	listEntrySize = 8

	// inlineDataThreshold is the maximum data size that can be stored inline in VK cells.
	inlineDataThreshold = format.DWORDSize

	// listHeaderSize is the size of the list header (signature + count).
	listHeaderSize = 4
)

// ErrKeyDeleted is returned when a key has been marked for deletion.
var ErrKeyDeleted = errors.New("key is deleted")

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
	neededSize := calculateBufferSize(root)
	// Add 100% padding for alignment overhead, safety margin, and metadata
	// This is conservative but ensures we never run out of space
	bufferSize := max(neededSize*bufferSizeMultiplier, 64*1024)

	// Ensure the pooled buffer is large enough
	ensureCapacity(cellBuf, int(bufferSize))

	// Slice to working size
	workBuf := (*cellBuf)[:int(bufferSize)]

	// Serialize the tree into the buffer
	if serializeErr := serializeNode(root, alloc, workBuf, opts); serializeErr != nil {
		return nil, serializeErr
	}

	// Trim buffer to actual size used
	if alloc.nextOffset > int32(len(workBuf)) {
		return nil, fmt.Errorf("buffer overflow: needed %d bytes, had %d (pre-calculated %d)",
			alloc.nextOffset, len(workBuf), neededSize)
	}
	workBuf = workBuf[:int(alloc.nextOffset)]

	// Build HBINs from the cell buffer
	hbins := packCellBuffer(workBuf, opts.Repack)

	// Build final hive with header
	return buildFinalHive(hbins, root.offset)
}

// calculateBufferSize recursively calculates the total buffer size needed
// for all cells in the tree.
func calculateBufferSize(node *treeNode) int32 {
	if node == nil {
		return 0
	}

	var total int32

	// NK cell size: 4 (header) + format.NKFixedHeaderSize + name length, aligned to 8 bytes
	nkSize := int32(format.CellHeaderSize + format.NKFixedHeaderSize + len(node.name))
	total += alignTo8(nkSize)

	// Value list cell if there are values: header + DWORD*count, aligned to 8 bytes
	if len(node.values) > 0 {
		valListSize := int32(format.CellHeaderSize + len(node.values)*format.DWORDSize)
		total += alignTo8(valListSize)

		// Each VK cell: header + format.VKFixedHeaderSize + name length, aligned to 8 bytes
		for _, val := range node.values {
			vkSize := int32(format.CellHeaderSize + format.VKFixedHeaderSize + len(val.name))
			total += alignTo8(vkSize)

			// Data cell if data > threshold: header + data length, aligned to 8 bytes
			if len(val.data) > inlineDataThreshold {
				dataSize := int32(format.CellHeaderSize + len(val.data))
				total += alignTo8(dataSize)
			}
		}
	}

	// Subkey list cell if there are children: header + list header + count*entrySize, aligned to 8 bytes
	if len(node.children) > 0 {
		subkeyListSize := int32(format.CellHeaderSize + listHeaderSize + len(node.children)*listEntrySize)
		total += alignTo8(subkeyListSize)
	}

	// Recursively calculate for all children
	for _, child := range node.children {
		total += calculateBufferSize(child)
	}

	return total
}

// alignTo8 returns the size aligned to 8-byte boundary.
func alignTo8(size int32) int32 {
	if size%format.CellAlignment == 0 {
		return size
	}
	return size + (format.CellAlignment - size%format.CellAlignment)
}

// treeNode represents a key in the logical tree.
type treeNode struct {
	name           string
	nameCompressed bool  // true if name should be stored in compressed (Windows-1252) format
	offset         int32 // cell offset (assigned during serialization)
	parent         *treeNode
	children       []*treeNode
	childMap       map[string]*treeNode // O(1) lookup map for children
	childrenSorted bool                 // true if children slice is sorted by name
	values         []treeValue
	valueMap       map[string]*treeValue // O(1) lookup map for values
	deleted        bool
}

// treeValue represents a value in the tree.
type treeValue struct {
	name           string
	nameCompressed bool // true if name should be stored in compressed (Windows-1252) format
	typ            types.RegType
	data           []byte
}

// buildTree constructs the logical tree by merging base + changes.
func buildTree(tx *transaction, _ *allocator, _ types.WriteOptions) (*treeNode, error) {
	var root *treeNode

	// If no base hive (creating from scratch), start with empty root
	if tx.editor.r == nil {
		root = &treeNode{
			name:           "",
			nameCompressed: true,
			parent:         nil,
			children:       make([]*treeNode, 0),
			childMap:       make(map[string]*treeNode),
			values:         make([]treeValue, 0),
			valueMap:       make(map[string]*treeValue),
			deleted:        false,
		}
	} else {
		// Start with root from base hive
		rootID, err := tx.editor.r.Root()
		if err != nil {
			return nil, err
		}

		root, err = buildNodeFromBase(tx, rootID, "", nil)
		if err != nil {
			return nil, err
		}
	}

	// Apply created keys
	for path, node := range tx.createdKeys {
		if node.exists {
			continue // already in tree
		}
		insertCreatedKey(root, path, node)
	}

	// Apply value changes
	for vk, vd := range tx.setValues {
		if err := setValueInTree(root, vk.path, vk.name, vd.typ, vd.data); err != nil {
			return nil, err
		}
	}

	// Apply value deletions
	for vk := range tx.deletedVals {
		deleteValueInTree(root, vk.path, vk.name)
	}

	return root, nil
}

// buildNodeFromBase recursively builds a treeNode from the base types.
func buildNodeFromBase(
	tx *transaction,
	id types.NodeID,
	path string,
	parent *treeNode,
) (*treeNode, error) {
	// Check if this key is deleted
	if tx.deletedKeys[path] {
		return nil, ErrKeyDeleted
	}

	meta, err := tx.editor.r.StatKey(id)
	if err != nil {
		return nil, err
	}

	node := &treeNode{
		name:           meta.Name,
		nameCompressed: meta.NameCompressed,
		parent:         parent,
		children:       make([]*treeNode, 0),
		childMap:       make(map[string]*treeNode),
		values:         make([]treeValue, 0),
		valueMap:       make(map[string]*treeValue),
	}

	// Load values
	valueIDs, valErr := tx.editor.r.Values(id)
	if valErr == nil {
		for _, vid := range valueIDs {
			vmeta, statErr := tx.editor.r.StatValue(vid)
			if statErr != nil {
				continue
			}
			vk := valueKey{path: path, name: vmeta.Name}
			if tx.deletedVals[vk] {
				continue // value is deleted
			}
			// Check if value is overridden
			if vd, ok := tx.setValues[vk]; ok {
				node.values = append(node.values, treeValue{
					name:           vmeta.Name,
					nameCompressed: vmeta.NameCompressed,
					typ:            vd.typ,
					data:           vd.data,
				})
				// Add to valueMap for O(1) lookup
				node.valueMap[vmeta.Name] = &node.values[len(node.values)-1]
			} else {
				// Use original value
				data, readErr := tx.editor.r.ValueBytes(vid, types.ReadOptions{CopyData: true})
				if readErr != nil {
					continue
				}
				node.values = append(node.values, treeValue{
					name:           vmeta.Name,
					nameCompressed: vmeta.NameCompressed,
					typ:            vmeta.Type,
					data:           data,
				})
				// Add to valueMap for O(1) lookup
				node.valueMap[vmeta.Name] = &node.values[len(node.values)-1]
			}
		}
	}

	// Load subkeys
	subkeys, subErr := tx.editor.r.Subkeys(id)
	if subErr == nil {
		for _, sid := range subkeys {
			smeta, statErr := tx.editor.r.StatKey(sid)
			if statErr != nil {
				continue
			}
			childPath := path
			if childPath != "" {
				childPath += "\\"
			}
			childPath += smeta.Name

			child, buildErr := buildNodeFromBase(tx, sid, childPath, node)
			if buildErr != nil {
				// Skip deleted keys (they return ErrKeyDeleted)
				if errors.Is(buildErr, ErrKeyDeleted) {
					continue
				}
				return nil, buildErr
			}
			node.children = append(node.children, child)
			node.childMap[child.name] = child
		}
		// Children from base hive are already sorted (registry format guarantees this)
		if len(node.children) > 0 {
			node.childrenSorted = true
		}
	}

	return node, nil
}

// insertChildSorted inserts a child node while maintaining sort order if possible.
// If parent's children are sorted, inserts at the correct position; otherwise appends.
func insertChildSorted(parent, child *treeNode) {
	if !parent.childrenSorted || len(parent.children) == 0 {
		// Not sorted or empty, just append
		parent.children = append(parent.children, child)
		parent.childMap[child.name] = child
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
	parent.childMap[child.name] = child
	// Remains sorted
}

// insertCreatedKey inserts a created key into the tree.
func insertCreatedKey(root *treeNode, path string, _ *keyNode) {
	segments := splitPath(path)
	if len(segments) == 0 {
		return
	}

	current := root
	for i, seg := range segments {
		// Use childMap for O(1) lookup
		child, found := current.childMap[seg]
		if !found {
			// Create new node with compressed name by default (deterministic: prefer compression)
			newNode := &treeNode{
				name:           seg,
				nameCompressed: true,
				parent:         current,
				children:       make([]*treeNode, 0),
				childMap:       make(map[string]*treeNode),
				childrenSorted: true, // New node starts sorted (empty)
				values:         make([]treeValue, 0),
				valueMap:       make(map[string]*treeValue),
			}
			insertChildSorted(current, newNode)
			current = newNode
		} else {
			// Use the child we found in the map
			current = child
		}
		// If this is the last segment, we're at the target
		if i == len(segments)-1 {
			return
		}
	}
}

// setValueInTree sets a value in the tree.
func setValueInTree(root *treeNode, path, name string, typ types.RegType, data []byte) error {
	node := findNode(root, path)
	if node == nil {
		return fmt.Errorf("key not found: %s", path)
	}

	// Use valueMap for O(1) lookup
	if val, found := node.valueMap[name]; found {
		// Update existing value
		val.typ = typ
		val.data = data
		// Preserve existing nameCompressed setting
		return nil
	}

	// New value - default to compressed (deterministic: prefer compression)
	node.values = append(node.values, treeValue{
		name:           name,
		nameCompressed: true,
		typ:            typ,
		data:           data,
	})
	// Add to valueMap for future O(1) lookups
	node.valueMap[name] = &node.values[len(node.values)-1]
	return nil
}

// deleteValueInTree deletes a value from the tree.
func deleteValueInTree(root *treeNode, path, name string) {
	node := findNode(root, path)
	if node == nil {
		return
	}

	// Use valueMap for O(1) lookup
	if _, found := node.valueMap[name]; !found {
		return // Value doesn't exist, nothing to delete
	}

	// Find and remove from slice
	for i, v := range node.values {
		if v.name == name {
			node.values = append(node.values[:i], node.values[i+1:]...)
			break
		}
	}

	// Remove from map
	delete(node.valueMap, name)

	// Note: We need to rebuild the map because slice indices changed
	// This is necessary because we store pointers to slice elements
	node.valueMap = make(map[string]*treeValue, len(node.values))
	for i := range node.values {
		node.valueMap[node.values[i].name] = &node.values[i]
	}
}

// findNode finds a node by path in the tree.
func findNode(root *treeNode, path string) *treeNode {
	if path == "" {
		return root
	}
	segments := splitPath(path)
	current := root
	for _, seg := range segments {
		found := false
		for _, child := range current.children {
			if child.name == seg {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// splitPath splits a path into segments.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := bytes.Split([]byte(path), []byte("\\"))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 0 {
			result = append(result, string(p))
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
// 4. Update parent NK with list offsets.
func serializeNode(node *treeNode, alloc *allocator, buf []byte, opts types.WriteOptions) error {
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
		if err := serializeNode(child, alloc, buf, opts); err != nil {
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
			valueOffsets[i] = cellBufOffsetToHBINOffset(vkOff)
		}
		valListOff := serializeValueListToBuf(valueOffsets, alloc, buf)
		valueListOffset = cellBufOffsetToHBINOffset(valListOff)
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
			childOffsets[i] = cellBufOffsetToHBINOffset(child.offset)
		}
		subkeyListOff := serializeSubkeyListToBuf(childOffsets, alloc, buf)
		subkeyListOffset = cellBufOffsetToHBINOffset(subkeyListOff)
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
	var flags uint16
	if node.nameCompressed {
		// Compressed name - encode to Windows-1252
		var err error
		nameBytes, err = reader.EncodeKeyName(node.name)
		if err != nil {
			// If encoding fails, the name has characters not in Windows-1252
			// Fall back to UTF-16LE and clear the compressed flag
			nameBytes = encodeUTF16LE(node.name)
			flags = 0x00
		} else {
			flags = format.NKFlagCompressedName // compressed name flag
		}
	} else {
		// Uncompressed name - encode to UTF-16LE
		nameBytes = encodeUTF16LE(node.name)
		flags = 0x00
	}

	contentSize := int32(format.NKFixedHeaderSize + len(nameBytes)) // Fixed from 0x50 to 0x4C
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
	if int(offset)+int(alignedTotalSize) > len(buf) {
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
		parentOffset = uint32(cellBufOffsetToHBINOffset(node.parent.offset))
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
	binary.LittleEndian.PutUint16(buf[pos:], uint16(len(nameBytes)))
	pos += 2

	// Class length (at 0x4A relative to payload start) - Fixed from 0x4E
	binary.LittleEndian.PutUint16(buf[pos:], 0)
	pos += 2

	// Name (at 0x4C relative to payload start) - Fixed from 0x50
	copy(buf[pos:], nameBytes)

	return offset
}

// buildFinalHive assembles the final hive with REGF header + HBINs.
func buildFinalHive(hbins [][]byte, rootOffset int32) ([]byte, error) {
	header := make([]byte, format.HeaderSize)
	// REGF Base Block Header (per Windows Registry File Format Specification)
	copy(header[:4], format.REGFSignature)        // 0x00: Signature "regf"
	binary.LittleEndian.PutUint32(header[4:], 1)  // 0x04: Primary sequence number
	binary.LittleEndian.PutUint32(header[8:], 1)  // 0x08: Secondary sequence number
	binary.LittleEndian.PutUint64(header[12:], 0) // 0x0C: Last written timestamp (FILETIME)
	binary.LittleEndian.PutUint32(header[20:], 1) // 0x14: Major version
	binary.LittleEndian.PutUint32(header[24:], 5) // 0x18: Minor version (5 = Windows 2000+)
	binary.LittleEndian.PutUint32(header[28:], 0) // 0x1C: File type (0 = primary file)
	binary.LittleEndian.PutUint32(header[32:], 1) // 0x20: File format (1 = direct memory load)
	// Root offset - convert from cellBuf offset to HBIN-relative offset
	binary.LittleEndian.PutUint32(
		header[36:],
		uint32(cellBufOffsetToHBINOffset(rootOffset)),
	) // 0x24: Root cell offset

	// Calculate total HBIN data size
	var totalHBINSize uint32
	for _, hbin := range hbins {
		totalHBINSize += uint32(len(hbin))
	}
	binary.LittleEndian.PutUint32(header[40:], totalHBINSize) // 0x28: Hive bins data size
	binary.LittleEndian.PutUint32(
		header[44:],
		1,
	) // 0x2C: Clustering factor (sector_size/512)

	// Calculate checksum (XOR of first 508 bytes as 127 DWORDs)
	var checksum uint32
	for i := 0; i < 0x1FC; i += 4 {
		checksum ^= binary.LittleEndian.Uint32(header[i : i+4])
	}
	binary.LittleEndian.PutUint32(header[0x1FC:], checksum)

	// Calculate total hive size
	totalSize := format.HeaderSize
	for _, hbin := range hbins {
		totalSize += len(hbin)
	}

	result := make([]byte, totalSize)
	copy(result, header)
	pos := format.HeaderSize
	for _, hbin := range hbins {
		copy(result[pos:], hbin)
		pos += len(hbin)
	}

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

// serializeVKToBuf writes a VK cell (and optional data cell) directly to the buffer.
func serializeVKToBuf(val treeValue, alloc *allocator, buf []byte) int32 {
	// Use the encoding format from the original record (deterministic)
	var nameBytes []byte
	var flags uint16
	if val.nameCompressed {
		// Compressed name - encode to Windows-1252
		var err error
		nameBytes, err = reader.EncodeKeyName(val.name)
		if err != nil {
			// If encoding fails, the name has characters not in Windows-1252
			// Fall back to UTF-16LE and clear the compressed flag
			nameBytes = encodeUTF16LE(val.name)
			flags = 0x00
		} else {
			flags = format.VKFlagASCIIName // compressed/ASCII flag
		}
	} else {
		// Uncompressed name - encode to UTF-16LE
		nameBytes = encodeUTF16LE(val.name)
		flags = 0x00
	}
	dataLen := int32(len(val.data))
	dataOff := int32(-1) // Placeholder for unused offset (format.InvalidOffset)

	// Determine if data should be inline (≤4 bytes)
	inline := dataLen <= 4

	if !inline && dataLen > 0 {
		// Write data cell with alignment
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
		dataOff = cellBufOffsetToHBINOffset(doff)
	}

	contentSize := int32(format.VKFixedHeaderSize + len(nameBytes))
	totalSize := contentSize + 4
	offset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	if int(offset)+int(alignedTotalSize) > len(buf) {
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
	copy(buf[pos:], nameBytes)

	return offset
}

// serializeValueListToBuf writes a value list cell directly to the buffer.
func serializeValueListToBuf(offsets []int32, alloc *allocator, buf []byte) int32 {
	contentSize := int32(len(offsets) * 4)
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

// serializeSubkeyListToBuf writes a subkey list (LF format) directly to the buffer.
func serializeSubkeyListToBuf(offsets []int32, alloc *allocator, buf []byte) int32 {
	count := len(offsets)
	contentSize := int32(4 + count*8) // signature(2) + count(2) + entries(count*8) - NO padding!
	totalSize := contentSize + 4
	offset := alloc.alloc(totalSize)

	// Calculate aligned size
	alignedTotalSize := totalSize
	if totalSize%8 != 0 {
		alignedTotalSize = totalSize + (8 - totalSize%8)
	}

	binary.LittleEndian.PutUint32(buf[offset:], uint32(-alignedTotalSize))
	pos := int(offset) + 4

	copy(buf[pos:], format.LFSignature) // LF signature
	pos += 2

	binary.LittleEndian.PutUint16(buf[pos:], uint16(count))
	pos += 2 // +2 for count field only - NO padding!

	for _, off := range offsets {
		binary.LittleEndian.PutUint32(buf[pos:], uint32(off))
		pos += 4
		// Hash (4 bytes) - simplified, use zeros
		binary.LittleEndian.PutUint32(buf[pos:], 0)
		pos += 4
	}

	return offset
}

// cellBufOffsetToHBINOffset converts a cellBuf offset to an HBIN-relative offset.
// This accounts for HBIN headers when cells span multiple HBINs.
func cellBufOffsetToHBINOffset(cellBufOff int32) int32 {
	const hbinDataSize = format.HBINDataSize // 4KB - header
	const hbinHeaderSize = format.HBINHeaderSize

	// Calculate which HBIN this cell is in
	hbinIndex := cellBufOff / hbinDataSize

	// Calculate position within that HBIN
	posInHBIN := cellBufOff % hbinDataSize

	// HBIN-relative offset accounts for all HBIN headers:
	// - Each HBIN is format.HBINAlignment bytes total
	// - First HBIN starts at HBIN-relative 0x0, but data starts at HBINHeaderSize
	// - HBIN N starts at HBIN-relative N*HBINAlignment, data at N*HBINAlignment + HBINHeaderSize
	return hbinIndex*format.HBINAlignment + hbinHeaderSize + posInHBIN
}

// encodeUTF16LE encodes a UTF-8 string to UTF-16LE bytes.
func encodeUTF16LE(s string) []byte {
	// Estimate 2 bytes per character
	result := make([]byte, 0, len(s)*2)

	// Range over string directly to get runes (Unicode code points)
	for _, r := range s {
		if r <= format.UTF16BMPMax {
			// BMP character - single UTF-16 code unit
			result = append(result, byte(r), byte(r>>8))
		} else {
			// Supplementary character - surrogate pair
			r -= format.UTF16SurrogateBase
			high := uint16(format.UTF16HighSurrogateStart + (r >> 10))
			low := uint16(format.UTF16LowSurrogateStart + (r & format.UTF16SurrogateMask))
			result = append(result, byte(high), byte(high>>8))
			result = append(result, byte(low), byte(low>>8))
		}
	}
	return result
}

// packCellBuffer packs the cell buffer into HBINs.
func packCellBuffer(cellBuf []byte, _ bool) [][]byte {
	const hbinSize = format.HBINAlignment // 4KB bins
	const hbinHeaderSize = format.HBINHeaderSize
	const hbinDataSize = format.HBINDataSize // 4KB - header

	// Calculate how many HBINs we need based on data size (not total size)
	// Each HBIN holds hbinDataSize bytes of cell data
	numHBINs := (len(cellBuf) + hbinDataSize - 1) / hbinDataSize
	if numHBINs == 0 {
		numHBINs = 1
	}

	hbins := make([][]byte, numHBINs)
	for i := range numHBINs {
		hbins[i] = make([]byte, hbinSize)

		// HBIN header
		copy(hbins[i][:4], format.HBINSignature)
		binary.LittleEndian.PutUint32(hbins[i][4:], uint32(i*hbinSize)) // offset from first HBIN
		binary.LittleEndian.PutUint32(hbins[i][8:], hbinSize)           // size of this HBIN
		binary.LittleEndian.PutUint64(hbins[i][12:], 0)                 // timestamp
		binary.LittleEndian.PutUint32(hbins[i][20:], 0)                 // spare
	}

	// Copy cell data into HBINs
	srcPos := 0
	for i := 0; i < numHBINs && srcPos < len(cellBuf); i++ {
		dstPos := hbinHeaderSize
		available := hbinSize - hbinHeaderSize
		toCopy := min(len(cellBuf)-srcPos, available)
		copy(hbins[i][dstPos:], cellBuf[srcPos:srcPos+toCopy])
		srcPos += toCopy

		// Fill remaining space with a free cell marker
		// Per Windows Registry spec, all unused space must be marked as a free cell
		// Free cells have positive size values (vs negative for used cells)
		remaining := available - toCopy
		if remaining >= 4 {
			// Write free cell size marker (positive value indicates free)
			freePos := dstPos + toCopy
			binary.LittleEndian.PutUint32(hbins[i][freePos:], uint32(remaining))
		}
	}

	return hbins
}
