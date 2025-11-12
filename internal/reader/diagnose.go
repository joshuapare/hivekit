package reader

import (
	"fmt"
	"math"
	"time"

	"github.com/joshuapare/hivekit/internal/buf"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

const (
	// Diagnostic confidence thresholds.
	confidenceLow    = 0.8
	confidenceMedium = 0.9
	confidenceHigh   = 0.95

	// Diagnostic limits.
	minQWORDSize       = 8
	maxIssuesPerReport = 10
)

// diagnosticScanner encapsulates the state for a full diagnostic scan.
type diagnosticScanner struct {
	r             *reader
	report        *types.DiagnosticReport
	visitedCells  map[uint32]bool // Track cells we've seen (detect cycles)
	visitedNodes  map[uint32]bool // Track NK nodes we've traversed
	orphanedCells map[uint32]bool // Cells not referenced by tree
	startTime     time.Time
	cellCount     int
	nodeCount     int
	valueCount    int
}

// newDiagnosticScanner creates a scanner for full hive validation.
func newDiagnosticScanner(r *reader) *diagnosticScanner {
	return &diagnosticScanner{
		r:             r,
		report:        types.NewDiagnosticReport(),
		visitedCells:  make(map[uint32]bool),
		visitedNodes:  make(map[uint32]bool),
		orphanedCells: make(map[uint32]bool),
		startTime:     time.Now(),
	}
}

// scan performs exhaustive hive validation.
func (s *diagnosticScanner) scan() (*types.DiagnosticReport, error) {
	// Phase 1: Validate REGF header
	s.validateREGF()

	// Phase 2: Validate all HBIN structures
	s.validateHBINs()

	// Phase 3: Catalog all cells (for orphan detection)
	s.catalogCells()

	// Phase 4: Walk tree from root
	root := types.NodeID(s.r.head.RootCellOffset)
	s.walkTree(root, "")

	// Phase 5: Detect orphaned cells
	s.detectOrphans()

	// Phase 6: Integrity checks
	s.validateIntegrity()

	// Finalize report
	s.report.FileSize = int64(len(s.r.buf))
	s.report.ScanTime = time.Since(s.startTime)
	s.report.Finalize()

	return s.report, nil
}

// validateREGF validates the REGF header.
func (s *diagnosticScanner) validateREGF() {
	// Header was already validated during Open(), but we can add additional checks
	head := s.r.head

	// Check sequence numbers (should be equal for clean hive)
	if head.PrimarySequence != head.SecondarySequence {
		s.report.Add(diagStructure(
			types.SevWarning,
			uint64(format.REGFPrimarySeqOffset),
			"REGF",
			"Primary and secondary sequence numbers differ - hive may not have been cleanly closed",
			head.PrimarySequence,
			head.SecondarySequence,
			&types.RepairAction{
				Type:        types.RepairDefault,
				Description: "Sync sequence numbers (indicates clean close)",
				Confidence:  confidenceLow,
				Risk:        types.RiskLow,
				AutoApply:   false,
			},
		))
	}

	// Check if root offset is valid
	rootOffset := head.RootCellOffset
	if rootOffset == 0 || rootOffset >= head.HiveBinsDataSize {
		s.report.Add(diagStructure(
			types.SevCritical,
			uint64(format.REGFRootCellOffset),
			"REGF",
			"Root cell offset is invalid",
			fmt.Sprintf("valid offset < %d", head.HiveBinsDataSize),
			rootOffset,
			nil,
		))
	}

	// Check hive bins data size alignment
	if head.HiveBinsDataSize%format.HBINAlignment != 0 {
		s.report.Add(diagStructure(
			types.SevWarning,
			uint64(format.REGFDataSizeOffset),
			"REGF",
			fmt.Sprintf("Hive bins data size not aligned to 0x%x", format.HBINAlignment),
			"aligned size",
			head.HiveBinsDataSize,
			nil,
		))
	}
}

// validateHBINs validates all HBIN structures.
func (s *diagnosticScanner) validateHBINs() {
	offset := int(format.HeaderSize)
	dataEnd := int(format.HeaderSize) + int(s.r.head.HiveBinsDataSize)
	hbinIndex := 0

	for offset < dataEnd && offset < len(s.r.buf) {
		hbin, next, err := format.NextHBIN(s.r.buf, offset)
		if err != nil {
			s.report.Add(diagStructure(
				types.SevCritical,
				uint64(offset),
				"HBIN",
				fmt.Sprintf("HBIN %d validation failed: %v", hbinIndex, err),
				"valid HBIN structure",
				"corrupted or invalid",
				nil,
			))
			break // Can't continue if HBIN is corrupt
		}

		// Check HBIN file offset matches actual position
		expectedFileOffset := uint32(offset - int(format.HeaderSize))
		if hbin.FileOffset != expectedFileOffset {
			s.report.Add(diagStructure(
				types.SevError,
				uint64(offset+format.HBINFileOffsetField),
				"HBIN",
				fmt.Sprintf("HBIN %d file offset mismatch", hbinIndex),
				expectedFileOffset,
				hbin.FileOffset,
				&types.RepairAction{
					Type:        types.RepairReplace,
					Description: fmt.Sprintf("Update HBIN file offset to 0x%x", expectedFileOffset),
					Confidence:  1.0,
					Risk:        types.RiskLow,
					AutoApply:   true,
				},
			))
		}

		offset = next
		hbinIndex++
	}
}

// catalogCells walks all HBINs and catalogs every cell.
func (s *diagnosticScanner) catalogCells() {
	offset := int(format.HeaderSize)
	dataEnd := int(format.HeaderSize) + int(s.r.head.HiveBinsDataSize)

	for offset < dataEnd && offset < len(s.r.buf) {
		hbin, next, err := format.NextHBIN(s.r.buf, offset)
		if err != nil {
			break // Already reported in validateHBINs
		}

		// Walk cells within this HBIN
		cellStart := offset + format.HBINHeaderSize
		cellEnd := offset + int(hbin.Size)

		for cellStart < cellEnd {
			// Parse cell header (4 bytes: size)
			if cellStart+4 > len(s.r.buf) {
				break
			}

			cellSize := int32(buf.U32LE(s.r.buf[cellStart : cellStart+4]))
			actualSize := cellSize
			isAllocated := cellSize < 0
			if isAllocated {
				actualSize = -actualSize // Allocated cell
			}

			if actualSize < minQWORDSize {
				// Invalid cell size
				s.report.Add(diagStructure(
					types.SevError,
					uint64(cellStart),
					"CELL",
					fmt.Sprintf("Invalid cell size: %d", actualSize),
					fmt.Sprintf(">= %d", minQWORDSize),
					actualSize,
					nil,
				))
				break // Can't continue parsing this HBIN
			}

			// Record allocated cells for orphan detection (skip free cells)
			if isAllocated {
				cellOffset := uint32(cellStart - int(format.HeaderSize))
				s.orphanedCells[cellOffset] = true
				s.cellCount++
			}

			// Move to next cell
			cellStart += int(actualSize)
		}

		offset = next
	}
}

// walkTree traverses the NK tree from the given node.
func (s *diagnosticScanner) walkTree(nodeID types.NodeID, path string) {
	offset := uint32(nodeID)

	// Check for cycles
	if s.visitedNodes[offset] {
		s.report.Add(diagIntegrity(
			types.SevError,
			uint64(format.HeaderSize)+uint64(offset),
			"NK",
			"Cycle detected at "+path,
			"acyclic tree",
			"cycle",
			&types.DiagContext{KeyPath: path, CellOffset: offset},
			nil,
		))
		return
	}
	s.visitedNodes[offset] = true

	// Mark this cell as referenced (not orphaned)
	delete(s.orphanedCells, offset)

	// Try to read NK
	nk, err := s.r.nk(nodeID)
	if err != nil {
		s.report.Add(diagData(
			types.SevError,
			uint64(format.HeaderSize)+uint64(offset),
			"NK",
			fmt.Sprintf("Failed to read NK at %s: %v", path, err),
			"valid NK record",
			"corrupted or invalid",
			&types.DiagContext{KeyPath: path, CellOffset: offset},
			nil,
		))
		return
	}
	s.nodeCount++

	// Get key name for path building
	name, err := DecodeKeyName(nk)
	if err != nil {
		name = fmt.Sprintf("(corrupt_name_0x%x)", offset)
	}
	if path != "" {
		path += "\\"
	}
	path += name

	// Validate NK fields
	s.validateNK(nk, nodeID, path)

	// Mark value list cell as referenced (if present)
	if nk.ValueCount > 0 && nk.ValueListOffset != format.InvalidOffset {
		delete(s.orphanedCells, nk.ValueListOffset)
	}

	// Mark subkey list cell as referenced (if present)
	if nk.SubkeyCount > 0 && nk.SubkeyListOffset != format.InvalidOffset {
		delete(s.orphanedCells, nk.SubkeyListOffset)
	}

	// Walk values
	s.walkValues(nodeID, path)

	// Walk subkeys
	subkeys, err := s.r.Subkeys(nodeID)
	if err != nil {
		// Error accessing subkeys - already recorded or benign
		return
	}

	for _, childID := range subkeys {
		s.walkTree(childID, path)
	}
}

// validateNK validates NK record fields.
func (s *diagnosticScanner) validateNK(nk format.NKRecord, nodeID types.NodeID, path string) {
	offset := uint32(nodeID)
	ctx := &types.DiagContext{KeyPath: path, CellOffset: offset}

	// Mark security cell as referenced (if present)
	if nk.SecurityOffset != format.InvalidOffset && nk.SecurityOffset != 0 {
		delete(s.orphanedCells, nk.SecurityOffset)
	}

	// Mark class name cell as referenced (if present)
	if nk.ClassNameOffset != format.InvalidOffset && nk.ClassNameOffset != 0 {
		delete(s.orphanedCells, nk.ClassNameOffset)
	}

	// Check timestamp (should be reasonable)
	if nk.LastWriteRaw == 0 {
		s.report.Add(diagData(
			types.SevInfo,
			uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKLastWriteOffset),
			"NK",
			"Key has zero timestamp",
			"non-zero timestamp",
			0,
			ctx,
			nil,
		))
	}

	// Check subkey count vs actual count
	if nk.SubkeyCount > 0 && nk.SubkeyListOffset == math.MaxUint32 {
		s.report.Add(diagIntegrity(
			types.SevError,
			uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKSubkeyCountOffset),
			"NK",
			"Subkey count > 0 but list offset is invalid",
			uint32(0), // Expected: subkey count should be 0
			nk.SubkeyCount,
			ctx,
			&types.RepairAction{
				Type:        types.RepairReplace,
				Description: "Set subkey count to 0",
				Confidence:  confidenceMedium,
				Risk:        types.RiskLow,
				AutoApply:   true,
			},
		))
	}

	// Check value count vs actual count
	if nk.ValueCount > 0 && nk.ValueListOffset == format.InvalidOffset {
		s.report.Add(diagIntegrity(
			types.SevError,
			uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKValueCountOffset),
			"NK",
			"Value count > 0 but list offset is invalid",
			uint32(0), // Expected: value count should be 0
			nk.ValueCount,
			ctx,
			&types.RepairAction{
				Type:        types.RepairReplace,
				Description: "Set value count to 0",
				Confidence:  confidenceMedium,
				Risk:        types.RiskLow,
				AutoApply:   true,
			},
		))
	}

	// Check for dangling subkey list offset (count=0 but offset set)
	if nk.SubkeyCount == 0 && nk.SubkeyListOffset != format.InvalidOffset && nk.SubkeyListOffset != 0 {
		s.report.Add(diagIntegrity(
			types.SevWarning,
			uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKSubkeyListOffset),
			"NK",
			fmt.Sprintf("Subkey count is 0 but list offset is set to 0x%X (should be 0xFFFFFFFF)", nk.SubkeyListOffset),
			format.InvalidOffset,
			nk.SubkeyListOffset,
			ctx,
			&types.RepairAction{
				Type:        types.RepairDefault,
				Description: "Set subkey list offset to InvalidOffset (0xFFFFFFFF)",
				Confidence:  confidenceHigh,
				Risk:        types.RiskLow,
				AutoApply:   true,
			},
		))
	}

	// Check for dangling value list offset (count=0 but offset set)
	if nk.ValueCount == 0 && nk.ValueListOffset != format.InvalidOffset && nk.ValueListOffset != 0 {
		s.report.Add(diagIntegrity(
			types.SevWarning,
			uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKValueListOffset),
			"NK",
			fmt.Sprintf("Value count is 0 but list offset is set to 0x%X (should be 0xFFFFFFFF)", nk.ValueListOffset),
			format.InvalidOffset,
			nk.ValueListOffset,
			ctx,
			&types.RepairAction{
				Type:        types.RepairDefault,
				Description: "Set value list offset to InvalidOffset (0xFFFFFFFF)",
				Confidence:  confidenceHigh,
				Risk:        types.RiskLow,
				AutoApply:   true,
			},
		))
	}

	// Check subkey list offset bounds (if non-zero and not invalid)
	if nk.SubkeyCount > 0 && nk.SubkeyListOffset != format.InvalidOffset {
		maxOffset := s.r.head.HiveBinsDataSize
		if nk.SubkeyListOffset >= maxOffset {
			s.report.Add(diagIntegrity(
				types.SevError,
				uint64(
					format.HeaderSize,
				)+uint64(
					offset,
				)+uint64(
					format.CellHeaderSize,
				)+uint64(
					format.NKSubkeyListOffset,
				),
				"NK",
				fmt.Sprintf("Subkey list offset 0x%X exceeds hive size 0x%X", nk.SubkeyListOffset, maxOffset),
				fmt.Sprintf("< 0x%X", maxOffset),
				nk.SubkeyListOffset,
				ctx,
				nil,
			))
		}
	}

	// Check value list offset bounds (if non-zero and not invalid)
	if nk.ValueCount > 0 && nk.ValueListOffset != format.InvalidOffset {
		maxOffset := s.r.head.HiveBinsDataSize
		if nk.ValueListOffset >= maxOffset {
			s.report.Add(diagIntegrity(
				types.SevError,
				uint64(format.HeaderSize)+uint64(offset)+uint64(format.CellHeaderSize)+uint64(format.NKValueListOffset),
				"NK",
				fmt.Sprintf("Value list offset 0x%X exceeds hive size 0x%X", nk.ValueListOffset, maxOffset),
				fmt.Sprintf("< 0x%X", maxOffset),
				nk.ValueListOffset,
				ctx,
				nil,
			))
		}
	}
}

