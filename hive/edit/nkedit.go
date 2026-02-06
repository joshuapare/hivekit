package edit

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/hive/values"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// utf16BytesPerChar is the number of bytes per UTF-16 character.
	utf16BytesPerChar = 2

	// windowsFileTimeTicksPerNanosecond is the conversion factor from nanoseconds
	// to Windows FILETIME ticks (100-nanosecond intervals).
	windowsFileTimeTicksPerNanosecond = 100
)

// decodeName decodes a registry name (NK or VK) from raw bytes.
// The name can be compressed (ASCII) or uncompressed (UTF-16LE), determined by the compressed flag.
// Returns the name in lowercase for case-insensitive comparison (matching index behavior).
//
// Uses the same optimized UTF-16LE decoding as internal/regtext for consistency and performance.
func decodeName(nameBytes []byte, compressed bool) string {
	if len(nameBytes) == 0 {
		return ""
	}

	if compressed {
		// Compressed (ASCII)
		return strings.ToLower(string(nameBytes))
	}

	// Uncompressed (UTF-16LE)
	// Truncate odd-length data (invalid UTF-16LE)
	if len(nameBytes)%utf16BytesPerChar != 0 {
		nameBytes = nameBytes[:len(nameBytes)-1]
	}
	if len(nameBytes) == 0 {
		return ""
	}

	// Convert UTF-16LE bytes to uint16 words using stdlib for efficiency
	words := make([]uint16, len(nameBytes)/utf16BytesPerChar)
	for i := range words {
		words[i] = format.ReadU16(nameBytes, i*utf16BytesPerChar)
	}

	// Use stdlib utf16.Decode for optimized conversion
	return strings.ToLower(string(utf16.Decode(words)))
}

// deferredParent tracks pending children for a parent key in deferred mode.
type deferredParent struct {
	children      []subkeys.Entry
	oldListRef    uint32          // Existing list reference (if any) to free during flush
	deletedNKRefs map[uint32]bool // NKRefs to exclude during flush (for deferred deletions)
	loaded        bool            // True if children have been loaded from disk
}

// keyEditor implements KeyEditor interface.
type keyEditor struct {
	h     *hive.Hive
	alloc alloc.Allocator
	index index.Index
	dt    dirty.DirtyTracker

	// Deferred subkey list building
	deferredMode    bool
	deferredParents map[uint32]*deferredParent // parentRef -> deferred children

	// SK cell deduplication (security descriptor sharing)
	skMap   map[string]uint32 // SHA256(sec_desc) -> SK cell offset
	skList  []uint32          // List of SK cell offsets for doubly-linked list
	skMutex sync.Mutex        // Protect concurrent access to skMap/skList
}

// NewKeyEditor creates a new key editor.
func NewKeyEditor(
	h *hive.Hive,
	allocator alloc.Allocator,
	idx index.Index,
	dt dirty.DirtyTracker,
) KeyEditor {
	return &keyEditor{
		h:               h,
		alloc:           allocator,
		index:           idx,
		dt:              dt,
		deferredMode:    false,
		deferredParents: nil, // Created when deferred mode is enabled
		skMap:           make(map[string]uint32),
		skList:          make([]uint32, 0),
	}
}

// EnableDeferredMode enables deferred subkey list building.
// In deferred mode, subkey list updates are accumulated in memory
// and written all at once during FlushDeferredSubkeys().
// This dramatically improves bulk building performance by eliminating
// expensive read-modify-write cycles.
func (ke *keyEditor) EnableDeferredMode() {
	ke.deferredMode = true
	ke.deferredParents = make(map[uint32]*deferredParent)
}

// DisableDeferredMode disables deferred subkey list building.
// Returns an error if there are pending deferred updates (call FlushDeferredSubkeys first).
func (ke *keyEditor) DisableDeferredMode() error {
	if len(ke.deferredParents) > 0 {
		return fmt.Errorf(
			"cannot disable deferred mode: %d parents have pending children (call FlushDeferredSubkeys first)",
			len(ke.deferredParents),
		)
	}
	ke.deferredMode = false
	ke.deferredParents = nil
	return nil
}

// EnsureKeyPath creates intermediate keys as needed (case-insensitive).
// Returns the final NK reference and the count of keys created.
func (ke *keyEditor) EnsureKeyPath(root NKRef, segments []string) (NKRef, int, error) {
	if root == 0 {
		return 0, 0, ErrInvalidRef
	}

	current := root
	keysCreated := 0

	// Walk each segment
	for _, segment := range segments {
		if segment == "" {
			return 0, 0, ErrInvalidKeyName
		}

		// Check if this key already exists (index handles case-insensitivity)
		existingRef, ok := ke.index.GetNK(current, segment)
		if ok {
			// Key exists, continue to next segment
			current = existingRef
			continue
		}

		// Key doesn't exist - need to create it
		newRef, err := ke.createKey(current, segment)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create key %q: %w", segment, err)
		}

		keysCreated++
		current = newRef
	}

	return current, keysCreated, nil
}

