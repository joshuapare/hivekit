// Package merge provides high-level APIs for applying changes to Windows Registry hive files.
//
// # Overview
//
// This package implements transactional merging of registry changes into hive files with:
//   - Multiple input formats (.reg files, JSON patches, programmatic Plans)
//   - Three write strategies (InPlace, Append, Hybrid)
//   - Transaction safety with automatic rollback
//   - Query optimization for multi-file merges
//   - Dirty page tracking for efficient commits
//
// # Quick Start
//
// Simplest use case - apply a .reg file to a hive:
//
//	regText := `Windows Registry Editor Version 5.00
//	[HKEY_LOCAL_MACHINE\Software\Test]
//	"Version"="1.0"
//	`
//	applied, err := merge.MergeRegText("/path/to/system.hive", regText, nil)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Created %d keys, set %d values\n", applied.KeysCreated, applied.ValuesSet)
//
// # Core Concepts
//
// Plan: An ordered collection of operations to apply to a hive
//   - OpEnsureKey: Create key if it doesn't exist
//   - OpDeleteKey: Remove key and optionally its subkeys
//   - OpSetValue: Create or update a registry value
//   - OpDeleteValue: Remove a registry value
//
// Session: Transaction-safe merge session that manages:
//   - Index building (for fast key/value lookups)
//   - Allocator (for cell allocation/deallocation)
//   - Dirty tracker (for page-level change tracking)
//   - Transaction manager (for ACID semantics)
//
// Strategy: Write approach (configured via Options.Strategy)
//   - InPlace: Mutate cells in-place when possible (best for small changes)
//   - Append: Always allocate new cells, never free (safe for logs)
//   - Hybrid: Heuristic-based selection (default, best for most cases)
//
// # Creating Plans
//
// Programmatic plan creation:
//
//	plan := merge.NewPlan()
//	plan.AddEnsureKey([]string{"Software", "Test"})
//	plan.AddSetValue([]string{"Software", "Test"}, "Version", format.REGSZ, []byte("1.0\x00"))
//	plan.AddDeleteValue([]string{"Software", "Test"}, "OldSetting")
//	plan.AddDeleteKey([]string{"Software", "Deprecated"})
//
// From .reg file text:
//
//	plan, err := merge.PlanFromRegText(regFileContent)
//	if err != nil {
//	    return err
//	}
//
// From multiple .reg files with optimization:
//
//	regTexts := []string{baseConfig, patch1, patch2}
//	plan, stats, err := merge.PlanFromRegTexts(regTexts)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Optimized: %d → %d ops (%.1f%% reduction)\n",
//	    stats.InputOps, stats.OutputOps, stats.ReductionPercent())
//
// From JSON patch:
//
//	jsonPatch := `{
//	    "operations": [
//	        {"op": "ensure_key", "key_path": ["Software", "Test"]},
//	        {"op": "set_value", "key_path": ["Software", "Test"],
//	         "value_name": "Version", "value_type": "REG_SZ",
//	         "data": [49, 46, 48, 0]}
//	    ]
//	}`
//	plan, err := merge.ParseJSONPatch([]byte(jsonPatch))
//
// # Applying Plans
//
// Simple one-shot merge:
//
//	applied, err := merge.MergePlan("/path/to/hive", plan, nil)
//
// With custom options:
//
//	opts := merge.DefaultOptions()
//	opts.Strategy = merge.StrategyInPlace
//	opts.GrowChunk = 4 << 20  // 4MB HBIN growth
//	applied, err := merge.MergePlan("/path/to/hive", plan, &opts)
//
// Manual session control (for multiple plans):
//
//	err := merge.WithSession("/path/to/hive", merge.DefaultOptions(),
//	    func(s *merge.Session) error {
//	        // Apply first plan
//	        if _, err := s.ApplyWithTx(plan1); err != nil {
//	            return err
//	        }
//	        // Apply second plan
//	        if _, err := s.ApplyWithTx(plan2); err != nil {
//	            return err
//	        }
//	        return nil
//	    })
//
// Advanced transaction control:
//
//	h, _ := hive.Open("/path/to/hive")
//	defer h.Close()
//
//	session, _ := merge.NewSession(h, merge.DefaultOptions())
//	defer session.Close()
//
//	// Begin transaction
//	if err := session.Begin(); err != nil {
//	    return err
//	}
//
//	// Apply operations
//	applied, err := session.Apply(plan)
//	if err != nil {
//	    session.Rollback()
//	    return err
//	}
//
//	// Commit changes
//	if err := session.Commit(); err != nil {
//	    return err
//	}
//
// # Write Strategies
//
// InPlace Strategy:
//   - Mutates cells in-place when new data fits
//   - Best for: Small updates, minimal fragmentation tolerance
//   - Trade-off: May fragment over time with many size changes
//   - Use case: Configuration updates with stable value sizes
//
// Append Strategy:
//   - Always allocates new cells, never frees old ones
//   - Best for: Append-only logs, crash recovery scenarios
//   - Trade-off: Higher space usage, but no fragmentation
//   - Use case: Event logs, audit trails
//
// Hybrid Strategy (DEFAULT):
//   - Uses heuristics to choose between in-place and append
//   - If new data fits in old cell with slack%, use in-place
//   - Otherwise, allocate new cell
//   - Slack percentage configurable via Options.HybridSlackPct
//   - Best for: General-purpose merges (balanced space/performance)
//   - Use case: Most production workloads
//
// Strategy selection example:
//
//	opts := merge.DefaultOptions()
//	opts.Strategy = merge.StrategyHybrid
//	opts.HybridSlackPct = 12  // Allow 12% slack for in-place updates
//
// # Query Optimization
//
// When merging multiple .reg files, the optimizer provides significant benefits:
//
// Optimizations applied:
//   - Last-write-wins deduplication (if file1 and file2 both set same value, file2 wins)
//   - Delete shadowing (removes ops under deleted subtrees)
//   - I/O-efficient ordering (groups by key, parents before children)
//
// Performance improvements:
//   - Hive opened once (not N times)
//   - Index built once (not N times)
//   - Single transaction (not N transactions)
//   - Redundant operations eliminated before execution
//
// Example:
//
//	// Without optimization (slow):
//	for _, regText := range regTexts {
//	    merge.MergeRegText(hivePath, regText, opts)  // N opens, N index builds
//	}
//
//	// With optimization (fast):
//	plan, stats, _ := merge.PlanFromRegTexts(regTexts)  // Parse + optimize
//	merge.MergePlan(hivePath, plan, opts)               // Single open, single index
//
// # Options and Tuning
//
// Production-ready defaults:
//
//	opts := merge.DefaultOptions()
//	// Hybrid strategy (balanced)
//	// 1MB HBIN growth
//	// Safe flush mode (msync + fdatasync)
//	// 12% in-place slack
//
// Tuning for large batch merges:
//
//	opts := merge.DefaultOptions()
//	opts.GrowChunk = 4 << 20         // 4MB HBIN growth
//	opts.WillNeedHint = true         // Pre-fault pages
//	opts.StripeUnit = 256 << 10      // 256KB EBS alignment
//
// Tuning for small frequent updates:
//
//	opts := merge.DefaultOptions()
//	opts.Strategy = merge.StrategyInPlace  // Minimize space usage
//	opts.GrowChunk = 512 << 10             // 512KB HBIN growth
//
// Linux huge pages (for large hives >1GB):
//
//	opts := merge.DefaultOptions()
//	opts.HugePages = true  // Advise kernel to use 2MB/1GB TLB entries
//
// # Transaction Semantics
//
// ACID properties:
//   - Atomicity: All operations in a plan succeed or all fail
//   - Consistency: Index kept in sync with hive structure
//   - Isolation: No concurrent access during transaction
//   - Durability: Changes flushed to disk on commit (based on FlushMode)
//
// Flush modes (Options.Flush):
//   - FlushAuto: Safe defaults (msync + fdatasync) - RECOMMENDED
//   - FlushDataOnly: Data flush only (caller handles fdatasync)
//   - FlushFull: Ultra-safe (msync + fdatasync + F_FULLFSYNC on macOS)
//
// Transaction lifecycle:
//
//	session.Begin()       // Increment REGF PrimarySeq, update timestamp
//	session.Apply(plan)   // Execute operations, track dirty pages
//	session.Commit()      // Flush dirty pages, set SecondarySeq=PrimarySeq
//
// On failure:
//
//	session.Rollback()    // Clear transaction state (best-effort)
//
// Note: Since operations work on mmap, data changes cannot be rolled back.
// Rollback primarily ensures header sequences remain consistent.
//
// # Applied Statistics
//
// The Applied struct tracks what changed:
//
//	type Applied struct {
//	    KeysCreated   int  // Number of keys created
//	    KeysDeleted   int  // Number of keys deleted
//	    ValuesSet     int  // Number of values set (created or updated)
//	    ValuesDeleted int  // Number of values deleted
//	}
//
// Example:
//
//	applied, err := session.ApplyWithTx(plan)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Applied changes:\n")
//	fmt.Printf("  Keys created: %d\n", applied.KeysCreated)
//	fmt.Printf("  Keys deleted: %d\n", applied.KeysDeleted)
//	fmt.Printf("  Values set: %d\n", applied.ValuesSet)
//	fmt.Printf("  Values deleted: %d\n", applied.ValuesDeleted)
//
// # Error Handling
//
// Operations return errors for:
//   - Invalid hive file path or corrupted hive
//   - Allocation failures (out of space)
//   - Invalid key paths or value data
//   - Transaction errors (begin/commit/rollback failures)
//   - I/O errors during flush
//
// Error handling pattern:
//
//	applied, err := merge.MergePlan(hivePath, plan, opts)
//	if err != nil {
//	    // Specific error checks
//	    if errors.Is(err, hive.ErrCorrupted) {
//	        return fmt.Errorf("hive corrupted: %w", err)
//	    }
//	    return fmt.Errorf("merge failed: %w", err)
//	}
//
// # Thread Safety
//
// Session instances are not thread-safe. Do not share sessions across goroutines.
//
// For concurrent processing:
//   - Use separate Session per goroutine
//   - Process different hive files in parallel
//   - Do NOT access the same hive from multiple goroutines
//
// # Performance Characteristics
//
// Typical performance (18K keys, 45K values per hive):
//   - Index build: ~100ms (one-time cost per session)
//   - Plan application: ~1-5ms per 100 operations
//   - Commit flush: ~10-50ms (depends on dirty page count and FlushMode)
//
// Throughput (with optimization):
//   - Single hive, multiple plans: ~1000 operations/second
//   - Multiple hives, single plan each: ~100-200 hives/second
//   - Multiple .reg files optimized: ~10M hives/hour (scan-merge workload)
//
// Memory overhead per session:
//   - Index: ~4-5MB (for typical hive with 18K keys, 45K values)
//   - Allocator: ~32KB (free-list metadata)
//   - Dirty tracker: ~32KB (for 1GB hive, 1 bit per 4KB page)
//   - Total: ~5MB per session
//
// # Integration with Other Packages
//
// The merge package integrates with:
//   - hive: Opens and manages hive files
//   - hive/index: Fast key/value lookups
//   - hive/alloc: Cell allocation and deallocation
//   - hive/dirty: Page-level modification tracking
//   - hive/tx: Transaction management with REGF sequences
//   - hive/walker: Index building via tree traversal
//   - hive/edit: Low-level editing operations (used by strategies)
//   - hive/merge/strategy: Write strategy implementations
//   - internal/regtext: .reg file parsing
//   - internal/regmerge: Query optimizer for multi-file merges
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive file operations
//   - github.com/joshuapare/hivekit/hive/merge/strategy: Write strategy implementations
//   - github.com/joshuapare/hivekit/hive/tx: Transaction manager
//   - github.com/joshuapare/hivekit/hive/index: Index implementations
//   - github.com/joshuapare/hivekit/internal/regtext: .reg file parser
//   - github.com/joshuapare/hivekit/internal/regmerge: Query optimizer
package merge
