package walker

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// utf16BytesPerChar is the number of bytes per UTF-16 character.
	utf16BytesPerChar = 2
)

// IndexBuilder builds a (parent, name) → offset index optimized for merge
// operations. This walker is specialized for index building and bypasses the
// visitor callback overhead for maximum performance.
//
// The resulting index enables O(1) key lookups by path, which is exactly what
// the merge executor needs.
type IndexBuilder struct {
	*WalkerCore

	idx index.Index
}

// NewIndexBuilder creates a new index builder for the given hive.
// The capacity hints help pre-size the index to reduce allocations during building.
func NewIndexBuilder(h *hive.Hive, nkCapacity, vkCapacity int) *IndexBuilder {
	return &IndexBuilder{
		WalkerCore: NewWalkerCore(h),
		idx:        index.NewStringIndex(nkCapacity, vkCapacity),
	}
}

// Build traverses the hive and builds a complete index of all keys and values.
// Returns the built index and any error encountered during traversal.
//
// Example:
//
//	builder := NewIndexBuilder(h, 10000, 10000)
//	idx, err := builder.Build()
//	if err != nil {
//	    return err
//	}
//	// Now idx can be used for O(1) lookups
//	offset, found := idx.LookupNK(parentOffset, "SubkeyName")
func (ib *IndexBuilder) Build() (index.Index, error) {
	rootOffset := ib.h.RootCellOffset()

	// Push root onto stack (parent is itself for root)
	ib.stack = append(ib.stack, StackEntry{offset: rootOffset, state: stateInitial})
	ib.visited.Set(rootOffset)

	// Index root with empty name
	ib.idx.AddNK(rootOffset, "", rootOffset)

	// Iterative DFS
	for len(ib.stack) > 0 {
		// Pop from stack
		entry := &ib.stack[len(ib.stack)-1]

		// Process based on state
		switch entry.state {
		case stateInitial:
			// Process this NK and its subkeys
			if err := ib.processNK(entry.offset); err != nil {
				return nil, err
			}
			entry.state = stateValuesDone

		case stateValuesDone:
			// Process values for this NK
			if err := ib.processValues(entry.offset); err != nil {
				return nil, err
			}
			entry.state = stateDone

		case stateDone:
			// Pop this entry and continue
			ib.stack = ib.stack[:len(ib.stack)-1]

		default:
			return nil, fmt.Errorf("invalid builder state: %d", entry.state)
		}
	}

	return ib.idx, nil
}

// processNK processes an NK cell, indexing its subkeys and pushing them onto the stack.
func (ib *IndexBuilder) processNK(nkOffset uint32) error {
	payload := ib.resolveAndParseCellFast(nkOffset)
	if len(payload) < signatureSize {
		return fmt.Errorf("NK cell too small at offset 0x%X", nkOffset)
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return fmt.Errorf("parse NK at 0x%X: %w", nkOffset, err)
	}

	// Get subkey list
	subkeyListOffset := nk.SubkeyListOffsetRel()
	if subkeyListOffset == format.InvalidOffset {
		return nil // No subkeys
	}

	// Read subkey list to get entries
	subkeyList, err := subkeys.Read(ib.h, subkeyListOffset)
	if err != nil {
		return fmt.Errorf("read subkey list at 0x%X: %w", subkeyListOffset, err)
	}

	// Index and push each subkey
	for _, entry := range subkeyList.Entries {
		childOffset := entry.NKRef
		nameLower := entry.NameLower

		// Index this child under its parent
		ib.idx.AddNK(nkOffset, nameLower, childOffset)

		// Push child onto stack if not visited
		if !ib.visited.IsSet(childOffset) {
			ib.visited.Set(childOffset)
			ib.stack = append(ib.stack, StackEntry{offset: childOffset, state: stateInitial})
		}
	}

	return nil
}

// processValues indexes all values for a key.
func (ib *IndexBuilder) processValues(nkOffset uint32) error {
	payload := ib.resolveAndParseCellFast(nkOffset)
	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	valueCount := nk.ValueCount()
	if valueCount == 0 {
		return nil
	}

	// Get VK offsets
	vkOffsets, err := ib.walkValuesFast(nk)
	if err != nil {
		return err
	}

	// Index each value (skip malformed values to maintain robustness)
	for _, vkOffset := range vkOffsets {
		if indexErr := ib.indexValue(nkOffset, vkOffset); indexErr != nil {
			// Skip malformed values rather than failing the entire build
			// This allows the walker to handle corrupted hives gracefully
			continue
		}
	}

	return nil
}

// indexValue indexes a single value under its parent key.
func (ib *IndexBuilder) indexValue(parentOffset, vkOffset uint32) error {
	payload := ib.resolveAndParseCellFast(vkOffset)
	if len(payload) < format.VKFixedHeaderSize {
		return fmt.Errorf("VK cell too small at offset 0x%X", vkOffset)
	}

	// Get value name
	// Name length at offset VKNameLenOffset
	nameLen := format.ReadU16(payload, format.VKNameLenOffset)

	// Flags at offset VKFlagsOffset (NOT 0x00 which is the signature!)
	flags := format.ReadU16(payload, format.VKFlagsOffset)

	// Name starts at offset VKNameOffset
	if len(payload) < format.VKFixedHeaderSize+int(nameLen) {
		return fmt.Errorf("VK name truncated at offset 0x%X", vkOffset)
	}

	nameBytes := payload[20 : 20+nameLen]

	// Check if compressed (ASCII) or uncompressed (UTF-16LE)
	var name string
	if flags&0x0001 != 0 {
		// Compressed (ASCII)
		name = string(nameBytes)
	} else {
		// Uncompressed (UTF-16LE) - convert to string
		// Validate that UTF-16LE name has even length
		if nameLen%utf16BytesPerChar != 0 {
			return fmt.Errorf("invalid UTF-16LE name at offset 0x%X: odd length %d", vkOffset, nameLen)
		}

		// Simple conversion: take every other byte (assumes BMP only)
		nameRunes := make([]rune, nameLen/utf16BytesPerChar)
		for i := range nameRunes {
			char := format.ReadU16(nameBytes, i*utf16BytesPerChar)
			nameRunes[i] = rune(char)
		}
		name = string(nameRunes)
	}

	// Index this value (index handles case-insensitivity internally)
	ib.idx.AddVK(parentOffset, name, vkOffset)

	return nil
}
