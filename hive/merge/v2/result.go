package v2

import "time"

// Options configures the v2 merge pipeline.
type Options struct {
	// ParseOptions controls regtext parsing behavior (for MergeRegText).
	// Currently unused for Merge() which takes pre-built ops.
}

// Result contains statistics from a completed merge.
type Result struct {
	KeysCreated    int
	KeysDeleted    int
	ValuesSet      int
	ValuesDeleted  int
	BytesAllocated int64
	HiveGrowth     int64
	PhaseTiming    PhaseTiming
}

// PhaseTiming records wall-clock duration for each phase of the pipeline.
type PhaseTiming struct {
	Parse time.Duration
	Walk  time.Duration
	Plan  time.Duration
	Write time.Duration
	Flush time.Duration
	Total time.Duration
}
