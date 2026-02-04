package strategy

import (
	"context"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
)

const (
	// percentageDivisor is the divisor for percentage calculations (e.g., slack% / 100).
	percentageDivisor = 100

	// smallValueThreshold is the size threshold for small values (in bytes).
	// Values smaller than this use InPlace strategy, larger values check slack.
	smallValueThreshold = 1024
)

// Hybrid uses heuristics to choose between InPlace and Append strategies.
//
// Decision rules:
//   - Keys: Always use InPlace (keys are small, rarely updated)
//   - Small values (<1KB): Use InPlace
//   - Large values: Check if fits with slack percentage
//   - Deletes: Always use InPlace (frees cells for reuse)
//
// The slack percentage (HybridSlackPct) allows some growth before switching
// to append mode. For example, with 12% slack:
//   - 100-byte value → 110 bytes: InPlace (fits in 112 bytes)
//   - 100-byte value → 115 bytes: Append (exceeds 112 bytes)
//
// Use cases:
//   - General-purpose merges (default strategy)
//   - Balances space efficiency and performance
//   - Reduces fragmentation while allowing controlled growth
//
// Trade-offs:
//   - Balanced: Best of both worlds
//   - Adaptive: Chooses strategy per operation
//   - Complexity: More logic than single-strategy approaches
type Hybrid struct {
	inplace  *InPlace
	append   *Append
	slackPct int
}

// NewHybrid creates a hybrid strategy that selects between InPlace and Append.
//
// The strategy maintains both InPlace and Append delegates and chooses
// based on operation size and slack threshold.
//
// Parameters:
//   - slackPct: Allowed slack percentage for in-place updates (e.g., 12 for 12%)
func NewHybrid(
	h *hive.Hive,
	a alloc.Allocator,
	dt *dirty.Tracker,
	idx index.Index,
	slackPct int,
) Strategy {
	inplace, ok := NewInPlace(h, a, dt, idx).(*InPlace)
	if !ok {
		panic("NewInPlace returned unexpected type")
	}
	appendStrat, ok := NewAppend(h, a, dt, idx).(*Append)
	if !ok {
		panic("NewAppend returned unexpected type")
	}
	return &Hybrid{
		inplace:  inplace,
		append:   appendStrat,
		slackPct: slackPct,
	}
}

// shouldUseInPlace decides whether to use in-place updates based on slack.
//
// Returns true if the needed size fits within the available size + slack percentage.
// For example, with 12% slack and available=100:
//   - needed=110: true (110 <= 112)
//   - needed=115: false (115 > 112)
func (hy *Hybrid) shouldUseInPlace(needed, available int) bool {
	slackBytes := (available * hy.slackPct) / percentageDivisor
	return needed <= (available + slackBytes)
}

// EnsureKey implements Strategy.
//
// Always delegates to InPlace because:
//   - Keys are small (typically <256 bytes)
//   - Keys are rarely updated after creation
//   - In-place allocation is efficient for keys
func (hy *Hybrid) EnsureKey(ctx context.Context, path []string) (uint32, int, error) {
	return hy.inplace.EnsureKey(ctx, path)
}

// SetValue implements Strategy.
//
// Decision logic:
//   - Values <1KB: Always use InPlace (small, efficient)
//   - Values ≥1KB: Use Append (large values grow over time)
//
// Future enhancement: Check existing VK cell size and use shouldUseInPlace()
// to make smarter decisions for updates vs. creates.
func (hy *Hybrid) SetValue(ctx context.Context, path []string, name string, typ uint32, data []byte) error {
	// Simple heuristic: small values use InPlace, large values use Append
	// TODO: For updates, check if fits in existing cell with slack
	if len(data) < smallValueThreshold {
		return hy.inplace.SetValue(ctx, path, name, typ, data)
	}
	return hy.append.SetValue(ctx, path, name, typ, data)
}

// DeleteValue implements Strategy.
//
// Always delegates to InPlace because:
//   - Deletes should free cells for reuse
//   - InPlace strategy properly frees VK cells
//   - No benefit to append-only for deletes
func (hy *Hybrid) DeleteValue(ctx context.Context, path []string, name string) error {
	return hy.inplace.DeleteValue(ctx, path, name)
}

// DeleteKey implements Strategy.
//
// Always delegates to InPlace because:
//   - Deletes should free cells for reuse
//   - InPlace strategy properly frees NK cells
//   - No benefit to append-only for deletes
func (hy *Hybrid) DeleteKey(ctx context.Context, path []string, recursive bool) error {
	return hy.inplace.DeleteKey(ctx, path, recursive)
}

// EnableDeferredMode enables deferred subkey list building on the InPlace delegate.
func (hy *Hybrid) EnableDeferredMode() {
	hy.inplace.EnableDeferredMode()
}

// DisableDeferredMode disables deferred subkey list building on the InPlace delegate.
func (hy *Hybrid) DisableDeferredMode() error {
	return hy.inplace.DisableDeferredMode()
}

// FlushDeferredSubkeys flushes accumulated deferred children from the InPlace delegate.
func (hy *Hybrid) FlushDeferredSubkeys() (int, error) {
	return hy.inplace.FlushDeferredSubkeys()
}