// createKey creates a new NK cell and inserts it into the parent's subkey list.
func (ke *keyEditor) createKey(parentRef NKRef, name string) (NKRef, error) {
	// Resolve parent NK
	parentPayload, err := ke.resolveCell(parentRef)
	if err != nil {
		return 0, fmt.Errorf("resolve parent: %w", err)
	}

	// Declare variable for later use after re-resolution
	var parentNK hive.NK
	_, err = hive.ParseNK(parentPayload)
	if err != nil {
		return 0, fmt.Errorf("parse parent NK: %w", err)
	}

	// Allocate new NK cell
	nkRef, err := ke.allocateNK(parentRef, name)
	if err != nil {
		return 0, fmt.Errorf("allocate NK: %w", err)
	}

	// CRITICAL: Re-resolve parent NK after allocation (may have grown hive)
	parentPayload, resolveErr := ke.resolveCell(parentRef)
	if resolveErr != nil {
		return 0, fmt.Errorf("re-resolve parent after alloc: %w", resolveErr)
	}

	parentNK, parseErr := hive.ParseNK(parentPayload)
	if parseErr != nil {
		return 0, fmt.Errorf("re-parse parent NK: %w", parseErr)
	}

	// Update parent's subkey list (subkey list uses lowercase for hash computation)
	if insertErr := ke.insertIntoSubkeyList(parentRef, parentNK, nkRef, name); insertErr != nil {
		return 0, fmt.Errorf("insert into subkey list: %w", insertErr)
	}

	// Register in index (index handles lowercasing internally)
	ke.index.AddNK(parentRef, name, nkRef)

	return nkRef, nil
}

// allocateNK allocates and initializes a new NK cell.
func (ke *keyEditor) allocateNK(parentRef NKRef, name string) (NKRef, error) {
	// Encode name (ASCII for simplicity - Windows may use UTF-16)
	nameBytes := []byte(name)
	nameLen := len(nameBytes)

	// Calculate payload size: fixed header + name
	payloadSize := format.NKFixedHeaderSize + nameLen
	totalSize := int32(payloadSize + format.CellHeaderSize)

	// Get or create the SK cell BEFORE allocating the NK cell. createSKCell
	// calls alloc.Alloc which can trigger hive growth, invalidating any buf
	// returned by a prior Alloc call.
	secDesc := DefaultSecurityDescriptor()
	skOffset, skErr := ke.getOrCreateSKCell(secDesc)
	if skErr != nil {
		return 0, fmt.Errorf("get/create SK cell: %w", skErr)
	}

	// Allocate cell (safe now — no further Alloc calls before we finish writing buf)
	ref, buf, err := ke.alloc.Alloc(totalSize, alloc.ClassNK)
	if err != nil {
		return 0, fmt.Errorf("alloc cell: %w", err)
	}

	// Write NK signature
	buf[0] = 'n'
	buf[1] = 'k'

	// Flags: compressed name
	flags := uint16(format.NKFlagCompressedName)
	format.PutU16(buf, format.NKFlagsOffset, flags)

	// Timestamp: current time in Windows FILETIME format
	filetime := timeToFiletime(time.Now())
	copy(buf[format.NKLastWriteOffset:format.NKLastWriteOffset+8], filetime[:])

	// Parent offset (relative)
	format.PutU32(buf, format.NKParentOffset, parentRef)

	// Subkey count: 0
	format.PutU32(buf, format.NKSubkeyCountOffset, 0)

	// Volatile subkey count: 0
	format.PutU32(buf, format.NKVolSubkeyCountOffset, 0)

	// Subkey list offset: InvalidOffset (none)
	format.PutU32(buf, format.NKSubkeyListOffset, format.InvalidOffset)

	// Volatile subkey list offset: InvalidOffset (none)
	format.PutU32(buf, format.NKVolSubkeyListOffset, format.InvalidOffset)

	// Value count: 0
	format.PutU32(buf, format.NKValueCountOffset, 0)

	// Value list offset: InvalidOffset (none)
	format.PutU32(buf, format.NKValueListOffset, format.InvalidOffset)

	// Security offset (computed before Alloc to avoid stale buf)
	format.PutU32(buf, format.NKSecurityOffset, skOffset)

	// Class name offset: InvalidOffset (none)
	format.PutU32(buf, format.NKClassNameOffset, format.InvalidOffset)

	// Max name len, max class len, max value name len, max value len: 0
	format.PutU32(buf, format.NKMaxNameLenOffset, 0)
	format.PutU32(buf, format.NKMaxClassLenOffset, 0)
	format.PutU32(buf, format.NKMaxValueNameOffset, 0)
	format.PutU32(buf, format.NKMaxValueDataOffset, 0)

	// Name length
	format.PutU16(buf, format.NKNameLenOffset, uint16(nameLen))

	// Class length: 0
	format.PutU16(buf, format.NKClassLenOffset, 0)

	// Copy name bytes
	copy(buf[format.NKNameOffset:], nameBytes)

	// Mark newly created NK cell as dirty so it's flushed to disk
	ke.markCellDirty(ref)

	return ref, nil
}

