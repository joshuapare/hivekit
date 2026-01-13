package walker

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
)

// CellStats contains statistics about cells in a hive.
type CellStats struct {
	TotalCells uint64

	// By type
	NKCells    uint64
	VKCells    uint64
	SKCells    uint64
	LFCells    uint64
	LHCells    uint64
	LICells    uint64
	RICells    uint64
	DBCells    uint64
	DataCells  uint64
	ValueLists uint64
	Blocklists uint64

	// By purpose
	KeyCells       uint64
	ValueCells     uint64
	SecurityCells  uint64
	SubkeyLists    uint64
	ValueListCells uint64
	ValueDataCells uint64
	ClassNameCells uint64
	BigDataHeaders uint64
	BigDataLists   uint64
	BigDataBlocks  uint64
}

// CellCounter counts cells by type and purpose during traversal.
// This is useful for debugging, validation, and understanding hive structure.
type CellCounter struct {
	*WalkerCore

	stats CellStats
}

// NewCellCounter creates a new cell counter for the given hive.
func NewCellCounter(h *hive.Hive) *CellCounter {
	return &CellCounter{
		WalkerCore: NewWalkerCore(h),
	}
}

// Count traverses the hive and returns statistics about all reachable cells.
//
// Example:
//
//	counter := NewCellCounter(h)
//	stats, err := counter.Count()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Total cells: %d\n", stats.TotalCells)
//	fmt.Printf("NK cells: %d\n", stats.NKCells)
func (cc *CellCounter) Count() (*CellStats, error) {
	// Reset stats
	cc.stats = CellStats{}

	// Use ValidationWalker internally to visit all cells
	validator := &ValidationWalker{
		WalkerCore: cc.WalkerCore,
	}

	err := validator.Walk(func(ref CellRef) error {
		cc.countCell(ref)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &cc.stats, nil
}

// countCell increments counters for a cell reference.
func (cc *CellCounter) countCell(ref CellRef) {
	cc.stats.TotalCells++

	// Count by type
	switch ref.Type {
	case CellTypeNK:
		cc.stats.NKCells++
	case CellTypeVK:
		cc.stats.VKCells++
	case CellTypeSK:
		cc.stats.SKCells++
	case CellTypeLF:
		cc.stats.LFCells++
	case CellTypeLH:
		cc.stats.LHCells++
	case CellTypeLI:
		cc.stats.LICells++
	case CellTypeRI:
		cc.stats.RICells++
	case CellTypeDB:
		cc.stats.DBCells++
	case CellTypeData:
		cc.stats.DataCells++
	case CellTypeValueList:
		cc.stats.ValueLists++
	case CellTypeBlocklist:
		cc.stats.Blocklists++
	}

	// Count by purpose
	switch ref.Purpose {
	case PurposeKey:
		cc.stats.KeyCells++
	case PurposeValue:
		cc.stats.ValueCells++
	case PurposeSecurity:
		cc.stats.SecurityCells++
	case PurposeSubkeyList:
		cc.stats.SubkeyLists++
	case PurposeValueList:
		cc.stats.ValueListCells++
	case PurposeValueData:
		cc.stats.ValueDataCells++
	case PurposeClassName:
		cc.stats.ClassNameCells++
	case PurposeBigDataHeader:
		cc.stats.BigDataHeaders++
	case PurposeBigDataList:
		cc.stats.BigDataLists++
	case PurposeBigDataBlock:
		cc.stats.BigDataBlocks++
	}
}

// String returns a human-readable summary of the cell statistics.
func (cs *CellStats) String() string {
	return fmt.Sprintf(
		"Total: %d cells\n"+
			"By Type:\n"+
			"  NK: %d, VK: %d, SK: %d\n"+
			"  LF: %d, LH: %d, LI: %d, RI: %d\n"+
			"  DB: %d, Data: %d\n"+
			"  ValueLists: %d, Blocklists: %d\n"+
			"By Purpose:\n"+
			"  Keys: %d, Values: %d, Security: %d\n"+
			"  SubkeyLists: %d, ValueLists: %d\n"+
			"  ValueData: %d, ClassNames: %d\n"+
			"  BigData (Headers: %d, Lists: %d, Blocks: %d)",
		cs.TotalCells,
		cs.NKCells, cs.VKCells, cs.SKCells,
		cs.LFCells, cs.LHCells, cs.LICells, cs.RICells,
		cs.DBCells, cs.DataCells,
		cs.ValueLists, cs.Blocklists,
		cs.KeyCells, cs.ValueCells, cs.SecurityCells,
		cs.SubkeyLists, cs.ValueListCells,
		cs.ValueDataCells, cs.ClassNameCells,
		cs.BigDataHeaders, cs.BigDataLists, cs.BigDataBlocks,
	)
}
