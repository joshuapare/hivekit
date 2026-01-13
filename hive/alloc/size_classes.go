package alloc

import "math"

// SizeClassConfig defines the allocation size class strategy.
// Different configurations can be tested to find optimal performance/fragmentation tradeoff.
type SizeClassConfig struct {
	// Name for this configuration (for benchmarking)
	Name string

	// Small allocation settings (linear increments)
	SmallMin       int32 // Minimum allocation size (typically 8)
	SmallMax       int32 // Max for linear increments (typically 256-512)
	SmallIncrement int32 // Increment size for small allocations (8, 16, or 32)

	// Medium/Large allocation settings (logarithmic growth)
	MediumMax    int32   // Max before large list (typically 16KB)
	GrowthFactor float64 // Exponential growth factor (1.5, 2.0, etc.)
}

// Predefined configurations for testing.
var (
	// FineGrained: Many small buckets, good for varied workloads
	// 8-256 step 8 (31 classes) + 256-16K log growth (~15 classes) = ~46 total.
	ConfigFineGrained = SizeClassConfig{
		Name:           "FineGrained",
		SmallMin:       8,
		SmallMax:       256,
		SmallIncrement: 8,
		MediumMax:      16384,
		GrowthFactor:   1.5,
	}

	// Balanced: Good balance between heap size and granularity
	// 8-512 step 16 (32 classes) + 512-16K log growth (~8 classes) = ~40 total.
	ConfigBalanced = SizeClassConfig{
		Name:           "Balanced",
		SmallMin:       8,
		SmallMax:       512,
		SmallIncrement: 16,
		MediumMax:      16384,
		GrowthFactor:   1.5,
	}

	// Coarse: Fewer buckets, faster operations but more internal fragmentation
	// 8-512 step 32 (16 classes) + 512-16K log growth (~6 classes) = ~22 total.
	ConfigCoarse = SizeClassConfig{
		Name:           "Coarse",
		SmallMin:       8,
		SmallMax:       512,
		SmallIncrement: 32,
		MediumMax:      16384,
		GrowthFactor:   2.0,
	}

	// Registry: Optimized for Windows Registry workloads
	// Based on empirical analysis of real registry hives
	// 8-128 step 8 (15 classes) + 128-4096 step 64 (62 classes) + 4K-16K log (~4 classes) = ~81 total.
	ConfigRegistry = SizeClassConfig{
		Name:           "Registry",
		SmallMin:       8,
		SmallMax:       128,
		SmallIncrement: 8,
		MediumMax:      16384,
		GrowthFactor:   1.3, // Tighter packing for registry data
	}

	// Default configuration (used if none specified).
	DefaultConfig = ConfigBalanced
)

// sizeClassTable holds the computed size class boundaries.
type sizeClassTable struct {
	config     SizeClassConfig
	boundaries []int32 // Upper bound for each size class
	numClasses int
}

// newSizeClassTable computes size class boundaries from config.
func newSizeClassTable(config SizeClassConfig) *sizeClassTable {
	table := &sizeClassTable{
		config:     config,
		boundaries: make([]int32, 0, 64), // Preallocate reasonable size
	}

	// Phase 1: Small allocations (linear increments)
	for size := config.SmallMin; size < config.SmallMax; size += config.SmallIncrement {
		table.boundaries = append(table.boundaries, size+config.SmallIncrement-1)
	}

	// Phase 2: Medium/Large allocations (logarithmic growth)
	if config.SmallMax < config.MediumMax {
		size := config.SmallMax
		for size < config.MediumMax {
			nextSize := int32(math.Ceil(float64(size) * config.GrowthFactor))
			if nextSize <= size {
				nextSize = size + 1 // Ensure progress
			}
			table.boundaries = append(table.boundaries, nextSize-1)
			size = nextSize
		}
	}

	table.numClasses = len(table.boundaries)
	return table
}

// getSizeClass returns the size class index for a given allocation size.
// Returns table.numClasses for sizes >= MediumMax (use large list).
func (t *sizeClassTable) getSizeClass(size int32) int {
	// Binary search for the appropriate size class
	lo, hi := 0, t.numClasses-1

	for lo <= hi {
		mid := (lo + hi) / 2
		if size <= t.boundaries[mid] {
			// Check if this is the smallest boundary that fits
			if mid == 0 || size > t.boundaries[mid-1] {
				return mid
			}
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}

	// Size is larger than all boundaries → large list
	return t.numClasses
}

// String returns a human-readable description of the size class table.
func (t *sizeClassTable) String() string {
	return t.config.Name
}

// NumClasses returns the number of size classes (excluding large list).
func (t *sizeClassTable) NumClasses() int {
	return t.numClasses
}