// insertIntoSubkeyList updates the parent NK's subkey list to include the new child.
// In deferred mode, accumulates children in memory for later flush.
// In immediate mode, performs the traditional read-modify-write cycle.
func (ke *keyEditor) insertIntoSubkeyList(
	parentRef NKRef,
	parentNK hive.NK,
	childRef NKRef,
	childNameLower string,
) error {
	// Compute hash for the child name
	hash := subkeys.Hash(childNameLower)

	// Create entry
	newEntry := subkeys.Entry{
		NameLower: childNameLower,
		NKRef:     childRef,
		Hash:      hash,
	}

	// DEFERRED MODE: Just accumulate the child in memory
	if ke.deferredMode {
		return ke.insertDeferredChild(parentRef, parentNK, newEntry)
	}

	// IMMEDIATE MODE: Traditional read-modify-write cycle
	return ke.insertImmediateChild(parentRef, parentNK, newEntry)
}

// insertDeferredChild accumulates a child in memory for later flush.
func (ke *keyEditor) insertDeferredChild(parentRef NKRef, parentNK hive.NK, entry subkeys.Entry) error {
	// Get or create deferred parent
	dp, exists := ke.deferredParents[parentRef]
	if !exists {
		// First child for this parent in current deferred batch
		// CRITICAL: Read existing children to merge with new ones.
		// This handles the case where AutoFlushThreshold causes multiple flush cycles
		// and a parent gets children across different cycles.
		var oldListRef uint32 = format.InvalidOffset
		existingChildren := []subkeys.Entry{}

		if parentNK.SubkeyCount() > 0 {
			oldListRef = parentNK.SubkeyListOffsetRel()

			// Read existing subkey list to preserve children from previous flushes
			if oldListRef != format.InvalidOffset {
				existingList, err := subkeys.Read(ke.h, oldListRef)
				if err == nil {
					existingChildren = existingList.Entries
				}
				// If read fails, start with empty list (degrades to overwriting)
			}
		}

		dp = &deferredParent{
			children:   existingChildren, // Start with existing children!
			oldListRef: oldListRef,
			loaded:     true, // We've loaded the children
		}
		ke.deferredParents[parentRef] = dp
	} else if !dp.loaded {
		// Parent exists but children not loaded yet (was created by removeDeferredChild).
		// We need to load existing children now before appending, otherwise
		// flushDeferredParent will overwrite our new entry when it loads children.
		if parentNK.SubkeyCount() > 0 {
			oldListRef := parentNK.SubkeyListOffsetRel()
			if oldListRef != format.InvalidOffset {
				existingList, err := subkeys.Read(ke.h, oldListRef)
				if err == nil {
					dp.children = existingList.Entries
				}
				dp.oldListRef = oldListRef
			}
		}
		dp.loaded = true
	}

	// Append child (we'll sort during flush)
	dp.children = append(dp.children, entry)

	return nil
}

// insertImmediateChild performs the traditional read-modify-write cycle.
func (ke *keyEditor) insertImmediateChild(parentRef NKRef, parentNK hive.NK, entry subkeys.Entry) error {
	// Read existing subkey list (if any)
	var existingList *subkeys.List
	var err error
	var oldListRef uint32 = format.InvalidOffset

	subkeyCount := parentNK.SubkeyCount()
	if subkeyCount > 0 {
		listRef := parentNK.SubkeyListOffsetRel()
		if listRef != format.InvalidOffset {
			oldListRef = listRef // Save for freeing later
			existingList, err = subkeys.Read(ke.h, listRef)
			if err != nil {
				// If read fails, start with empty list
				existingList = &subkeys.List{Entries: []subkeys.Entry{}}
			}
		} else {
			existingList = &subkeys.List{Entries: []subkeys.Entry{}}
		}
	} else {
		existingList = &subkeys.List{Entries: []subkeys.Entry{}}
	}

	// Insert new entry (maintains sorted order)
	newList := existingList.Insert(entry)

	// OPTIMIZATION: Free old subkey list BEFORE allocating new one.
	// This allows the allocator to reuse freed space immediately,
	// preventing hive growth when cells of similar size are freed and reallocated.
	// The children are already in memory (newList), so the old list is no longer needed.
	if oldListRef != format.InvalidOffset {
		if err := ke.freeSubkeyList(oldListRef); err != nil {
			// Log but don't fail - continue with the write
			_ = err
		}
	}

	// Write updated subkey list (List.Insert maintains sorted order)
	newListRef, err := subkeys.WritePresorted(ke.h, ke.alloc, newList.Entries)
	if err != nil {
		return fmt.Errorf("write subkey list: %w", err)
	}

	// Update parent NK's subkey count and list reference
	if err := ke.updateParentNK(parentRef, uint32(len(newList.Entries)), newListRef); err != nil {
		return fmt.Errorf("update parent NK: %w", err)
	}

	return nil
}

