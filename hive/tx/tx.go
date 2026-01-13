// Package tx provides transaction management for hive modifications.
//
// The transaction manager ensures durability and consistency by managing
// REGF sequence numbers and coordinating ordered flushes of dirty data.
//
// Transaction Protocol:
//  1. Begin() - Increment PrimarySeq, mark transaction as started
//  2. [Apply modifications - tracked by DirtyTracker]
//  3. Commit() - Flush data ranges, set SecondarySeq=PrimarySeq, flush header
//
// Crash Recovery:
// If a crash occurs between Begin() and Commit(), PrimarySeq != SecondarySeq,
// indicating an incomplete transaction. The hive should be validated before use.
package tx

import (
	"context"
	"fmt"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Manager handles REGF sequence numbers and coordinates ordered flushes
// to ensure transaction durability and crash recovery.
//
// The manager is NOT thread-safe. Only one goroutine should use it at a time.
type Manager struct {
	h    *hive.Hive             // Hive being modified
	dt   dirty.FlushableTracker // Dirty page tracker
	mode dirty.FlushMode        // Flush mode for commits
	seq  uint32                 // Current sequence number
	inTx bool                   // Whether a transaction is active
}

// NewManager creates a transaction manager for the given hive.
//
// Parameters:
//   - h: The hive to manage transactions for
//   - dt: Dirty tracker for recording and flushing modified pages
//   - mode: Flush mode for transaction commits
func NewManager(h *hive.Hive, dt dirty.FlushableTracker, mode dirty.FlushMode) *Manager {
	return &Manager{
		h:    h,
		dt:   dt,
		mode: mode,
		seq:  0,
		inTx: false,
	}
}

// Begin starts a new transaction.
//
// This method:
//  1. Reads the current PrimarySeq from the REGF header
//  2. Increments it by 1
//  3. Writes the new value to the header
//  4. Updates the timestamp to now (Windows FILETIME format)
//  5. Marks the header page as dirty
//  6. Sets inTx = true
//
// The transaction is NOT visible to readers until Commit() is called.
// If Begin() is called while already in a transaction, it's a no-op.
//
// The context can be used to cancel the operation before it starts.
//
// Performance: < 1 μs (header write only, no allocations).
func (m *Manager) Begin(ctx context.Context) error {
	// Check for cancellation before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	if m.inTx {
		// Already in transaction, idempotent
		return nil
	}

	data := m.h.Bytes()
	if len(data) < format.HeaderSize {
		return fmt.Errorf("hive data too small: %d bytes", len(data))
	}

	// Read current PrimarySeq
	m.seq = format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Increment and write back PrimarySeq
	newSeq := m.seq + 1
	format.PutU32(data, format.REGFPrimarySeqOffset, newSeq)

	// Update timestamp to now
	nowFiletime := format.TimeToFiletime(time.Now())
	format.PutU64(data, format.REGFTimeStampOffset, nowFiletime)

	// Mark header dirty
	m.dt.Add(0, format.HeaderSize)

	// Update state
	m.seq = newSeq
	m.inTx = true

	return nil
}

// Commit finalizes the transaction using ordered flush protocol:
//
//  1. Flush all dirty data pages (via msync)
//  2. Set SecondarySeq = PrimarySeq (marking transaction complete)
//  3. Update timestamp to reflect commit time
//  4. Recalculate and update REGF header checksum
//  5. Mark header dirty again
//  6. Flush header page
//  7. Optionally call fdatasync() based on FlushMode
//
// After Commit() returns successfully, all changes are durable and visible.
//
// IMPORTANT: tx.Manager owns ALL protocol fields (sequences, timestamp).
// Operations like Grow() only update structural fields (data size, checksum).
// This separation prevents sequence number conflicts when multiple operations
// occur within a single transaction.
//
// If Commit() is called without an active transaction, it's a no-op.
//
// The context can be used to cancel the operation. Note that partial commits
// may occur if cancelled mid-way through the flush process.
//
// Performance: Depends on dirty page count and OS page cache.
// Typical: 5-10ms for 100KB of dirty data.
func (m *Manager) Commit(ctx context.Context) error {
	if !m.inTx {
		// No active transaction
		return nil
	}

	// Step 1: Flush all dirty data pages (NOT header yet)
	if err := m.dt.FlushDataOnly(ctx); err != nil {
		return fmt.Errorf("flush data pages: %w", err)
	}

	// Check for cancellation before header updates
	if err := ctx.Err(); err != nil {
		return err
	}

	// Step 2: Set SecondarySeq = PrimarySeq (transaction complete marker)
	data := m.h.Bytes()
	format.PutU32(data, format.REGFSecondarySeqOffset, m.seq)

	// Step 2b: Update timestamp to reflect commit time
	// This was previously done by Grow() via TouchNowAndBumpSeq(), but that caused
	// sequence number conflicts. Now tx.Manager owns ALL protocol fields including timestamp.
	nowFiletime := format.TimeToFiletime(time.Now())
	format.PutU64(data, format.REGFTimeStampOffset, nowFiletime)

	// Step 3: Recalculate and update header checksum (AFTER timestamp update)
	checksum := calculateHeaderChecksum(data)
	format.PutU32(data, format.REGFCheckSumOffset, checksum)

	// Step 4: Mark header dirty again (for SecondarySeq and checksum updates)
	m.dt.Add(0, format.HeaderSize)

	// Step 5 & 6: Flush header and optionally fdatasync
	if err := m.dt.FlushHeaderAndMeta(ctx, m.mode); err != nil {
		return fmt.Errorf("flush header: %w", err)
	}

	// Transaction complete
	m.inTx = false
	return nil
}

// Rollback aborts the current transaction.
//
// This is a best-effort operation that:
//   - Sets inTx = false
//   - Does NOT restore PrimarySeq (would require another write)
//   - Does NOT flush any dirty pages
//
// After Rollback(), the hive is in an inconsistent state with
// PrimarySeq != SecondarySeq. The hive should be closed without
// further modifications.
//
// To fully recover, close the hive and reopen it (which will
// detect the sequence mismatch).
func (m *Manager) Rollback() {
	m.inTx = false
}

// InTransaction returns whether a transaction is currently active.
func (m *Manager) InTransaction() bool {
	return m.inTx
}

// CurrentSequence returns the current sequence number.
func (m *Manager) CurrentSequence() uint32 {
	return m.seq
}

// calculateHeaderChecksum computes the REGF header checksum.
//
// The checksum is the XOR of the first 508 bytes (127 dwords) of the header.
// The checksum field itself (at offset 0x1FC) is NOT included in the calculation.
//
// This matches the Windows Registry hive format specification and ensures
// compatibility with tools like hivexsh.
func calculateHeaderChecksum(data []byte) uint32 {
	if len(data) < format.REGFChecksumRegionLen {
		return 0
	}

	var checksum uint32

	// XOR the first 127 dwords (508 bytes)
	for i := range format.REGFChecksumDwords {
		offset := i * format.DWORDSize
		dword := format.ReadU32(data, offset)
		checksum ^= dword
	}

	return checksum
}
