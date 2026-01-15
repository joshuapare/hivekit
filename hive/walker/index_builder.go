package walker

import (
	"context"
	"fmt"
	"sync"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// runeBufferPool pools rune buffers for UTF-16 value name decoding.
// This eliminates per-value allocations in the hot path.
// Most value names are short (< 64 chars), so we pre-allocate 128 runes.
var runeBufferPool = sync.Pool{
	New: func() any {
		b := make([]rune, 0, 128)
		return &b
	},
}

// byteBufferPool pools byte buffers for ASCII lowercase conversion.
// Most value names are short (< 64 bytes).
var byteBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 64)
		return &b
	},
}

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
// Uses NumericIndex by default (zero-allocation, faster).
func NewIndexBuilder(h *hive.Hive, nkCapacity, vkCapacity int) *IndexBuilder {
	return NewIndexBuilderWithKind(h, nkCapacity, vkCapacity, index.IndexNumeric)
}

// NewIndexBuilderWithKind creates an index builder with the specified index implementation.
//
// IndexKind options:
//   - index.IndexNumeric: Zero-allocation uint64 map keys (recommended, default)
//   - index.IndexString: Traditional string map keys (useful for debugging)
func NewIndexBuilderWithKind(h *hive.Hive, nkCapacity, vkCapacity int, kind index.IndexKind) *IndexBuilder {
	return &IndexBuilder{
		WalkerCore: NewWalkerCore(h),
		idx:        index.NewIndex(kind, nkCapacity, vkCapacity),
	}
}

