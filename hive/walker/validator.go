package walker

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// CellType identifies the type of cell structure.
type CellType uint8

const (
	CellTypeNK        CellType = 1  // NK - Key/Node
	CellTypeVK        CellType = 2  // VK - Value
	CellTypeSK        CellType = 3  // SK - Security descriptor
	CellTypeLF        CellType = 4  // LF - Subkey list (fast leaf)
	CellTypeLH        CellType = 5  // LH - Subkey list (hash leaf)
	CellTypeLI        CellType = 6  // LI - Subkey list (index)
	CellTypeRI        CellType = 7  // RI - Subkey list (index root)
	CellTypeDB        CellType = 8  // DB - Big data header
	CellTypeData      CellType = 9  // Generic data cell
	CellTypeValueList CellType = 10 // Value list
	CellTypeBlocklist CellType = 11 // Big data blocklist
)

// CellPurpose identifies the role of a cell in the hive structure.
type CellPurpose uint8

const (
	PurposeKey           CellPurpose = 1  // Key/Node structure
	PurposeValue         CellPurpose = 2  // Value structure
	PurposeSecurity      CellPurpose = 3  // Security descriptor
	PurposeSubkeyList    CellPurpose = 4  // Subkey list (any type)
	PurposeValueList     CellPurpose = 5  // Value list
	PurposeValueData     CellPurpose = 6  // Value data
	PurposeClassName     CellPurpose = 7  // Class name data
	PurposeBigDataHeader CellPurpose = 8  // Big data header (DB)
	PurposeBigDataList   CellPurpose = 9  // Big data blocklist
	PurposeBigDataBlock  CellPurpose = 10 // Big data block
)

// CellRef describes a reference to a cell, including its type and purpose.
// This struct is passed to visitor callbacks during traversal.
type CellRef struct {
	Offset  uint32      // Relative offset of the cell (from hive base + 0x1000)
	Type    CellType    // Structural type of the cell
	Purpose CellPurpose // Semantic purpose in the hive hierarchy
}

// ValidationWalker performs depth-first traversal of a hive, visiting all
// reachable cells and invoking a callback for each one.
//
// This walker provides API compatibility with hive.WalkReferences but with
// significantly improved performance through bitmap-based visited tracking
// and iterative traversal.
type ValidationWalker struct {
	*WalkerCore

	visitor func(CellRef) error
}

// NewValidationWalker creates a new validation walker for the given hive.
func NewValidationWalker(h *hive.Hive) *ValidationWalker {
	return &ValidationWalker{
		WalkerCore: NewWalkerCore(h),
	}
}

// Walk traverses the entire hive starting from the root, invoking the visitor
// callback for each reachable cell. The traversal uses depth-first search and
// tracks visited cells to avoid cycles.
//
// The visitor callback receives a CellRef describing each cell's offset, type,
// and purpose. If the visitor returns an error, traversal stops immediately.
//
// Example:
//
//	walker := NewValidationWalker(h)
//	err := walker.Walk(func(ref CellRef) error {
//	    fmt.Printf("Cell at 0x%X: type=%d, purpose=%d\n", ref.Offset, ref.Type, ref.Purpose)
//	    return nil
//	})
func (vw *ValidationWalker) Walk(visitor func(CellRef) error) error {
	vw.visitor = visitor

	rootOffset := vw.h.RootCellOffset()

	// Push root onto stack
	vw.stack = append(vw.stack, StackEntry{offset: rootOffset, state: stateInitial})
	vw.visited.Set(rootOffset)

	// Iterative DFS
	for len(vw.stack) > 0 {
		// Pop from stack
		entry := &vw.stack[len(vw.stack)-1]

		// Process based on state
		switch entry.state {
		case stateInitial:
			// Visit the NK cell
			if err := vw.visitNK(entry.offset); err != nil {
				return err
			}
			entry.state = stateSubkeysDone

		case stateSubkeysDone:
			// Process values
			if err := vw.processValues(entry.offset); err != nil {
				return err
			}
			entry.state = stateValuesDone

		case stateValuesDone:
			// Process security
			if err := vw.processSecurity(entry.offset); err != nil {
				return err
			}
			entry.state = stateSecurityDone

		case stateSecurityDone:
			// Process class name
			if err := vw.processClassName(entry.offset); err != nil {
				return err
			}
			entry.state = stateDone

		case stateDone:
			// Pop this entry and continue
			vw.stack = vw.stack[:len(vw.stack)-1]

		default:
			return fmt.Errorf("invalid walker state: %d", entry.state)
		}
	}

	return nil
}

// visitNK visits an NK cell and processes its subkeys.
func (vw *ValidationWalker) visitNK(offset uint32) error {
	// Invoke visitor for NK cell
	if err := vw.visitor(CellRef{
		Offset:  offset,
		Type:    CellTypeNK,
		Purpose: PurposeKey,
	}); err != nil {
		return err
	}

	// Parse NK
	payload := vw.resolveAndParseCellFast(offset)
	if len(payload) < signatureSize {
		return errors.New("NK cell too small")
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return fmt.Errorf("parse NK at 0x%X: %w", offset, err)
	}

	// Process subkey list
	subkeyListOffset := nk.SubkeyListOffsetRel()
	if subkeyListOffset != format.InvalidOffset {
		if subkeyErr := vw.processSubkeyList(subkeyListOffset); subkeyErr != nil {
			return subkeyErr
		}
	}

	return nil
}

