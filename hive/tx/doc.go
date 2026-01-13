// Package tx provides transaction management for Windows Registry hive modifications.
//
// # Overview
//
// This package implements ACID transaction semantics for hive files using the Windows
// Registry REGF sequence number protocol. It coordinates with dirty page tracking to
// ensure durability through ordered flushes.
//
// Transaction lifecycle:
//  1. Begin(): Increment PrimarySeq, update timestamp
//  2. Apply modifications (tracked automatically by DirtyTracker)
//  3. Commit(): Flush data pages, set SecondarySeq=PrimarySeq, flush header
//  4. Rollback(): Abort transaction (best-effort)
//
// # Transaction Manager
//
// The Manager type coordinates all transaction operations:
//
//	type Manager struct {
//	    h    *hive.Hive             // Hive being modified
//	    dt   dirty.FlushableTracker // Dirty page tracker
//	    mode dirty.FlushMode        // Flush mode for commits
//	    seq  uint32                 // Current sequence number
//	    inTx bool                   // Whether a transaction is active
//	}
//
// Creating a manager:
//
//	dt := dirty.NewTracker(hive)
//	mgr := tx.NewManager(hive, dt, dirty.FlushAuto)
//
// # Transaction Protocol
//
// Windows Registry hives use sequence numbers for crash recovery:
//   - PrimarySeq (offset 0x04): Written at transaction Begin()
//   - SecondarySeq (offset 0x08): Written at transaction Commit()
//   - When PrimarySeq == SecondarySeq, the hive is consistent
//   - When PrimarySeq != SecondarySeq, an incomplete transaction exists
//
// Begin() sequence:
//
//	err := mgr.Begin()
//	// 1. Read current PrimarySeq
//	// 2. Increment by 1
//	// 3. Write new PrimarySeq to header
//	// 4. Update timestamp to now (Windows FILETIME)
//	// 5. Mark header page dirty
//	// 6. Set inTx = true
//
// Commit() sequence:
//
//	err := mgr.Commit()
//	// 1. Flush all dirty data pages (msync)
//	// 2. Set SecondarySeq = PrimarySeq
//	// 3. Update timestamp to commit time
//	// 4. Recalculate header checksum
//	// 5. Mark header dirty
//	// 6. Flush header page
//	// 7. Call fdatasync() (based on FlushMode)
//
// Rollback() sequence:
//
//	mgr.Rollback()
//	// 1. Set inTx = false
//	// 2. Leave PrimarySeq != SecondarySeq (incomplete state)
//
// # Basic Usage
//
// Simple transaction:
//
//	// Create manager
//	dt := dirty.NewTracker(hive)
//	mgr := tx.NewManager(hive, dt, dirty.FlushAuto)
//
//	// Begin transaction
//	if err := mgr.Begin(); err != nil {
//	    return err
//	}
//
//	// Apply modifications (e.g., using edit package)
//	editor := edit.NewKeyEditor(hive, allocator, index, dt)
//	nkRef, _, err := editor.EnsureKeyPath(rootRef, []string{"Software", "Test"})
//	if err != nil {
//	    mgr.Rollback()
//	    return err
//	}
//
//	// Commit transaction
//	if err := mgr.Commit(); err != nil {
//	    return err
//	}
//
// # Ordered Flush Protocol
//
// The Commit() method uses a carefully ordered flush sequence to ensure durability:
//
// Step 1: Flush data pages
//   - Write all modified data pages to disk via msync()
//   - Does NOT include header page
//   - Ensures data is durable before marking transaction complete
//
// Step 2: Update header sequences
//   - Set SecondarySeq = PrimarySeq (transaction complete marker)
//   - Update timestamp to commit time
//   - Recalculate header checksum
//
// Step 3: Flush header
//   - Write header page to disk via msync()
//   - Call fdatasync() to ensure metadata persistence (based on FlushMode)
//
// This ordering is critical for crash recovery:
//   - Data written first → safe to set SecondarySeq
//   - Header written last → atomic completion marker
//
// # Crash Recovery
//
// When opening a hive, check sequence numbers for incomplete transactions:
//
//	h, _ := hive.Open(path)
//	bb := h.BaseBlock()
//
//	if bb.Sequence1() != bb.Sequence2() {
//	    // Incomplete transaction detected
//	    fmt.Printf("Warning: Hive has incomplete transaction\n")
//	    fmt.Printf("  PrimarySeq: %d\n", bb.Sequence1())
//	    fmt.Printf("  SecondarySeq: %d\n", bb.Sequence2())
//
//	    // Option 1: Reject the hive
//	    return errors.New("hive corrupted: incomplete transaction")
//
//	    // Option 2: Accept and repair
//	    // (requires write access to set sequences equal)
//	}
//
// Recovery strategies:
//   - Conservative: Reject hive, require manual inspection
//   - Automatic: Accept hive, log warning, continue
//   - Repair: Write SecondarySeq = PrimarySeq (requires transaction)
//
// # Flush Modes
//
// The Manager supports three flush modes (from dirty package):
//
// FlushAuto (RECOMMENDED):
//   - msync() for data pages
//   - msync() + fdatasync() for header
//   - Safe defaults for most use cases
//   - Performance: ~10ms per commit
//
// FlushDataOnly:
//   - msync() only, no fdatasync()
//   - Caller responsible for calling fdatasync()
//   - Use for batch operations with final fdatasync()
//   - Performance: ~5ms per commit
//
// FlushFull:
//   - msync() + fdatasync() + F_FULLFSYNC (macOS)
//   - Ultra-safe, forces disk write-through
//   - Use for critical transactions
//   - Performance: ~50ms per commit
//
// Example with custom flush mode:
//
//	mgr := tx.NewManager(hive, dt, dirty.FlushFull)
//	// All commits will use FlushFull durability
//
// # Transaction State
//
// Query transaction state:
//
//	if mgr.InTransaction() {
//	    fmt.Printf("Transaction active, seq: %d\n", mgr.CurrentSequence())
//	} else {
//	    fmt.Printf("No active transaction\n")
//	}
//
// Begin() is idempotent:
//
//	mgr.Begin() // OK: starts transaction
//	mgr.Begin() // OK: no-op (already in transaction)
//
// Commit() is idempotent:
//
//	mgr.Commit() // OK: commits transaction
//	mgr.Commit() // OK: no-op (no active transaction)
//
// # Header Checksum
//
// The Manager automatically maintains the REGF header checksum:
//
// Checksum calculation:
//   - XOR of first 508 bytes (127 dwords) of header
//   - Excludes checksum field itself (offset 0x1FC)
//   - Special cases: 0x00000000 → 0x00000001, 0xFFFFFFFF → 0xFFFFFFFE
//
// When checksum is updated:
//   - Commit(): Always recalculated after timestamp update
//   - Manual header modifications: Caller must recalculate
//
// Checksum verification:
//
//	bb := hive.BaseBlock()
//	if !bb.ChecksumOK() {
//	    return errors.New("header checksum mismatch")
//	}
//
// # Integration with Dirty Tracker
//
// The Manager coordinates closely with dirty.Tracker:
//
// During Begin():
//   - Manager marks header dirty (0-4096 bytes)
//   - Tracker records this in dirty page bitmap
//
// During modifications:
//   - Edit operations mark cells dirty
//   - Allocator marks new HBINs dirty
//   - All tracked automatically by DirtyTracker
//
// During Commit():
//   - Manager calls dt.FlushDataOnly() for data pages
//   - Manager updates header (SecondarySeq, timestamp, checksum)
//   - Manager marks header dirty again
//   - Manager calls dt.FlushHeaderAndMeta() for header page
//
// Example coordination:
//
//	dt := dirty.NewTracker(hive)
//	mgr := tx.NewManager(hive, dt, dirty.FlushAuto)
//	editor := edit.NewKeyEditor(hive, allocator, index, dt)
//
//	mgr.Begin()              // Marks header dirty
//	editor.EnsureKeyPath(...)  // Marks cells dirty via dt
//	mgr.Commit()             // Flushes all dirty pages
//
// # Performance Characteristics
//
// Begin():
//   - Cost: < 1 μs
//   - Operations: Read PrimarySeq, increment, write, update timestamp
//   - No allocations, no I/O
//
// Commit():
//   - Cost: 5-50ms (depends on dirty page count and FlushMode)
//   - Operations: msync() for data, header update, msync() + fdatasync() for header
//   - Dominates transaction latency
//
// Rollback():
//   - Cost: < 1 μs
//   - Operations: Set inTx = false
//   - No I/O
//
// Typical transaction (100 operations, 100KB dirty):
//   - Begin: < 1 μs
//   - Apply: 500 μs
//   - Commit: 10ms
//   - Total: ~11ms
//
// # Error Handling
//
// Transactions return errors for:
//   - Hive too small (< 4096 bytes)
//   - Flush failures (I/O errors)
//   - msync() failures
//   - fdatasync() failures
//
// Error handling pattern:
//
//	if err := mgr.Begin(); err != nil {
//	    return fmt.Errorf("begin transaction: %w", err)
//	}
//
//	// Apply modifications
//	if err := applyChanges(); err != nil {
//	    mgr.Rollback()
//	    return fmt.Errorf("apply changes: %w", err)
//	}
//
//	if err := mgr.Commit(); err != nil {
//	    return fmt.Errorf("commit transaction: %w", err)
//	}
//
// # Thread Safety
//
// Manager instances are NOT thread-safe. Only one goroutine should use a
// Manager at a time.
//
// For concurrent processing:
//   - Use separate Manager per goroutine
//   - Process different hive files in parallel
//   - Do NOT share Managers across goroutines
//
// # Timestamp Management
//
// The Manager owns ALL REGF protocol fields including timestamps:
//
// When timestamps are updated:
//   - Begin(): Set to current time (transaction start)
//   - Commit(): Set to current time (transaction commit)
//
// Format: Windows FILETIME (100ns intervals since 1601-01-01)
//
// Example timestamp conversion:
//
//	nowFiletime := format.TimeToFiletime(time.Now())
//	format.PutU64(data, format.REGFTimeStampOffset, nowFiletime)
//
// # Design Rationale
//
// Separation of concerns:
//   - tx.Manager: Owns protocol fields (sequences, timestamp, checksum)
//   - dirty.Tracker: Owns dirty page bitmap and flush operations
//   - edit package: Owns cell modifications
//   - alloc package: Owns cell allocation
//
// This prevents conflicts:
//   - Multiple operations in one transaction don't conflict on sequences
//   - Grow() doesn't touch sequences (only data size)
//   - Timestamp updates happen only in Begin()/Commit()
//
// # Integration with Other Packages
//
// The tx package is used by:
//   - hive/merge: Session uses Manager for transaction control
//   - hive/edit: Editors coordinate with dirty tracker
//   - hive/alloc: Allocator marks dirty pages
//   - hive/dirty: Tracker flushes pages on Commit()
//
// Example integration in merge package:
//
//	type Session struct {
//	    txMgr *tx.Manager
//	    dt    *dirty.Tracker
//	    // ...
//	}
//
//	func (s *Session) ApplyWithTx(plan *Plan) error {
//	    s.txMgr.Begin()
//	    result, err := s.Apply(plan)
//	    if err != nil {
//	        s.txMgr.Rollback()
//	        return err
//	    }
//	    return s.txMgr.Commit()
//	}
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive file operations
//   - github.com/joshuapare/hivekit/hive/dirty: Dirty page tracking and flushing
//   - github.com/joshuapare/hivekit/hive/merge: High-level merge API using transactions
//   - github.com/joshuapare/hivekit/hive/edit: Cell editing operations
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package tx
