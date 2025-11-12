// Package merge provides high-level API for merging changes into hive files.
//
// Session is the main entry point for merge operations, providing transaction
// safety, dirty page tracking, and configurable write strategies.
package merge

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/merge/strategy"
	"github.com/joshuapare/hivekit/hive/tx"
	"github.com/joshuapare/hivekit/hive/walker"
)

const (
	// defaultIndexCapacity is the default capacity hint for index building.
	// Used for both NK (node key) and VK (value key) capacity.
	defaultIndexCapacity = 10000
)

// Session provides a high-level API for merge operations with tx/dirty integration.
//
// Session wraps the transaction manager, dirty page tracker, and strategy engine
// to provide a clean API for applying merge operations to a hive.
//
// Phase 3: Now uses Strategy implementations for proper dirty tracking.
// The strategy is selected based on Options.Strategy (InPlace, Append, or Hybrid).
type Session struct {
	h        *hive.Hive
	opt      Options
	txMgr    *tx.Manager
	dt       *dirty.Tracker
	idx      index.Index
	alloc    alloc.Allocator
	strategy strategy.Strategy
}

// NewSession creates a merge session for the given hive with the specified options.
//
// This function builds an index from the existing hive structure using a walker.
// For large hives, this may take some time (target: <100ms for 10K keys).
//
// If you already have an index built, use NewSessionWithIndex instead.
func NewSession(h *hive.Hive, opt Options) (*Session, error) {
	// Build index using walker
	builder := walker.NewIndexBuilder(h, defaultIndexCapacity, defaultIndexCapacity)
	idx, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build index: %w", err)
	}

	return NewSessionWithIndex(h, idx, opt)
}

// NewSessionWithIndex creates a session using an existing index.
//
// This is more efficient than NewSession if you already have an index built,
// or if you want to reuse an index across multiple sessions.
//
// The strategy is selected based on opt.Strategy:
//   - StrategyInPlace: Mutates cells in-place when possible
//   - StrategyAppend: Never frees cells, always appends
//   - StrategyHybrid: Heuristic selection between InPlace and Append (default)
func NewSessionWithIndex(h *hive.Hive, idx index.Index, opt Options) (*Session, error) {
	// Create dirty tracker first (needed by allocator)
	dt := dirty.NewTracker(h)

	// Create allocator with dirty tracker (nil = use default config)
	allocator, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		return nil, fmt.Errorf("create allocator: %w", err)
	}

	// Create transaction manager
	txMgr := tx.NewManager(h, dt, opt.Flush)

	// Create strategy based on opt.Strategy
	var strat strategy.Strategy
	switch opt.Strategy {
	case StrategyInPlace:
		strat = strategy.NewInPlace(h, allocator, dt, idx)
	case StrategyAppend:
		strat = strategy.NewAppend(h, allocator, dt, idx)
	case StrategyHybrid:
		strat = strategy.NewHybrid(h, allocator, dt, idx, opt.HybridSlackPct)
	default:
		return nil, fmt.Errorf("unknown strategy: %d", opt.Strategy)
	}

	return &Session{
		h:        h,
		opt:      opt,
		txMgr:    txMgr,
		dt:       dt,
		idx:      idx,
		alloc:    allocator,
		strategy: strat,
	}, nil
}

// Begin starts a transaction.
//
// This increments the REGF PrimarySeq field and updates the timestamp.
// After Begin(), all modifications will be tracked by the dirty page tracker.
//
// You must call either Commit() or Rollback() after Begin().
func (s *Session) Begin() error {
	return s.txMgr.Begin()
}

// Commit flushes changes and updates REGF sequences.
//
// This performs the ordered flush protocol:
// 1. Flush all dirty data ranges (not header)
// 2. Update SecondarySeq = PrimarySeq
// 3. Flush header page + fdatasync (based on FlushMode)
//
// After Commit(), the transaction is complete and changes are durable.
func (s *Session) Commit() error {
	return s.txMgr.Commit()
}

// Rollback aborts the transaction.
//
// This is a best-effort operation that clears the transaction state.
// Note: Since we operate on mmap, data changes cannot be rolled back.
// Rollback primarily ensures the header sequences remain consistent.
func (s *Session) Rollback() {
	s.txMgr.Rollback()
}

// Apply executes a merge plan within the current transaction.
//
// You must call Begin() before Apply() and Commit() after Apply().
// If any operation fails, the error is returned and you should Rollback().
//
// Phase 3: This delegates to the selected Strategy (InPlace, Append, or Hybrid).
// Dirty tracking is handled automatically by the strategy implementations.
//
// Returns Applied statistics (keys created, values set, etc.).
func (s *Session) Apply(plan *Plan) (Applied, error) {
	var result Applied

	// Apply each operation in the plan
	for i, op := range plan.Ops {
		if err := s.applyOp(&op, &result); err != nil {
			return result, fmt.Errorf("operation %d (%s): %w", i, op.Type, err)
		}
	}

	return result, nil
}