// processSubkeyList visits a subkey list cell and pushes children onto the stack.
func (vw *ValidationWalker) processSubkeyList(listOffset uint32) error {
	// Mark as visited and invoke visitor
	if vw.visited.IsSet(listOffset) {
		return nil
	}
	vw.visited.Set(listOffset)

	payload := vw.resolveAndParseCellFast(listOffset)
	if len(payload) < signatureSize {
		return errors.New("subkey list too small")
	}

	// Optimized: Check signature bytes directly (no string allocation)
	sig0, sig1 := payload[0], payload[1]
	var cellType CellType

	if sig0 == 'l' && sig1 == 'f' {
		cellType = CellTypeLF
	} else if sig0 == 'l' && sig1 == 'h' {
		cellType = CellTypeLH
	} else if sig0 == 'l' && sig1 == 'i' {
		cellType = CellTypeLI
	} else if sig0 == 'r' && sig1 == 'i' {
		cellType = CellTypeRI
	} else {
		return fmt.Errorf("unknown subkey list signature: %c%c", sig0, sig1)
	}

	// Visit the list cell
	if err := vw.visitor(CellRef{
		Offset:  listOffset,
		Type:    cellType,
		Purpose: PurposeSubkeyList,
	}); err != nil {
		return err
	}

	// Walk subkeys and push children onto stack
	return vw.walkSubkeysFast(listOffset)
}

// processValues visits value list and VK cells for a key.
func (vw *ValidationWalker) processValues(nkOffset uint32) error {
	payload := vw.resolveAndParseCellFast(nkOffset)
	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	valueCount := nk.ValueCount()
	if valueCount == 0 {
		return nil
	}

	valueListOffset := nk.ValueListOffsetRel()
	if valueListOffset == format.InvalidOffset {
		return nil
	}

	// Visit value list cell
	if !vw.visited.IsSet(valueListOffset) {
		vw.visited.Set(valueListOffset)

		if visitorErr := vw.visitor(CellRef{
			Offset:  valueListOffset,
			Type:    CellTypeValueList,
			Purpose: PurposeValueList,
		}); visitorErr != nil {
			return visitorErr
		}
	}

	// Get VK offsets
	vkOffsets, err := vw.walkValuesFast(nk)
	if err != nil {
		return err
	}

	// Visit each VK
	for _, vkOffset := range vkOffsets {
		if vw.visited.IsSet(vkOffset) {
			continue
		}
		vw.visited.Set(vkOffset)

		if visitErr := vw.visitVK(vkOffset); visitErr != nil {
			return visitErr
		}
	}

	return nil
}

// visitVK visits a VK cell and its associated data.
func (vw *ValidationWalker) visitVK(vkOffset uint32) error {
	// Visit VK cell
	if err := vw.visitor(CellRef{
		Offset:  vkOffset,
		Type:    CellTypeVK,
		Purpose: PurposeValue,
	}); err != nil {
		return err
	}

	// Parse VK to get data reference
	payload := vw.resolveAndParseCellFast(vkOffset)
	if len(payload) < format.VKFixedHeaderSize {
		return nil // VK too small to have data
	}

	// Data size at offset VKDataLenOffset
	dataSize := format.ReadU32(payload, format.VKDataLenOffset)

	// Data offset at VKDataOffOffset
	dataOffset := format.ReadU32(payload, format.VKDataOffOffset)

	if dataOffset == format.InvalidOffset || dataSize == 0 {
		return nil
	}

	// Check if inline data
	if dataSize&0x80000000 != 0 {
		// Inline data, no cell to visit
		return nil
	}

	// Visit data cell
	if vw.visited.IsSet(dataOffset) {
		return nil
	}
	vw.visited.Set(dataOffset)

	// Check if DB (big-data)
	isDB, err := vw.walkDataCell(dataOffset, dataSize)
	if err != nil {
		return err
	}

	cellType := CellTypeData
	purpose := PurposeValueData

	if isDB {
		cellType = CellTypeDB
		purpose = PurposeBigDataHeader
	}

	return vw.visitor(CellRef{
		Offset:  dataOffset,
		Type:    cellType,
		Purpose: purpose,
	})
}

// processSecurity visits the security descriptor for a key.
func (vw *ValidationWalker) processSecurity(nkOffset uint32) error {
	payload := vw.resolveAndParseCellFast(nkOffset)
	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	skOffset := nk.SecurityOffsetRel()
	if skOffset == format.InvalidOffset {
		return nil
	}

	if vw.visited.IsSet(skOffset) {
		return nil
	}
	vw.visited.Set(skOffset)

	return vw.visitor(CellRef{
		Offset:  skOffset,
		Type:    CellTypeSK,
		Purpose: PurposeSecurity,
	})
}

// processClassName visits the class name data for a key.
func (vw *ValidationWalker) processClassName(nkOffset uint32) error {
	payload := vw.resolveAndParseCellFast(nkOffset)
	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	classOffset := nk.ClassNameOffsetRel()
	classLen := nk.ClassLength()

	if classOffset == 0xFFFFFFFF || classLen == 0 {
		return nil
	}

	if vw.visited.IsSet(classOffset) {
		return nil
	}
	vw.visited.Set(classOffset)

	return vw.visitor(CellRef{
		Offset:  classOffset,
		Type:    CellTypeData,
		Purpose: PurposeClassName,
	})
}