// walkValues validates all values for a key.
func (s *diagnosticScanner) walkValues(nodeID types.NodeID, path string) {
	values, err := s.r.Values(nodeID)
	if err != nil {
		// Error already handled
		return
	}

	for _, vid := range values {
		s.valueCount++
		vOffset := uint32(vid)
		delete(s.orphanedCells, vOffset) // Mark as referenced

		// Try to read value metadata
		_, statErr := s.r.StatValue(vid)
		if statErr != nil {
			s.report.Add(diagData(
				types.SevError,
				uint64(format.HeaderSize)+uint64(vOffset),
				"VK",
				fmt.Sprintf("Failed to read VK: %v", statErr),
				"valid VK record",
				"corrupted",
				&types.DiagContext{KeyPath: path, CellOffset: vOffset},
				nil,
			))
			continue
		}

		// Try to read value data
		_, err = s.r.ValueBytes(vid, types.ReadOptions{})
		if err != nil {
			// Error reading data - may be truncated or corrupt
			// (already recorded by passive diagnostics if enabled)
		}
	}
}

// detectOrphans identifies cells not referenced by the tree.
func (s *diagnosticScanner) detectOrphans() {
	orphanCount := len(s.orphanedCells)
	if orphanCount > 0 {
		// Report summary of orphaned cells
		s.report.Add(types.Diagnostic{
			Severity:  types.SevWarning,
			Category:  types.DiagIntegrity,
			Structure: "HIVE",
			Issue:     fmt.Sprintf("Found %d orphaned cells not referenced by tree", orphanCount),
			Expected:  "all cells referenced",
			Actual:    orphanCount,
		})

		// Report first few orphans (don't spam with thousands)
		count := 0
		for offset := range s.orphanedCells {
			if count >= maxIssuesPerReport {
				break
			}
			s.report.Add(diagIntegrity(
				types.SevInfo,
				uint64(format.HeaderSize)+uint64(offset),
				"CELL",
				"Orphaned cell not referenced by tree",
				"referenced",
				"orphaned",
				nil,
				nil,
			))
			count++
		}
	}
}

// validateIntegrity performs final integrity checks.
func (s *diagnosticScanner) validateIntegrity() {
	// No additional checks for now
	// The scan stats are captured in the report's FileSize and ScanTime fields
}