// applyOp applies a single operation using the strategy.
func (s *Session) applyOp(op *Op, result *Applied) error {
	switch op.Type {
	case OpEnsureKey:
		_, keysCreated, err := s.strategy.EnsureKey(op.KeyPath)
		if err != nil {
			return err
		}
		result.KeysCreated += keysCreated
		return nil

	case OpSetValue:
		err := s.strategy.SetValue(op.KeyPath, op.ValueName, op.ValueType, op.Data)
		if err != nil {
			return err
		}
		result.ValuesSet++
		return nil

	case OpDeleteValue:
		err := s.strategy.DeleteValue(op.KeyPath, op.ValueName)
		if err != nil {
			return err
		}
		result.ValuesDeleted++
		return nil

	case OpDeleteKey:
		// Check if key exists before deletion (read-only index lookup)
		// This works correctly even when keys are created+deleted in same transaction
		// because the index is updated as operations are applied
		_, keyExists := index.WalkPath(s.idx, s.h.RootCellOffset(), op.KeyPath...)

		// Delete the key (idempotent - no-op if doesn't exist)
		err := s.strategy.DeleteKey(op.KeyPath, true) // recursive=true
		if err != nil {
			return err
		}

		// Only count as deleted if the key existed before deletion
		if keyExists {
			result.KeysDeleted++
		}
		return nil

	default:
		return fmt.Errorf("unknown operation type: %d", op.Type)
	}
}

// ApplyWithTx is a convenience method that wraps Begin -> Apply -> Commit.
//
// This is the recommended way to apply a plan if you don't need manual
// transaction control. If Apply fails, Rollback is called automatically.
//
// Example:
//
//	plan := merge.NewPlan()
//	plan.AddSetValue([]string{"Software", "Test"}, "Version", 1, []byte("1.0\x00"))
//	applied, err := session.ApplyWithTx(plan)
func (s *Session) ApplyWithTx(plan *Plan) (Applied, error) {
	// Begin transaction
	if err := s.Begin(); err != nil {
		return Applied{}, fmt.Errorf("begin: %w", err)
	}

	// Apply plan
	result, err := s.Apply(plan)
	if err != nil {
		s.Rollback()
		return result, fmt.Errorf("apply: %w", err)
	}

	// Commit transaction
	if commitErr := s.Commit(); commitErr != nil {
		return result, fmt.Errorf("commit: %w", commitErr)
	}

	return result, nil
}

// Index returns the current index for inspection.
//
// You can use this to query keys/values that exist in the hive.
// The index is kept up-to-date as operations are applied.
func (s *Session) Index() index.Index {
	return s.idx
}

// EnableDeferredMode enables deferred subkey list building for bulk operations.
// This dramatically improves performance by eliminating expensive read-modify-write cycles.
// Supported by InPlace and Append strategies (any strategy that embeds strategy.Base).
// Must be followed by FlushDeferredSubkeys before committing.
func (s *Session) EnableDeferredMode() {
	// Type-assert to access Base methods
	// Both InPlace and Append embed Base, so this works for both
	if ip, ok := s.strategy.(*strategy.InPlace); ok {
		ip.EnableDeferredMode()
	} else if ap, ok := s.strategy.(*strategy.Append); ok {
		ap.EnableDeferredMode()
	}
	// If strategy doesn't support deferred mode, this is a no-op
}

// DisableDeferredMode disables deferred subkey list building.
// Returns an error if there are pending deferred updates.
func (s *Session) DisableDeferredMode() error {
	if ip, ok := s.strategy.(*strategy.InPlace); ok {
		return ip.DisableDeferredMode()
	} else if ap, ok := s.strategy.(*strategy.Append); ok {
		return ap.DisableDeferredMode()
	}
	return nil // Strategy doesn't support deferred mode
}

// FlushDeferredSubkeys writes all accumulated deferred children to disk.
// Returns the number of parents flushed and any error encountered.
func (s *Session) FlushDeferredSubkeys() (int, error) {
	if ip, ok := s.strategy.(*strategy.InPlace); ok {
		return ip.FlushDeferredSubkeys()
	} else if ap, ok := s.strategy.(*strategy.Append); ok {
		return ap.FlushDeferredSubkeys()
	}
	return 0, nil // Strategy doesn't support deferred mode
}

// Close cleans up resources used by the session.
//
// CRITICAL: Flushes all dirty pages to disk before cleanup.
// This ensures all modifications made during the session are persisted.
// The underlying hive is NOT closed - you must close it separately.
func (s *Session) Close() error {
	// CRITICAL: Flush dirty pages before resetting tracker
	// Without this, all tracked dirty pages are discarded and changes are lost
	if err := s.dt.FlushDataOnly(); err != nil {
		return fmt.Errorf("failed to flush data pages: %w", err)
	}
	if err := s.dt.FlushHeaderAndMeta(dirty.FlushAuto); err != nil {
		return fmt.Errorf("failed to flush header: %w", err)
	}

	// Reset dirty tracker
	s.dt.Reset()
	return nil
}
