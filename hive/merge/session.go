// Package merge provides high-level API for merging changes into hive files.
//
// Session is the main entry point for merge operations, providing transaction
// safety, dirty page tracking, and configurable write strategies.
package merge

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
// The context can be used to cancel index building for large hives.
//
// If you already have an index built, use NewSessionWithIndex instead.
func NewSession(ctx context.Context, h *hive.Hive, opt Options) (*Session, error) {
	// Build index using walker with configurable index kind
	builder := walker.NewIndexBuilderWithKind(h, defaultIndexCapacity, defaultIndexCapacity, opt.IndexKind)
	idx, err := builder.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("build index: %w", err)
	}

	return NewSessionWithIndex(ctx, h, idx, opt)
}

// NewSessionWithIndex creates a session using an existing index.
//
// This is more efficient than NewSession if you already have an index built,
// or if you want to reuse an index across multiple sessions.
//
// The context is passed through for consistency but is not used directly
// since index building is already complete.
//
// The strategy is selected based on opt.Strategy:
//   - StrategyInPlace: Mutates cells in-place when possible
//   - StrategyAppend: Never frees cells, always appends
//   - StrategyHybrid: Heuristic selection between InPlace and Append (default)
func NewSessionWithIndex(ctx context.Context, h *hive.Hive, idx index.Index, opt Options) (*Session, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
// The context can be used to cancel the operation.
func (s *Session) Begin(ctx context.Context) error {
	return s.txMgr.Begin(ctx)
}

// Commit flushes changes and updates REGF sequences.
//
// This performs the ordered flush protocol:
// 1. Flush all dirty data ranges (not header)
// 2. Update SecondarySeq = PrimarySeq
// 3. Flush header page + fdatasync (based on FlushMode)
//
// After Commit(), the transaction is complete and changes are durable.
// The context can be used to cancel the operation.
func (s *Session) Commit(ctx context.Context) error {
	return s.txMgr.Commit(ctx)
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
// The context can be used to cancel the operation. If cancelled, partial
// operations may have been applied.
//
// Returns Applied statistics (keys created, values set, etc.).
func (s *Session) Apply(ctx context.Context, plan *Plan) (Applied, error) {
	var result Applied

	// Apply each operation in the plan
	for i, op := range plan.Ops {
		// Check for cancellation before each operation
		if err := ctx.Err(); err != nil {
			return result, err
		}

		if err := s.applyOp(ctx, &op, &result); err != nil {
			return result, fmt.Errorf("operation %d (%s): %w", i, op.Type, err)
		}
	}

	return result, nil
}

// applyOp applies a single operation using the strategy.
func (s *Session) applyOp(ctx context.Context, op *Op, result *Applied) error {
	switch op.Type {
	case OpEnsureKey:
		_, keysCreated, err := s.strategy.EnsureKey(ctx, op.KeyPath)
		if err != nil {
			return err
		}
		result.KeysCreated += keysCreated
		return nil

	case OpSetValue:
		err := s.strategy.SetValue(ctx, op.KeyPath, op.ValueName, op.ValueType, op.Data)
		if err != nil {
			return err
		}
		result.ValuesSet++
		return nil

	case OpDeleteValue:
		err := s.strategy.DeleteValue(ctx, op.KeyPath, op.ValueName)
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
		err := s.strategy.DeleteKey(ctx, op.KeyPath, true) // recursive=true
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
// The context can be used to cancel the entire operation (begin, apply, commit).
//
// Example:
//
//	plan := merge.NewPlan()
//	plan.AddSetValue([]string{"Software", "Test"}, "Version", 1, []byte("1.0\x00"))
//	applied, err := session.ApplyWithTx(ctx, plan)
func (s *Session) ApplyWithTx(ctx context.Context, plan *Plan) (Applied, error) {
	// Begin transaction
	if err := s.Begin(ctx); err != nil {
		return Applied{}, fmt.Errorf("begin: %w", err)
	}

	// Apply plan
	result, err := s.Apply(ctx, plan)
	if err != nil {
		s.Rollback()
		return result, fmt.Errorf("apply: %w", err)
	}

	// Commit transaction
	if commitErr := s.Commit(ctx); commitErr != nil {
		return result, fmt.Errorf("commit: %w", commitErr)
	}

	return result, nil
}

// ApplyRegTextWithPrefix parses regtext, transforms paths with prefix, and applies.
//
// This is the session-based equivalent of MergeRegTextWithPrefix. Use it when you
// need control over the session lifecycle (e.g., to check storage stats after apply).
//
// The prefix is prepended to all key paths in the regtext. Hive root prefixes
// (HKEY_LOCAL_MACHINE\, HKLM\, etc.) are automatically stripped.
//
// Parse options are taken from the session's Options.ParseOptions. To parse headerless
// regtext, create the session with Options{ParseOptions: types.RegParseOptions{AllowMissingHeader: true}}.
//
// Example:
//
//	h, _ := hive.Open(path)
//	defer h.Close()
//	sess, _ := merge.NewSession(ctx, h, opts)
//	defer sess.Close(ctx)
//
//	applied, _ := sess.ApplyRegTextWithPrefix(ctx, regText, "SOFTWARE")
//	stats := sess.GetStorageStats()  // check bloat
//	result, _ := sess.HasKeys(ctx, "SOFTWARE\\Microsoft")  // validation
func (s *Session) ApplyRegTextWithPrefix(ctx context.Context, regText string, prefix string) (Applied, error) {
	plan, err := PlanFromRegTextWithPrefixOpts(regText, prefix, s.opt.ParseOptions)
	if err != nil {
		return Applied{}, err
	}
	return s.ApplyWithTx(ctx, plan)
}

// ApplyRegText parses regtext and applies in one transaction (no prefix transformation).
//
// Equivalent to ApplyRegTextWithPrefix(ctx, regText, "").
// Use when the regtext paths are already correct relative to the hive root.
//
// Parse options are taken from the session's Options.ParseOptions.
func (s *Session) ApplyRegText(ctx context.Context, regText string) (Applied, error) {
	plan, err := PlanFromRegTextOpts(regText, s.opt.ParseOptions)
	if err != nil {
		return Applied{}, err
	}
	return s.ApplyWithTx(ctx, plan)
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

// GetEfficiencyStats returns efficiency statistics from the allocator.
//
// This provides information about space usage, fragmentation, and HBIN efficiency.
// The statistics include total capacity, allocated bytes, and wasted space.
func (s *Session) GetEfficiencyStats() alloc.EfficiencyStats {
	// Type assert to FastAllocator to access GetEfficiencyStats
	// The default allocator is always FastAllocator
	if fa, ok := s.alloc.(*alloc.FastAllocator); ok {
		return fa.GetEfficiencyStats()
	}
	// Return empty stats if not a FastAllocator (shouldn't happen)
	return alloc.EfficiencyStats{}
}

// GetStorageStats returns storage statistics for the hive.
//
// This provides file size, free space, and usage metrics without reopening the hive.
// The statistics are derived from the allocator's basic stats, which avoids the
// expensive O(n²) sorting done by GetEfficiencyStats().
func (s *Session) GetStorageStats() StorageStats {
	fileSize := s.h.Size()

	// Use GetBasicStats for lightweight computation (avoids sorting)
	var capacity, allocated int64
	if fa, ok := s.alloc.(*alloc.FastAllocator); ok {
		capacity, allocated = fa.GetBasicStats()
	} else {
		// Fallback to full stats if not FastAllocator (shouldn't happen)
		effStats := s.GetEfficiencyStats()
		capacity = effStats.TotalCapacity
		allocated = effStats.TotalAllocated
	}

	wasted := capacity - allocated
	var freePercent float64
	if fileSize > 0 {
		freePercent = float64(wasted) / float64(fileSize) * 100.0
	}

	return StorageStats{
		FileSize:    fileSize,
		FreeBytes:   wasted,
		UsedBytes:   allocated,
		FreePercent: freePercent,
	}
}

// GetHiveStats returns comprehensive hive statistics including storage,
// efficiency, and structure metrics.
//
// This is the recommended method for getting complete hive information
// after applying merge operations, as it avoids reopening the hive file.
func (s *Session) GetHiveStats() HiveStats {
	return HiveStats{
		Storage:    s.GetStorageStats(),
		Efficiency: s.GetEfficiencyStats(),
		// Note: TotalKeys and TotalValues are not populated here
		// since that requires a full tree walk. Use AnalyzeHive() for those.
	}
}

// KeyInfo represents a key in the hive with its path and metadata.
type KeyInfo struct {
	// Path is the full path from the root key to this key.
	// Each element is a key name segment.
	Path []string

	// SubkeyCount is the number of immediate subkeys under this key.
	SubkeyCount uint32

	// ValueCount is the number of values stored in this key.
	ValueCount uint32

	// Offset is the cell offset of this key in the hive file.
	// This is useful for advanced operations that need to reference the key directly.
	Offset uint32
}

// WalkKeys walks all keys in the hive recursively, calling fn for each key.
//
// The walk is performed in depth-first order starting from the root key.
// If fn returns walker.ErrStopWalk, the walk stops early and nil is returned.
// If the context is cancelled, the context error is returned.
// Any other error from fn is returned to the caller.
//
// Example:
//
//	err := session.WalkKeys(ctx, func(info merge.KeyInfo) error {
//	    fmt.Printf("Key: %s (subkeys: %d, values: %d)\n",
//	        strings.Join(info.Path, "\\"), info.SubkeyCount, info.ValueCount)
//	    return nil
//	})
func (s *Session) WalkKeys(ctx context.Context, fn func(KeyInfo) error) error {
	// Start from the root key
	rootOffset := s.h.RootCellOffset()
	err := s.walkKeysRecursive(ctx, rootOffset, nil, fn)
	// Convert ErrStopWalk to nil for the caller (normal early termination)
	return wrapWalkError(err)
}

// walkKeysRecursive performs depth-first traversal of the key tree.
func (s *Session) walkKeysRecursive(ctx context.Context, nkOffset uint32, parentPath []string, fn func(KeyInfo) error) error {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Resolve the NK cell
	payload, err := s.h.ResolveCellPayload(nkOffset)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	// Build the current path
	name := string(nk.Name())
	var currentPath []string
	if parentPath == nil {
		// Root key - use empty path or the root name
		currentPath = []string{name}
	} else {
		currentPath = make([]string, len(parentPath)+1)
		copy(currentPath, parentPath)
		currentPath[len(parentPath)] = name
	}

	// Create KeyInfo and call the callback
	info := KeyInfo{
		Path:        currentPath,
		SubkeyCount: nk.SubkeyCount(),
		ValueCount:  nk.ValueCount(),
		Offset:      nkOffset,
	}

	if callbackErr := fn(info); callbackErr != nil {
		// Propagate ErrStopWalk to stop the entire walk
		return callbackErr
	}

	// Recursively walk subkeys (only if this key has subkeys)
	if nk.SubkeyCount() > 0 {
		err = walker.WalkSubkeysCtx(ctx, s.h, nkOffset, func(subkeyNK hive.NK, ref uint32) error {
			return s.walkKeysRecursive(ctx, ref, currentPath, fn)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// wrapWalkError checks if an error is ErrStopWalk and converts it to nil for the caller.
func wrapWalkError(err error) error {
	if errors.Is(err, walker.ErrStopWalk) {
		return nil
	}
	return err
}

// ListAllKeys returns all key paths in the hive as backslash-separated strings.
//
// This is a convenience wrapper around WalkKeys that collects all paths.
// For large hives with many keys, consider using WalkKeys directly to avoid
// storing all paths in memory.
//
// Example:
//
//	keys, err := session.ListAllKeys(ctx)
//	if err != nil {
//	    return err
//	}
//	for _, key := range keys {
//	    fmt.Println(key)
//	}
func (s *Session) ListAllKeys(ctx context.Context) ([]string, error) {
	var paths []string
	err := s.WalkKeys(ctx, func(info KeyInfo) error {
		paths = append(paths, strings.Join(info.Path, "\\"))
		return nil
	})
	return paths, err
}

// KeyCheckResult contains results from checking key existence.
type KeyCheckResult struct {
	// AllPresent is true if all requested keys exist in the hive.
	AllPresent bool

	// Present contains the key paths that exist in the hive.
	Present []string

	// Missing contains the key paths that do not exist in the hive.
	Missing []string
}

// HasKeys checks if the specified key paths exist in the hive.
//
// Key paths should be provided as backslash-separated strings
// (e.g., "Software\\Microsoft\\Windows").
//
// This method uses the index for O(1) lookups per key, making it very efficient
// for checking many keys.
//
// Returns detailed information about which keys are present vs missing.
// The context can be used to cancel the operation.
//
// Example:
//
//	result, err := session.HasKeys(ctx,
//	    "Software\\Microsoft",
//	    "Software\\NonExistent",
//	    "System\\ControlSet001",
//	)
//	if err != nil {
//	    return err
//	}
//	if !result.AllPresent {
//	    fmt.Printf("Missing keys: %v\n", result.Missing)
//	}
func (s *Session) HasKeys(ctx context.Context, keyPaths ...string) (KeyCheckResult, error) {
	// Check for cancellation before starting
	if err := ctx.Err(); err != nil {
		return KeyCheckResult{}, err
	}

	result := KeyCheckResult{
		AllPresent: true,
		Present:    make([]string, 0, len(keyPaths)),
		Missing:    make([]string, 0),
	}

	rootOffset := s.h.RootCellOffset()

	for _, keyPath := range keyPaths {
		// Check for cancellation before each key
		if err := ctx.Err(); err != nil {
			return result, err
		}

		// Split path and check via index
		parts := strings.Split(keyPath, "\\")
		_, exists := index.WalkPath(s.idx, rootOffset, parts...)

		if exists {
			result.Present = append(result.Present, keyPath)
		} else {
			result.Missing = append(result.Missing, keyPath)
			result.AllPresent = false
		}
	}

	return result, nil
}

// HasKey is a convenience method to check if a single key exists.
//
// The key path should be a backslash-separated string
// (e.g., "Software\\Microsoft\\Windows").
//
// This method uses the index for O(1) lookup.
func (s *Session) HasKey(keyPath string) bool {
	parts := strings.Split(keyPath, "\\")
	_, exists := index.WalkPath(s.idx, s.h.RootCellOffset(), parts...)
	return exists
}

// Close cleans up resources used by the session.
//
// CRITICAL: Flushes all dirty pages to disk before cleanup.
// This ensures all modifications made during the session are persisted.
// The underlying hive is NOT closed - you must close it separately.
//
// The context can be used to cancel the flush operations.
func (s *Session) Close(ctx context.Context) error {
	// CRITICAL: Flush dirty pages before resetting tracker
	// Without this, all tracked dirty pages are discarded and changes are lost
	if err := s.dt.FlushDataOnly(ctx); err != nil {
		return fmt.Errorf("failed to flush data pages: %w", err)
	}
	if err := s.dt.FlushHeaderAndMeta(ctx, dirty.FlushAuto); err != nil {
		return fmt.Errorf("failed to flush header: %w", err)
	}

	// Reset dirty tracker
	s.dt.Reset()
	return nil
}