// flushDeferredParent writes a single parent's accumulated deferred children to disk.
// After flushing, the parent's on-disk subkey list and count are up-to-date,
// and the deferred entry is removed from the map.
// Returns nil if the parent has no deferred entry.
func (ke *keyEditor) flushDeferredParent(parentRef uint32) error {
	dp, exists := ke.deferredParents[parentRef]
	if !exists {
		return nil // No deferred children for this parent
	}

	// OPTIMIZATION: Delete-only path uses ReadRaw/WriteRaw to avoid NK decoding.
	// If no children were added (dp.children is empty) and we have deletions,
	// we can read the raw entries (NKRef+Hash), filter, and write back directly.
	// This skips the expensive name decoding that dominates CPU time.
	if !dp.loaded && len(dp.children) == 0 && len(dp.deletedNKRefs) > 0 {
		parentPayload, err := ke.resolveCell(parentRef)
		if err != nil {
			return fmt.Errorf("resolve parent: %w", err)
		}

		parentNK, err := hive.ParseNK(parentPayload)
		if err != nil {
			return fmt.Errorf("parse parent NK: %w", err)
		}

		oldListRef := parentNK.SubkeyListOffsetRel()
		if oldListRef == format.InvalidOffset {
			// No existing list, nothing to delete from
			delete(ke.deferredParents, parentRef)
			return nil
		}

		// Read raw entries (no name decoding!)
		rawEntries, err := subkeys.ReadRaw(ke.h, oldListRef)
		if err != nil {
			return fmt.Errorf("read raw subkey list: %w", err)
		}

		// Filter out deleted entries (preserves sorted order)
		filtered := make([]subkeys.RawEntry, 0, len(rawEntries)-len(dp.deletedNKRefs))
		for _, entry := range rawEntries {
			if !dp.deletedNKRefs[entry.NKRef] {
				filtered = append(filtered, entry)
			}
		}

		// Free old list before allocating new one
		if err := ke.freeSubkeyList(oldListRef); err != nil {
			_ = err // Log but continue
		}

		// Write raw entries (no name needed!)
		newListRef, err := subkeys.WriteRaw(ke.h, ke.alloc, filtered)
		if err != nil {
			return fmt.Errorf("write raw subkey list: %w", err)
		}

		// Update parent NK
		if err := ke.updateParentNK(parentRef, uint32(len(filtered)), newListRef); err != nil {
			return fmt.Errorf("update parent NK 0x%X: %w", parentRef, err)
		}

		delete(ke.deferredParents, parentRef)
		return nil
	}

	// Standard path: children already loaded by insertDeferredChild.
	// (Delete-only parents take the raw path above, so if we reach here, loaded must be true.)

	// Filter out any deleted entries
	children := dp.children
	if len(dp.deletedNKRefs) > 0 {
		children = make([]subkeys.Entry, 0, len(dp.children))
		for _, child := range dp.children {
			if !dp.deletedNKRefs[child.NKRef] {
				children = append(children, child)
			}
		}
	}

	// Sort children by name (lowercase) for consistent ordering
	sortEntries(children)

	// OPTIMIZATION: Free old subkey list BEFORE allocating new one.
	// This allows the allocator to reuse freed space immediately,
	// preventing hive growth when cells of similar size are freed and reallocated.
	// The children are already in memory, so the old list is no longer needed.
	if dp.oldListRef != format.InvalidOffset {
		if err := ke.freeSubkeyList(dp.oldListRef); err != nil {
			// Log but don't fail - continue with the write
			_ = err
		}
	}

	// Write the complete subkey list (already sorted above)
	newListRef, err := subkeys.WritePresorted(ke.h, ke.alloc, children)
	if err != nil {
		return fmt.Errorf("write subkey list for parent 0x%X: %w", parentRef, err)
	}

	// Update parent NK cell
	if err := ke.updateParentNK(parentRef, uint32(len(children)), newListRef); err != nil {
		return fmt.Errorf("update parent NK 0x%X: %w", parentRef, err)
	}

	// Remove from deferred parents map
	delete(ke.deferredParents, parentRef)

	return nil
}

// FlushDeferredSubkeys writes all accumulated deferred children to disk.
// This must be called before disabling deferred mode.
// Returns the number of parents flushed.
//
// OPTIMIZATION: Uses a two-pass approach:
// 1. First pass: Free all old subkey lists (returns space to allocator)
// 2. Second pass: Allocate and write all new subkey lists
// This allows the allocator to reuse freed space immediately, preventing
// hive growth when many subkey lists of similar size are being rewritten.
func (ke *keyEditor) FlushDeferredSubkeys() (int, error) {
	if !ke.deferredMode {
		return 0, nil // Nothing to do
	}

	if len(ke.deferredParents) == 0 {
		return 0, nil // No pending children
	}

	// Collect keys first since we'll be modifying the map
	parents := make([]uint32, 0, len(ke.deferredParents))
	for ref := range ke.deferredParents {
		parents = append(parents, ref)
	}

	// PASS 1: Free all old subkey lists FIRST
	// This returns space to the allocator before any new allocations,
	// maximizing the chance of reusing freed cells.
	for _, parentRef := range parents {
		dp := ke.deferredParents[parentRef]
		if dp.oldListRef != format.InvalidOffset {
			// Sort entries now so they're ready for pass 2
			sortEntries(dp.children)

			// Free the old list - this makes space available for new allocations
			if err := ke.freeSubkeyList(dp.oldListRef); err != nil {
				// Log but don't fail
				_ = err
			}
			// Mark as already freed so pass 2 doesn't double-free
			dp.oldListRef = format.InvalidOffset
		}
	}

	// PASS 2: Allocate and write all new subkey lists
	flushedCount := 0
	for _, parentRef := range parents {
		if err := ke.flushDeferredParent(parentRef); err != nil {
			return flushedCount, err
		}
		flushedCount++
	}

	return flushedCount, nil
}

