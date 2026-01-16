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

	// Reusable slices for zero-allocation offset extraction.
	// These are reused across all processNK and processValues calls.
	vkOffsetsBuf    []uint32 // For VK offset extraction in walkValuesFastCached
	childOffsetsBuf []uint32 // For child offset extraction in processNK
}

// estimateIndexCapacity estimates NK/VK counts from hive size.
// Based on empirical analysis of test suite hives:
//   - ~1 NK per 256-512 bytes of hive data
//   - ~2-4 VKs per NK
//
// This helps pre-size maps to reduce rehashing overhead (which can be 8+ seconds
// for large hives according to profiling).
func estimateIndexCapacity(hiveSize int64) (nkCap, vkCap int) {
	// Conservative estimate: 1 NK per 300 bytes
	nkCap = int(hiveSize / 300)
	vkCap = nkCap * 3

	// Ensure minimums for small hives
	if nkCap < 1024 {
		nkCap = 1024
	}
	if vkCap < 4096 {
		vkCap = 4096
	}

	return nkCap, vkCap
}

// NewIndexBuilder creates a new index builder for the given hive.
// The capacity hints help pre-size the index to reduce allocations during building.
// Uses NumericIndex by default (zero-allocation, faster).
//
// If nkCapacity or vkCapacity is 0, capacity will be estimated from hive size.
func NewIndexBuilder(h *hive.Hive, nkCapacity, vkCapacity int) *IndexBuilder {
	// If hints are default/zero, use estimation from hive size
	if nkCapacity == 0 || vkCapacity == 0 {
		nkCapacity, vkCapacity = estimateIndexCapacity(h.Size())
	}
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

	// Push root onto stack (parent is itself for root - special marker)
	ib.stack = append(ib.stack, StackEntry{
		offset:       rootOffset,
		parentOffset: rootOffset, // Root's parent is itself (signals: don't add to index)
		state:        stateInitial,
	})
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

// processNK processes an NK cell using deferred name decoding.
//
// Key optimization: Instead of decoding ALL child names when reading a subkey list,
// each NK decodes its OWN name when processed. This eliminates the ~68% allocation
// overhead from subkeys.Read() that was decoding names for all children at once.
//
// Flow:
//  1. Parse this NK and cache value info
//  2. Index THIS NK under its parent (using parentOffset from StackEntry)
//  3. Read only child offsets (no name decoding)
//  4. Push children to stack with parentOffset = this NK
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

	// Index THIS NK under its parent (skip for root - root is already indexed in Build())
	parentOffset := entry.parentOffset
	if parentOffset != nkOffset {
		// Decode this NK's name and add to index
		nameBytes := nk.Name()
		if nk.IsCompressedName() {
			// ASCII name - use zero-allocation hash path
			numIdx, ok := ib.idx.(*index.NumericIndex)
			if ok {
				hash := index.Fnv32LowerBytes(nameBytes)
				numIdx.AddNKHashFast(parentOffset, hash, nkOffset)
			} else {
				nameLower := decodeASCIILower(nameBytes)
				ib.idx.AddNKLower(parentOffset, nameLower, nkOffset)
			}
		} else {
			// UTF-16LE name - need string conversion
			nameLower := decodeUTF16LELower(nameBytes)
			ib.idx.AddNKLower(parentOffset, nameLower, nkOffset)
		}
	}

	// Get subkey list offset
	subkeyListOffset := nk.SubkeyListOffsetRel()
	if subkeyListOffset == format.InvalidOffset {
		return nil // No subkeys
	}

	// Read only child offsets (no name decoding) using reusable buffer
	var readErr error
	ib.childOffsetsBuf, readErr = subkeys.ReadOffsetsInto(ib.h, subkeyListOffset, ib.childOffsetsBuf)
	if readErr != nil {
		return fmt.Errorf("read subkey offsets at 0x%X: %w", subkeyListOffset, readErr)
	}

	// Push children onto stack with this NK as their parent
	for _, childOffset := range ib.childOffsetsBuf {
		if !ib.visited.IsSet(childOffset) {
			ib.visited.Set(childOffset)
			ib.stack = append(ib.stack, StackEntry{
				offset:       childOffset,
				parentOffset: nkOffset, // Child will index itself under this NK
				state:        stateInitial,
			})
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

	// Get VK offsets using cached value list offset (reusable buffer)
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
// Uses the IndexBuilder's reusable vkOffsetsBuf slice for zero-allocation.
// The returned slice is only valid until the next call to walkValuesFastCached.
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

	// Reuse the IndexBuilder's buffer (reset to zero length, keep capacity)
	ib.vkOffsetsBuf = ib.vkOffsetsBuf[:0]

	// Ensure capacity (will allocate only once if initial capacity is exceeded)
	if cap(ib.vkOffsetsBuf) < int(valueCount) {
		ib.vkOffsetsBuf = make([]uint32, 0, valueCount)
	}

	// Extract VK offsets
	for i := range valueCount {
		off := int(i) * format.DWORDSize
		vkOffset, err := format.CheckedReadU32(payload, off)
		if err != nil {
			// Skip malformed entry, continue processing
			continue
		}
		if vkOffset != 0 && vkOffset != format.InvalidOffset {
			ib.vkOffsetsBuf = append(ib.vkOffsetsBuf, vkOffset)
		}
	}

	return ib.vkOffsetsBuf, nil
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

	if flags&0x0001 != 0 {
		// Compressed (ASCII) - use zero-allocation hash-based path
		// This avoids string allocation entirely in the common case
		numIdx, ok := ib.idx.(*index.NumericIndex)
		if ok {
			hash := index.Fnv32LowerBytes(nameBytes)
			numIdx.AddVKHashFast(parentOffset, hash, vkOffset)
			return nil
		}
		// Fallback for non-numeric index
		nameLower := decodeASCIILower(nameBytes)
		ib.idx.AddVKLower(parentOffset, nameLower, vkOffset)
	} else {
		// Uncompressed (UTF-16LE) - still need to decode to string
		if nameLen%utf16BytesPerChar != 0 {
			return fmt.Errorf("invalid UTF-16LE name at offset 0x%X: odd length %d", vkOffset, nameLen)
		}
		nameLower := decodeUTF16LELower(nameBytes)
		ib.idx.AddVKLower(parentOffset, nameLower, vkOffset)
	}

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
