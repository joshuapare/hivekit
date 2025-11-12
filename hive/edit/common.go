package edit

import (
	"errors"

	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/bigdata"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// dbSignatureSize is the size of the "db" signature.
	dbSignatureSize = 2

	// dbBlocklistOffset is the offset to the blocklist field in DB headers.
	dbBlocklistOffset = 4
)

// cellResolver is an interface for types that can resolve and mark cells as dirty.
// Both keyEditor and valueEditor implement this interface.
type cellResolver interface {
	resolveCell(ref uint32) ([]byte, error)
	markCellDirty(ref uint32)
}

// freeBigDataIfNeeded frees a DB structure if the ref points to one.
// This is shared logic used by both keyEditor and valueEditor.
func freeBigDataIfNeeded(resolver cellResolver, allocator alloc.Allocator, ref uint32) error {
	payload, err := resolver.resolveCell(ref)
	if err != nil {
		return err
	}

	// Check if it's a DB header (signature "db")
	// Optimized: Use byte comparison instead of string allocation
	if len(payload) >= dbSignatureSize && payload[0] == 'd' && payload[1] == 'b' {
		// Parse DB header
		if len(payload) < bigdata.DBHeaderSize {
			return errors.New("DB header too small")
		}

		count := int(format.ReadU16(payload, format.DBCountOffset))
		blocklistRef := format.ReadU32(payload, dbBlocklistOffset)

		// Free blocklist and data blocks
		if blocklistRef != 0 && blocklistRef != format.InvalidOffset {
			// Read blocklist
			blocklistPayload, resolveErr := resolver.resolveCell(blocklistRef)
			if resolveErr == nil && len(blocklistPayload) >= count*format.DWORDSize {
				// Free each data block
				for i := range count {
					blockRef := format.ReadU32(blocklistPayload, i*format.DWORDSize)
					if blockRef != 0 && blockRef != format.InvalidOffset {
						// Mark data block as dirty before freeing (size field changes to positive)
						resolver.markCellDirty(blockRef)
						_ = allocator.Free(blockRef)
					}
				}
			}

			// Mark blocklist as dirty before freeing
			resolver.markCellDirty(blocklistRef)
			// Free blocklist cell
			_ = allocator.Free(blocklistRef)
		}

		// Mark DB header as dirty before freeing
		resolver.markCellDirty(ref)
		// Free DB header cell
		return allocator.Free(ref)
	}

	// Not a DB structure, free as single cell
	// Mark as dirty before freeing
	resolver.markCellDirty(ref)
	return allocator.Free(ref)
}