// sortEntries sorts entries by NameLower (ascending).
// This matches the sort order used by List.Insert.
func sortEntries(entries []subkeys.Entry) {
	// Simple insertion sort for small lists, quicksort for larger
	if len(entries) < 32 {
		// Insertion sort
		for i := 1; i < len(entries); i++ {
			key := entries[i]
			j := i - 1
			for j >= 0 && entries[j].NameLower > key.NameLower {
				entries[j+1] = entries[j]
				j--
			}
			entries[j+1] = key
		}
	} else {
		// Use stdlib sort for larger lists
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].NameLower < entries[j].NameLower
		})
	}
}

// freeSubkeyList frees a subkey list and all its associated structures.
// Handles LF/LH/LI/RI list formats.
func (ke *keyEditor) freeSubkeyList(listRef uint32) error {
	if listRef == 0 || listRef == format.InvalidOffset {
		return nil
	}

	// Get the list cell to determine its type
	listBuf, err := ke.resolveCell(listRef)
	if err != nil {
		return fmt.Errorf("resolve subkey list: %w", err)
	}

	if len(listBuf) < 2 {
		return fmt.Errorf("subkey list too short: %d bytes", len(listBuf))
	}

	// Check signature to determine list type
	sig := string(listBuf[0:2])

	switch sig {
	case "ri":
		// RI list - contains pointers to LF/LH chunks
		// Must free all chunks before freeing the RI list itself
		if len(listBuf) < 8 {
			return errors.New("RI list too short")
		}

		// Get number of chunks (at offset 2)
		numChunks := format.ReadU16(listBuf, 2)

		// Free each chunk (chunk refs start at offset 4)
		for i := range numChunks {
			chunkOffset := 4 + (i * 4)
			if int(chunkOffset)+4 > len(listBuf) {
				break
			}
			chunkRef := format.ReadU32(listBuf, int(chunkOffset))
			if chunkRef != 0 && chunkRef != format.InvalidOffset {
				// Recursively free the chunk (it's an LF/LH list)
				if freeErr := ke.freeSubkeyList(chunkRef); freeErr != nil {
					// Log but continue freeing other chunks
					_ = freeErr
				}
			}
		}

		// Now free the RI list itself
		return ke.alloc.Free(listRef)

	case "lf", "lh", "li":
		// Direct list - just free the cell
		return ke.alloc.Free(listRef)

	default:
		// Unknown signature - free anyway to prevent leaks
		return ke.alloc.Free(listRef)
	}
}

// updateParentNK updates the parent NK's subkey count and list reference.
func (ke *keyEditor) updateParentNK(parentRef NKRef, newCount uint32, newListRef uint32) error {
	parentPayload, err := ke.resolveCell(parentRef)
	if err != nil {
		return err
	}

	// Update subkey count
	format.PutU32(parentPayload, format.NKSubkeyCountOffset, newCount)

	// Update subkey list offset
	format.PutU32(parentPayload, format.NKSubkeyListOffset, newListRef)

	// Update timestamp
	filetime := timeToFiletime(time.Now())
	copy(parentPayload[format.NKLastWriteOffset:format.NKLastWriteOffset+8], filetime[:])

	// Mark parent NK cell as dirty so changes are flushed to disk
	ke.markCellDirty(parentRef)

	return nil
}

