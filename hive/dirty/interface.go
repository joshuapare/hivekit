package dirty

// DirtyTracker is the minimal interface for tracking dirty (modified) byte ranges.
// Implementations track which regions of a memory-mapped hive file have been modified
// and need to be flushed to disk.
//
// This interface is intended for components that only need to notify about dirty regions
// but don't manage flushing themselves (e.g., allocators, writers, editors).
type DirtyTracker interface {
	// Add marks a byte range as dirty.
	// off is the offset from the start of the file, length is the number of bytes.
	Add(off, length int)
}

// FlushableTracker extends DirtyTracker with methods for flushing dirty regions to disk.
// This interface is intended for components that need to control when and how
// dirty data is persisted (e.g., transaction managers).
type FlushableTracker interface {
	DirtyTracker

	// FlushDataOnly flushes only the data regions (not header/metadata).
	FlushDataOnly() error

	// FlushHeaderAndMeta flushes header and metadata based on the specified mode.
	FlushHeaderAndMeta(mode FlushMode) error
}
