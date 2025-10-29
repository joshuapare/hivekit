package repair

import (
	"fmt"
	"strings"
	"time"

)

// TransactionLog maintains a record of all repairs applied, enabling rollback
// if any repair fails. This ensures atomicity: either all repairs succeed or
// the hive is restored to its original state.
type TransactionLog struct {
	entries []LogEntry
}

// LogEntry records a single repair operation.
type LogEntry struct {
	Offset      uint64          // Offset where repair was applied
	Size        uint64          // Number of bytes modified
	OldData     []byte          // Original data (for rollback)
	NewData     []byte          // New data (for verification)
	Diagnostic  Diagnostic // Diagnostic that triggered this repair
	Module      string          // Module that applied the repair
	Timestamp   time.Time       // When the repair was applied
	Applied     bool            // Whether this repair was successfully applied
}

// NewTransactionLog creates a new transaction log.
func NewTransactionLog() *TransactionLog {
	return &TransactionLog{
		entries: make([]LogEntry, 0, 16), // Pre-allocate for common case
	}
}

// AddEntry logs a repair before applying it.
// This captures the original data so we can rollback if needed.
func (tl *TransactionLog) AddEntry(offset uint64, oldData, newData []byte, d Diagnostic, module string) {
	entry := LogEntry{
		Offset:      offset,
		Size:        uint64(len(oldData)),
		OldData:     append([]byte(nil), oldData...), // Deep copy
		NewData:     append([]byte(nil), newData...), // Deep copy
		Diagnostic:  d,
		Module:      module,
		Timestamp:   time.Now(),
		Applied:     false,
	}
	tl.entries = append(tl.entries, entry)
}

// MarkApplied marks the most recent entry as successfully applied.
// This is called after a repair completes without error.
func (tl *TransactionLog) MarkApplied() error {
	if len(tl.entries) == 0 {
		return &TransactionError{
			Operation: "mark_applied",
			Message:   "no entries in transaction log",
		}
	}
	tl.entries[len(tl.entries)-1].Applied = true
	return nil
}

// Rollback reverts all applied repairs in reverse order.
// This restores the data to its state before any repairs were applied.
// Returns the number of repairs rolled back and any error encountered.
func (tl *TransactionLog) Rollback(data []byte) (int, error) {
	if len(tl.entries) == 0 {
		return 0, nil
	}

	rolled := 0
	// Process entries in reverse order (most recent first)
	for i := len(tl.entries) - 1; i >= 0; i-- {
		entry := tl.entries[i]
		if !entry.Applied {
			continue // Skip unapplied entries
		}

		// Validate offset and size
		if entry.Offset+entry.Size > uint64(len(data)) {
			return rolled, &TransactionError{
				Operation: "rollback",
				Message:   fmt.Sprintf("invalid offset/size for entry %d: offset=0x%X size=%d datalen=%d", i, entry.Offset, entry.Size, len(data)),
			}
		}

		// Restore original data
		copy(data[entry.Offset:entry.Offset+entry.Size], entry.OldData)
		rolled++
	}

	return rolled, nil
}

// AppliedCount returns the number of successfully applied repairs.
func (tl *TransactionLog) AppliedCount() int {
	count := 0
	for _, entry := range tl.entries {
		if entry.Applied {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of entries in the log.
func (tl *TransactionLog) TotalCount() int {
	return len(tl.entries)
}

// Export generates a human-readable summary of all repairs.
// This is useful for logging and debugging.
func (tl *TransactionLog) Export() string {
	if len(tl.entries) == 0 {
		return "Transaction log: empty"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Transaction log: %d entries (%d applied)\n", len(tl.entries), tl.AppliedCount()))
	sb.WriteString(strings.Repeat("=", 80))
	sb.WriteString("\n")

	for i, entry := range tl.entries {
		status := "PENDING"
		if entry.Applied {
			status = "APPLIED"
		}

		sb.WriteString(fmt.Sprintf("\n[%d] %s - %s\n", i+1, status, entry.Module))
		sb.WriteString(fmt.Sprintf("  Offset:   0x%08X\n", entry.Offset))
		sb.WriteString(fmt.Sprintf("  Size:     %d bytes\n", entry.Size))
		sb.WriteString(fmt.Sprintf("  Issue:    %s\n", entry.Diagnostic.Issue))
		if entry.Diagnostic.Repair != nil {
			sb.WriteString(fmt.Sprintf("  Repair:   %s\n", entry.Diagnostic.Repair.Description))
		}
		sb.WriteString(fmt.Sprintf("  Time:     %s\n", entry.Timestamp.Format(time.RFC3339)))

		// Show byte changes (limited to first 32 bytes for readability)
		showBytes := entry.Size
		if showBytes > 32 {
			showBytes = 32
		}
		sb.WriteString(fmt.Sprintf("  Before:   % X", entry.OldData[:showBytes]))
		if entry.Size > 32 {
			sb.WriteString(" ...")
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  After:    % X", entry.NewData[:showBytes]))
		if entry.Size > 32 {
			sb.WriteString(" ...")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetEntries returns a copy of all log entries for external processing.
func (tl *TransactionLog) GetEntries() []LogEntry {
	entries := make([]LogEntry, len(tl.entries))
	copy(entries, tl.entries)
	return entries
}

// Clear removes all entries from the log.
// This should only be called when starting a new repair session.
func (tl *TransactionLog) Clear() {
	tl.entries = tl.entries[:0]
}