// DeleteKey removes a key and optionally its subtree.
// If recursive=false and the key has subkeys, returns ErrKeyHasSubkeys.
// Cannot delete the root key (returns ErrCannotDeleteRoot).
func (ke *keyEditor) DeleteKey(nk NKRef, recursive bool) error {
	// Validate reference
	if nk == 0 || nk == 0xFFFFFFFF {
		return ErrInvalidRef
	}

	// Parse NK cell
	nkPayload, err := ke.resolveCell(nk)
	if err != nil {
		return fmt.Errorf("resolve NK: %w", err)
	}

	nkCell, err := hive.ParseNK(nkPayload)
	if err != nil {
		return fmt.Errorf("parse NK: %w", err)
	}

	// Check if this is the root key
	// The root can be identified by comparing with the hive's root cell offset
	rootCellOffset := ke.h.RootCellOffset()
	if nk == rootCellOffset {
		return ErrCannotDeleteRoot
	}

	parentRef := nkCell.ParentOffsetRel()

	// If deferred mode is active, flush this key's deferred children to disk
	// so that deleteSubkeys can find them, and flush the parent so that
	// removeFromParentSubkeyList reads the current on-disk subkey list.
	if ke.deferredMode {
		if err := ke.flushDeferredParent(nk); err != nil {
			return fmt.Errorf("flush deferred children before delete: %w", err)
		}
		if err := ke.flushDeferredParent(parentRef); err != nil {
			return fmt.Errorf("flush deferred parent before delete: %w", err)
		}
		// Re-resolve NK after flush may have caused allocations/hive growth
		nkPayload, err = ke.resolveCell(nk)
		if err != nil {
			return fmt.Errorf("re-resolve NK after deferred flush: %w", err)
		}
		nkCell, err = hive.ParseNK(nkPayload)
		if err != nil {
			return fmt.Errorf("re-parse NK after deferred flush: %w", err)
		}
	}

	// Check recursive constraint (re-read subkeyCount after potential flush)
	subkeyCount := nkCell.SubkeyCount()
	if !recursive && subkeyCount > 0 {
		return ErrKeyHasSubkeys
	}

	// Phase 1: Recursively delete all subkeys (if any)
	if subkeyCount > 0 {
		if deleteErr := ke.deleteSubkeys(nk, nkCell); deleteErr != nil {
			return fmt.Errorf("delete subkeys: %w", deleteErr)
		}

		// CRITICAL: Re-resolve NK after deleteSubkeys which may have caused hive growth
		var resolveErr error
		nkPayload, resolveErr = ke.resolveCell(nk)
		if resolveErr != nil {
			return fmt.Errorf("re-resolve NK after deleteSubkeys: %w", resolveErr)
		}

		var parseErr error
		nkCell, parseErr = hive.ParseNK(nkPayload)
		if parseErr != nil {
			return fmt.Errorf("re-parse NK after deleteSubkeys: %w", parseErr)
		}
	}

	// Phase 2: Delete all values (if any)
	if deleteErr := ke.deleteAllValues(nk, nkCell); deleteErr != nil {
		return fmt.Errorf("delete values: %w", deleteErr)
	}

	// CRITICAL: Re-resolve NK after deleteAllValues which may have caused hive growth
	nkPayload, resolveErr := ke.resolveCell(nk)
	if resolveErr != nil {
		return fmt.Errorf("re-resolve NK after deleteAllValues: %w", resolveErr)
	}

	nkCell, parseErr := hive.ParseNK(nkPayload)
	if parseErr != nil {
		return fmt.Errorf("re-parse NK after deleteAllValues: %w", parseErr)
	}

	// Extract the key name BEFORE removeFromParentSubkeyList, which can trigger
	// alloc.Alloc → hive growth → invalidating the nkCell byte slice.
	nkName := decodeName(nkCell.Name(), nkCell.IsCompressedName())

	// Phase 3: Remove from parent's subkey list (using NKRef, not nkCell)
	if removeErr := ke.removeFromParentSubkeyList(parentRef, nk); removeErr != nil {
		return fmt.Errorf("remove from parent: %w", removeErr)
	}

	// Phase 4: Remove from index and free the NK cell itself

	// Remove from index to maintain consistency
	ke.index.RemoveNK(parentRef, nkName)

	// Free the NK cell
	if freeErr := ke.alloc.Free(nk); freeErr != nil {
		return fmt.Errorf("free NK cell: %w", freeErr)
	}

	return nil
}

// deleteSubkeys recursively deletes all subkeys of the given NK.
// OPTIMIZATION: This directly deletes child subtrees without updating the parent's
// subkey list for each child. The parent's subkey list is freed once at the end.
// This reduces O(n²) subkey list rewrites to O(n).
func (ke *keyEditor) deleteSubkeys(parentRef NKRef, nk hive.NK) error {
	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		// No subkeys
		return nil
	}

	// Read subkey list
	subkeyList, err := subkeys.Read(ke.h, subkeyListRef)
	if err != nil {
		return fmt.Errorf("read subkey list: %w", err)
	}

	// Delete each child's subtree WITHOUT updating this key's subkey list
	for _, entry := range subkeyList.Entries {
		if deleteErr := ke.deleteSubtreeNoParentUpdate(parentRef, entry.NKRef); deleteErr != nil {
			return fmt.Errorf("delete subkey %q: %w", entry.NameLower, deleteErr)
		}
	}

	// Free the subkey list itself (just once, not per-child)
	if freeErr := ke.freeSubkeyList(subkeyListRef); freeErr != nil {
		// Log but don't fail - we've already deleted the children
		_ = freeErr
	}

	return nil
}