// Build traverses the hive and builds a complete index of all keys and values.
// Returns the built index and any error encountered during traversal.
//
// The context can be used to cancel the build operation early. If the context
// is cancelled, Build returns the context error (context.Canceled or
// context.DeadlineExceeded).
//
// Example:
//
//	builder := NewIndexBuilder(h, 10000, 10000)
//	idx, err := builder.Build(ctx)
//	if err != nil {
//	    return err
//	}
//	// Now idx can be used for O(1) lookups
//	offset, found := idx.LookupNK(parentOffset, "SubkeyName")
func (ib *IndexBuilder) Build(ctx context.Context) (index.Index, error) {
	rootOffset := ib.h.RootCellOffset()

	// Push root onto stack (parent is itself for root)
	ib.stack = append(ib.stack, StackEntry{offset: rootOffset, state: stateInitial})
	ib.visited.Set(rootOffset)

	// Index root with empty name
	ib.idx.AddNK(rootOffset, "", rootOffset)

	// Iterative DFS
	for len(ib.stack) > 0 {
		// Check for cancellation at the start of each iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

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
// It also caches value info in the current StackEntry to avoid redundant NK parsing in processValues.
func (ib *IndexBuilder) processNK(nkOffset uint32) error {
	payload := ib.resolveAndParseCellFast(nkOffset)
	if len(payload) < signatureSize {
		return fmt.Errorf("NK cell too small at offset 0x%X", nkOffset)
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return fmt.Errorf("parse NK at 0x%X: %w", nkOffset, err)
	}

	// Cache value info in StackEntry to avoid re-parsing NK in processValues
	entry := &ib.stack[len(ib.stack)-1]
	entry.valueCount = nk.ValueCount()
	entry.valueListOffset = nk.ValueListOffsetRel()

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

		// Index this child under its parent (use AddNKLower since subkeys.Read
		// already returns lowercased names - avoids redundant strings.ToLower)
		ib.idx.AddNKLower(nkOffset, nameLower, childOffset)

		// Push child onto stack if not visited
		if !ib.visited.IsSet(childOffset) {
			ib.visited.Set(childOffset)
			ib.stack = append(ib.stack, StackEntry{offset: childOffset, state: stateInitial})
		}
	}

	return nil
}

// processValues indexes all values for a key.
// Uses cached value info from StackEntry (set by processNK) to avoid redundant NK parsing.
func (ib *IndexBuilder) processValues(nkOffset uint32) error {
	// Use cached value info from StackEntry (set by processNK)
	entry := &ib.stack[len(ib.stack)-1]
	valueCount := entry.valueCount
	if valueCount == 0 {
		return nil
	}

	// Get VK offsets using cached value list offset
	vkOffsets, err := ib.walkValuesFastCached(entry.valueListOffset, valueCount)
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

// walkValuesFastCached processes a value list using cached offset and count.
// This avoids the need to parse the NK cell again to get these values.
func (ib *IndexBuilder) walkValuesFastCached(listOffset, valueCount uint32) ([]uint32, error) {
	if valueCount == 0 || listOffset == format.InvalidOffset {
		return nil, nil
	}

	// Sanity check: value count
	if valueCount > format.MaxValueCount {
		return nil, fmt.Errorf("value count %d exceeds limit: %w", valueCount, format.ErrSanityLimit)
	}

	payload := ib.resolveAndParseCellFast(listOffset)
	if payload == nil {
		return nil, fmt.Errorf("value list cell not found at offset 0x%X", listOffset)
	}

	// Check bounds with overflow protection
	needed := int(valueCount) * format.DWORDSize
	if needed < 0 || needed > len(payload) {
		return nil, fmt.Errorf("value list too small: need %d bytes, have %d", needed, len(payload))
	}

	// Extract VK offsets
	vkOffsets := make([]uint32, 0, valueCount)
	for i := range valueCount {
		off := int(i) * format.DWORDSize
		vkOffset, err := format.CheckedReadU32(payload, off)
		if err != nil {
			// Skip malformed entry, continue processing
			continue
		}
		if vkOffset != 0 && vkOffset != format.InvalidOffset {
			vkOffsets = append(vkOffsets, vkOffset)
		}
	}

	return vkOffsets, nil
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

	// Decode and lowercase value name in one pass (fused operation)
	// This avoids the separate strings.ToLower call in AddVK
	var nameLower string
	if flags&0x0001 != 0 {
		// Compressed (ASCII) - fused decode+lowercase
		nameLower = decodeASCIILower(nameBytes)
	} else {
		// Uncompressed (UTF-16LE) - fused decode+lowercase
		if nameLen%utf16BytesPerChar != 0 {
			return fmt.Errorf("invalid UTF-16LE name at offset 0x%X: odd length %d", vkOffset, nameLen)
		}
		nameLower = decodeUTF16LELower(nameBytes)
	}

	// Index this value with pre-lowercased name (avoids redundant ToLower)
	ib.idx.AddVKLower(parentOffset, nameLower, vkOffset)

	return nil
}

// decodeASCIILower decodes ASCII bytes to a lowercase string.
// Uses a fast path for already-lowercase names (zero-copy).
func decodeASCIILower(data []byte) string {
	// Fast path: check if already lowercase
	hasUpper := false
	for _, c := range data {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
			break
		}
	}

	if !hasUpper {
		// Already lowercase - return directly (zero allocation for common case)
		return string(data)
	}

	// Need to lowercase - use pooled buffer
	bufPtr := byteBufferPool.Get().(*[]byte)
	buf := *bufPtr

	if cap(buf) < len(data) {
		buf = make([]byte, len(data))
	} else {
		buf = buf[:len(data)]
	}

	for i, c := range data {
		if c >= 'A' && c <= 'Z' {
			buf[i] = c + ('a' - 'A')
		} else {
			buf[i] = c
		}
	}

	result := string(buf)

	*bufPtr = buf[:0]
	byteBufferPool.Put(bufPtr)

	return result
}

// decodeUTF16LELower decodes UTF-16LE bytes to a lowercase string.
// Uses pooled rune buffer and lowercases during decode.
func decodeUTF16LELower(data []byte) string {
	runeCount := len(data) / utf16BytesPerChar

	bufPtr := runeBufferPool.Get().(*[]rune)
	buf := *bufPtr

	if cap(buf) < runeCount {
		buf = make([]rune, runeCount)
	} else {
		buf = buf[:runeCount]
	}

	// Decode and lowercase in one pass
	for i := range runeCount {
		char := rune(format.ReadU16(data, i*utf16BytesPerChar))
		// Lowercase ASCII range (most common case)
		if char >= 'A' && char <= 'Z' {
			char = char + ('a' - 'A')
		}
		buf[i] = char
	}

	result := string(buf)

	*bufPtr = buf[:0]
	runeBufferPool.Put(bufPtr)

	return result
}
