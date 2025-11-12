package subkeys

// Entry represents a single subkey list entry.
// Each entry contains a lowercased key name and a reference to the NK cell.
type Entry struct {
	NameLower string // Lowercased key name for case-insensitive comparison
	NKRef     uint32 // HCELL_INDEX reference to the NK (key node) cell
	Hash      uint32 // Cached Windows Registry hash (computed during decode)
}

// List represents a subkey list with its entries.
// The list can be encoded as LF (fast leaf), LH (hash leaf), LI (indexed), or RI (indirect).
type List struct {
	Entries []Entry
}

// Len returns the number of entries in the list.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.Entries)
}

// ListKind represents the type of subkey list.
type ListKind uint8

const (
	KindLI ListKind = 1 // Indexed list (no hash)
	KindLF ListKind = 2 // Fast leaf (with 4-byte hash, basic hash)
	KindLH ListKind = 3 // Hash leaf (with 4-byte hash, improved hash)
	KindRI ListKind = 4 // Indirect list (list of list references)
)