// deleteSubtreeNoParentUpdate deletes a key and its entire subtree without
// updating the parent's subkey list. Used by deleteSubkeys for O(n) deletion.
func (ke *keyEditor) deleteSubtreeNoParentUpdate(parentRef NKRef, nkRef uint32) error {
	// Parse NK cell
	nkPayload, err := ke.resolveCell(nkRef)
	if err != nil {
		return fmt.Errorf("resolve NK: %w", err)
	}

	nkCell, err := hive.ParseNK(nkPayload)
	if err != nil {
		return fmt.Errorf("parse NK: %w", err)
	}

	// Extract name for index removal
	nkName := decodeName(nkCell.Name(), nkCell.IsCompressedName())

	// Recursively delete all subkeys (if any)
	subkeyCount := nkCell.SubkeyCount()
	if subkeyCount > 0 {
		if deleteErr := ke.deleteSubkeys(nkRef, nkCell); deleteErr != nil {
			return fmt.Errorf("delete subkeys: %w", deleteErr)
		}
	}

	// Delete all values
	// Re-resolve after subkey deletion may have caused hive growth
	nkPayload, err = ke.resolveCell(nkRef)
	if err != nil {
		return fmt.Errorf("re-resolve NK: %w", err)
	}
	nkCell, err = hive.ParseNK(nkPayload)
	if err != nil {
		return fmt.Errorf("re-parse NK: %w", err)
	}

	if deleteErr := ke.deleteAllValues(nkRef, nkCell); deleteErr != nil {
		return fmt.Errorf("delete values: %w", deleteErr)
	}

	// Remove from index
	ke.index.RemoveNK(parentRef, nkName)

	// Free the NK cell
	if freeErr := ke.alloc.Free(nkRef); freeErr != nil {
		return fmt.Errorf("free NK cell: %w", freeErr)
	}

	return nil
}

// deleteAllValues deletes all values under the given NK.
func (ke *keyEditor) deleteAllValues(nkRef NKRef, nk hive.NK) error {
	valueCount := nk.ValueCount()
	if valueCount == 0 {
		return nil
	}

	// Read value list
	valueList, err := values.Read(ke.h, nk)
	if err != nil {
		return fmt.Errorf("read value list: %w", err)
	}

	// Delete each value
	for _, vkRef := range valueList.VKRefs {
		if deleteErr := ke.deleteSingleValue(nkRef, vkRef); deleteErr != nil {
			// Continue deleting other values even if one fails
			continue
		}
	}

	// Free the value list cell
	valueListRef := nk.ValueListOffsetRel()
	if valueListRef != format.InvalidOffset {
		// Mark value list as dirty before freeing (size field changes to positive)
		ke.markCellDirty(valueListRef)
		if freeErr := ke.alloc.Free(valueListRef); freeErr != nil {
			return fmt.Errorf("free value list: %w", freeErr)
		}
	}

	return nil
}

// deleteSingleValue deletes a single VK and its associated data.
func (ke *keyEditor) deleteSingleValue(parentNK NKRef, vkRef VKRef) error {
	if vkRef == 0 || vkRef == 0xFFFFFFFF {
		return nil
	}

	// Parse VK
	vkPayload, err := ke.resolveCell(vkRef)
	if err != nil {
		return err
	}

	vk, err := hive.ParseVK(vkPayload)
	if err != nil {
		return err
	}

	// Extract value name before freeing (for index removal)
	vkName := decodeName(vk.Name(), vk.NameCompressed())

	// Free data cells (if external)
	dataLen := vk.DataLen()
	if dataLen > format.DWORDSize {
		// Data is stored externally (not inline in VK)
		dataRef := vk.DataOffsetRel()
		if dataRef != 0 && dataRef != format.InvalidOffset {
			// Check if it's big-data (DB format)
			if freeErr := ke.freeBigDataIfNeeded(dataRef); freeErr != nil {
				// If it's not DB format, just free as single cell
				ke.markCellDirty(dataRef)
				_ = ke.alloc.Free(dataRef)
			}
		}
	}

	// Remove from index to maintain consistency
	ke.index.RemoveVK(parentNK, vkName)

	// Mark VK cell as dirty before freeing (size field changes to positive)
	ke.markCellDirty(vkRef)

	// Free the VK cell itself
	if freeErr := ke.alloc.Free(vkRef); freeErr != nil {
		return freeErr
	}

	return nil
}

// freeBigDataIfNeeded frees a DB structure if the ref points to one.
func (ke *keyEditor) freeBigDataIfNeeded(ref uint32) error {
	return freeBigDataIfNeeded(ke, ke.alloc, ref)
}

// removeFromParentSubkeyList removes a key from its parent's subkey list by NKRef.
// In deferred mode, marks the entry for deletion during flush.
// In immediate mode, uses RemoveByRef which avoids parsing all sibling NK cells.
func (ke *keyEditor) removeFromParentSubkeyList(parentRef NKRef, nkRef uint32) error {
	// DEFERRED MODE: Mark for deletion instead of immediate rewrite
	if ke.deferredMode {
		return ke.removeDeferredChild(parentRef, nkRef)
	}

	// IMMEDIATE MODE: Traditional remove and rewrite
	// Parse parent NK to get subkey list ref
	parentPayload, err := ke.resolveCell(parentRef)
	if err != nil {
		return fmt.Errorf("resolve parent: %w", err)
	}

	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		return fmt.Errorf("parse parent NK: %w", err)
	}

	parentSubkeyListRef := parentNK.SubkeyListOffsetRel()
	if parentSubkeyListRef == format.InvalidOffset {
		return nil
	}

	// Remove by NKRef - avoids parsing all sibling NK cells
	newListRef, newCount, err := subkeys.RemoveByRef(ke.h, ke.alloc, parentSubkeyListRef, nkRef)
	if err != nil {
		return fmt.Errorf("remove from subkey list: %w", err)
	}

	// Re-resolve parent after allocation may have grown hive
	parentPayload, resolveErr := ke.resolveCell(parentRef)
	if resolveErr != nil {
		return fmt.Errorf("re-resolve parent: %w", resolveErr)
	}

	// Update parent NK
	format.PutU32(parentPayload, format.NKSubkeyCountOffset, uint32(newCount))
	format.PutU32(parentPayload, format.NKSubkeyListOffset, newListRef)

	filetime := timeToFiletime(time.Now())
	copy(parentPayload[format.NKLastWriteOffset:format.NKLastWriteOffset+8], filetime[:])

	ke.markCellDirty(parentRef)

	// Free old list
	if parentSubkeyListRef != newListRef && parentSubkeyListRef != format.InvalidOffset {
		_ = ke.alloc.Free(parentSubkeyListRef)
	}

	return nil
}

