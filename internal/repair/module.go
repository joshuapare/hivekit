package repair



// RepairModule defines the interface that all repair modules must implement.
// Each module is responsible for repairing a specific type of structure
// (REGF, HBIN, NK, VK, etc.) and ensures repairs are safe and correct.
type RepairModule interface {
	// Name returns the module identifier (e.g., "REGF", "NK", "VK").
	// This is used for logging and debugging.
	Name() string

	// CanRepair checks if this module can handle the given diagnostic.
	// It examines the diagnostic's structure type and repair action to
	// determine if this module is responsible for the repair.
	CanRepair(d Diagnostic) bool

	// Validate checks if the repair is safe to apply.
	// It performs pre-repair validation to ensure:
	//   - The offset is valid and within bounds
	//   - The structure at the offset matches expectations
	//   - The repair won't corrupt adjacent structures
	// Returns an error if the repair is unsafe.
	Validate(data []byte, d Diagnostic) error

	// Apply performs the actual repair on the data.
	// The data slice is modified in-place at the diagnostic's offset.
	// This method assumes Validate() has already passed.
	// Returns an error if the repair fails.
	Apply(data []byte, d Diagnostic) error

	// Verify confirms the repair was successful.
	// It performs post-repair validation to ensure:
	//   - The bytes were written correctly
	//   - The structure is still parseable
	//   - No side effects on adjacent data
	// Returns an error if verification fails.
	Verify(data []byte, d Diagnostic) error
}

// RepairModuleBase provides common functionality for repair modules.
// Concrete modules can embed this to reduce boilerplate.
type RepairModuleBase struct {
	name string
}

// Name returns the module identifier.
func (m *RepairModuleBase) Name() string {
	return m.name
}

// ValidateOffset checks if an offset is valid within the data bounds.
// This is a common validation that most modules need.
func (m *RepairModuleBase) ValidateOffset(data []byte, offset uint64, size uint64) error {
	if offset+size > uint64(len(data)) {
		return &RepairError{
			Module:  m.name,
			Offset:  offset,
			Message: "offset out of bounds",
		}
	}
	return nil
}

// ValidateNoOverlap checks if a repair region doesn't overlap with critical structures.
// This prevents repairs from accidentally corrupting other parts of the types.
func (m *RepairModuleBase) ValidateNoOverlap(offset, size uint64, criticalRegions []Region) error {
	repairRegion := Region{Start: offset, End: offset + size}
	for _, critical := range criticalRegions {
		if repairRegion.Overlaps(critical) {
			return &RepairError{
				Module:  m.name,
				Offset:  offset,
				Message: "repair region overlaps with critical structure",
			}
		}
	}
	return nil
}

// Region represents a byte range in the hive file.
type Region struct {
	Start uint64
	End   uint64
}

// Overlaps checks if two regions overlap.
func (r Region) Overlaps(other Region) bool {
	return r.Start < other.End && other.Start < r.End
}

// Contains checks if this region completely contains another.
func (r Region) Contains(other Region) bool {
	return r.Start <= other.Start && other.End <= r.End
}

// Size returns the size of the region in bytes.
func (r Region) Size() uint64 {
	return r.End - r.Start
}