// removeDeferredChild marks a child for deletion in deferred mode.
// The actual removal happens during FlushDeferredSubkeys.
// OPTIMIZATION: This is lazy - we don't load the subkey list until flush time.
func (ke *keyEditor) removeDeferredChild(parentRef NKRef, childNKRef uint32) error {
	dp, exists := ke.deferredParents[parentRef]
	if !exists {
		// Parent not tracked yet - create a lazy entry (don't load children yet)
		dp = &deferredParent{
			children:      nil, // Will be loaded lazily during flush
			oldListRef:    format.InvalidOffset,
			deletedNKRefs: make(map[uint32]bool),
			loaded:        false, // Mark as not loaded
		}
		ke.deferredParents[parentRef] = dp
	}

	// Initialize deletedNKRefs if nil
	if dp.deletedNKRefs == nil {
		dp.deletedNKRefs = make(map[uint32]bool)
	}

	// Mark the child for deletion
	dp.deletedNKRefs[childNKRef] = true

	// Also remove from in-memory children list if already loaded
	if dp.loaded {
		for i, child := range dp.children {
			if child.NKRef == childNKRef {
				dp.children = append(dp.children[:i], dp.children[i+1:]...)
				break
			}
		}
	}

	return nil
}

// decodeNKName decodes an NK name (ASCII or UTF-16LE).
func decodeNKName(nameBytes []byte, compressed bool) string {
	if compressed {
		// ASCII encoding
		return string(nameBytes)
	}
	// UTF-16LE encoding
	if len(nameBytes)%utf16BytesPerChar != 0 {
		return string(nameBytes) // Fallback to ASCII if odd length
	}
	runes := make([]rune, len(nameBytes)/utf16BytesPerChar)
	for i := range len(nameBytes) / utf16BytesPerChar {
		runes[i] = rune(format.ReadU16(nameBytes, i*utf16BytesPerChar))
	}
	return string(runes)
}

// resolveCell resolves an HCELL_INDEX to its payload.
func (ke *keyEditor) resolveCell(ref uint32) ([]byte, error) {
	if ref == 0 || ref == format.InvalidOffset {
		return nil, ErrInvalidRef
	}

	data := ke.h.Bytes()
	offset := format.HeaderSize + int(ref)

	if offset+format.CellHeaderSize > len(data) {
		return nil, fmt.Errorf("ref 0x%X beyond hive bounds", ref)
	}

	// Read cell size
	cellSize := int32(format.ReadU32(data, offset))
	if cellSize >= 0 {
		return nil, fmt.Errorf("ref 0x%X points to free cell", ref)
	}

	size := int(-cellSize)
	payloadSize := size - format.CellHeaderSize

	payloadOffset := offset + format.CellHeaderSize
	if payloadOffset+payloadSize > len(data) {
		return nil, fmt.Errorf("ref 0x%X payload truncated", ref)
	}

	return data[payloadOffset : payloadOffset+payloadSize], nil
}

// markCellDirty marks a cell as dirty in the dirty tracker.
func (ke *keyEditor) markCellDirty(ref uint32) {
	data := ke.h.Bytes()
	offset := format.HeaderSize + int(ref)

	// Read cell size (including header)
	cellSize := int32(format.ReadU32(data, offset))
	if cellSize < 0 {
		cellSize = -cellSize
	}

	// Mark the entire cell (including header) as dirty
	ke.dt.Add(offset, int(cellSize))
}

// timeToFiletime converts a Go time to Windows FILETIME (100ns intervals since 1601-01-01).
func timeToFiletime(t time.Time) [8]byte {
	// Windows epoch: January 1, 1601
	windowsEpoch := time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)

	// Time since Windows epoch in 100ns intervals
	duration := t.Sub(windowsEpoch)
	intervals := duration.Nanoseconds() / windowsFileTimeTicksPerNanosecond

	var buf [8]byte
	format.PutU64(buf[:], 0, uint64(intervals))
	return buf
}

// normalizeName converts a name to lowercase for case-insensitive comparison.
func normalizeName(name string) string {
	var buf strings.Builder
	buf.Grow(len(name))
	for _, r := range name {
		buf.WriteRune(unicode.ToLower(r))
	}
	return buf.String()
}
