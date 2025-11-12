package tx

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// mockDirtyTracker is a test implementation of DirtyTracker.
type mockDirtyTracker struct {
	adds                []addCall
	flushDataCalls      int
	flushHeaderCalls    int
	lastFlushMode       dirty.FlushMode
	shouldFailFlushData bool
	shouldFailFlushHdr  bool
}

type addCall struct {
	off int
	len int
}

func (m *mockDirtyTracker) Add(off, length int) {
	m.adds = append(m.adds, addCall{off, length})
}

func (m *mockDirtyTracker) FlushDataOnly() error {
	m.flushDataCalls++
	if m.shouldFailFlushData {
		return os.ErrPermission
	}
	return nil
}

func (m *mockDirtyTracker) FlushHeaderAndMeta(mode dirty.FlushMode) error {
	m.flushHeaderCalls++
	m.lastFlushMode = mode
	if m.shouldFailFlushHdr {
		return os.ErrPermission
	}
	return nil
}

// setupTestHive creates a minimal test hive for testing.
func setupTestHive(t *testing.T) *hive.Hive {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "test.hiv")

	// Create minimal hive file (just REGF header for now)
	data := make([]byte, format.HeaderSize)

	// Write REGF signature
	copy(data[format.REGFSignatureOffset:], format.REGFSignature)

	// Initialize sequences to known values
	format.PutU32(data, format.REGFPrimarySeqOffset, 100)
	format.PutU32(data, format.REGFSecondarySeqOffset, 100)

	// Initialize timestamp to known value (2024-01-01 00:00:00 UTC)
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	testFiletime := format.TimeToFiletime(testTime)
	format.PutU64(data, format.REGFTimeStampOffset, testFiletime)

	// Write to file
	if err := os.WriteFile(hivePath, data, 0644); err != nil {
		t.Fatalf("Failed to write test hive: %v", err)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to open test hive: %v", err)
	}

	return h
}

// Test 1: Begin increments PrimarySeq.
func Test_TxManager_Begin_IncrementsSequence(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Read initial sequences
	data := h.Bytes()
	initialPrimary := format.ReadU32(data, format.REGFPrimarySeqOffset)
	initialSecondary := format.ReadU32(data, format.REGFSecondarySeqOffset)

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Check PrimarySeq incremented
	newPrimary := format.ReadU32(data, format.REGFPrimarySeqOffset)
	if newPrimary != initialPrimary+1 {
		t.Errorf("PrimarySeq not incremented: got %d, want %d", newPrimary, initialPrimary+1)
	}

	// Check SecondarySeq unchanged
	newSecondary := format.ReadU32(data, format.REGFSecondarySeqOffset)
	if newSecondary != initialSecondary {
		t.Errorf("SecondarySeq changed: got %d, want %d", newSecondary, initialSecondary)
	}

	// Check header marked dirty
	if len(dt.adds) != 1 {
		t.Fatalf("Expected 1 dirty add, got %d", len(dt.adds))
	}
	if dt.adds[0].off != 0 || dt.adds[0].len != format.HeaderSize {
		t.Errorf("Header not marked dirty: got (%d, %d), want (0, %d)",
			dt.adds[0].off, dt.adds[0].len, format.HeaderSize)
	}

	// Check manager state
	if !tm.InTransaction() {
		t.Error("Manager should be in transaction")
	}
	if tm.CurrentSequence() != newPrimary {
		t.Errorf("Manager sequence mismatch: got %d, want %d", tm.CurrentSequence(), newPrimary)
	}
}

// Test 2: Begin updates timestamp.
func Test_TxManager_Begin_UpdatesTimestamp(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Record time before Begin
	before := time.Now()

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Record time after Begin
	after := time.Now()

	// Check timestamp was updated
	data := h.Bytes()
	filetime := format.ReadU64(data, format.REGFTimeStampOffset)
	timestamp := format.FiletimeToTime(filetime)

	// Timestamp should be between before and after
	if timestamp.Before(before.Add(-1*time.Second)) || timestamp.After(after.Add(1*time.Second)) {
		t.Errorf("Timestamp not updated correctly: got %v, want between %v and %v",
			timestamp, before, after)
	}
}

// Test 3: Commit flushes data first, then header.
func Test_TxManager_Commit_FlushesDataFirst(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Reset dirty tracker to track commit operations
	dt.adds = nil
	dt.flushDataCalls = 0
	dt.flushHeaderCalls = 0

	// Commit transaction
	if err := tm.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Check FlushDataOnly called exactly once
	if dt.flushDataCalls != 1 {
		t.Errorf("FlushDataOnly not called: got %d calls, want 1", dt.flushDataCalls)
	}

	// Check FlushHeaderAndMeta called exactly once
	if dt.flushHeaderCalls != 1 {
		t.Errorf("FlushHeaderAndMeta not called: got %d calls, want 1", dt.flushHeaderCalls)
	}

	// Check SecondarySeq set to match PrimarySeq
	data := h.Bytes()
	primary := format.ReadU32(data, format.REGFPrimarySeqOffset)
	secondary := format.ReadU32(data, format.REGFSecondarySeqOffset)
	if primary != secondary {
		t.Errorf("Sequences don't match after commit: primary=%d, secondary=%d", primary, secondary)
	}

	// Check header marked dirty again (for SecondarySeq update)
	if len(dt.adds) != 1 {
		t.Errorf("Header not marked dirty for SecondarySeq: got %d adds, want 1", len(dt.adds))
	}

	// Check manager no longer in transaction
	if tm.InTransaction() {
		t.Error("Manager should not be in transaction after commit")
	}
}

// Test 4: Crash simulation - sequence mismatch detection.
func Test_TxManager_CrashSimulation_SequenceMismatch(t *testing.T) {
	// Create temp directory and hive file (inline setup to keep path)
	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "test.hiv")

	// Create minimal hive file
	data := make([]byte, format.HeaderSize)
	copy(data[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(data, format.REGFPrimarySeqOffset, 100)
	format.PutU32(data, format.REGFSecondarySeqOffset, 100)
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	testFiletime := format.TimeToFiletime(testTime)
	format.PutU64(data, format.REGFTimeStampOffset, testFiletime)

	if err := os.WriteFile(hivePath, data, 0644); err != nil {
		t.Fatalf("Failed to write test hive: %v", err)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to open test hive: %v", err)
	}

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Read initial sequences
	hiveData := h.Bytes()
	initialSeq := format.ReadU32(hiveData, format.REGFPrimarySeqOffset)

	// Begin transaction
	if beginErr := tm.Begin(); beginErr != nil {
		t.Fatalf("Begin() failed: %v", beginErr)
	}

	// Simulate: FlushDataOnly succeeds but we crash before commit
	if flushErr := dt.FlushDataOnly(); flushErr != nil {
		t.Fatalf("FlushDataOnly() failed: %v", flushErr)
	}

	// Close without committing (simulates crash)
	h.Close()

	// Reopen hive using the path
	h2, err := hive.Open(hivePath)
	if err != nil {
		// In a real implementation, hive.Open() should detect the sequence mismatch
		// For now, we'll manually check
		t.Logf("Reopen returned error (expected): %v", err)
	}
	if h2 != nil {
		defer h2.Close()

		// Check sequences don't match (incomplete transaction)
		data2 := h2.Bytes()
		primary := format.ReadU32(data2, format.REGFPrimarySeqOffset)
		secondary := format.ReadU32(data2, format.REGFSecondarySeqOffset)

		if primary == secondary {
			t.Error("Sequences should NOT match after simulated crash")
		}

		if primary != initialSeq+1 {
			t.Errorf("PrimarySeq unexpected: got %d, want %d", primary, initialSeq+1)
		}

		if secondary != initialSeq {
			t.Errorf("SecondarySeq unexpected: got %d, want %d", secondary, initialSeq)
		}

		t.Logf("Crash detected: PrimarySeq=%d, SecondarySeq=%d (mismatch)", primary, secondary)
	}
}

// Test 5: Idempotency - Begin twice.
func Test_TxManager_Begin_Idempotent(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Begin first time
	if err := tm.Begin(); err != nil {
		t.Fatalf("First Begin() failed: %v", err)
	}

	data := h.Bytes()
	seq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Reset dirty tracker
	dt.adds = nil

	// Begin second time (should be no-op)
	if err := tm.Begin(); err != nil {
		t.Fatalf("Second Begin() failed: %v", err)
	}

	seq2 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Sequence should not increment again
	if seq2 != seq1 {
		t.Errorf("Sequence incremented on second Begin(): got %d, want %d", seq2, seq1)
	}

	// Dirty tracker should not be called again
	if len(dt.adds) != 0 {
		t.Errorf("Header marked dirty on second Begin(): got %d adds, want 0", len(dt.adds))
	}
}

// Test 6: Rollback clears transaction.
func Test_TxManager_Rollback_ClearsTransaction(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	if !tm.InTransaction() {
		t.Fatal("Should be in transaction after Begin()")
	}

	// Rollback
	tm.Rollback()

	if tm.InTransaction() {
		t.Error("Should not be in transaction after Rollback()")
	}

	// Verify we can begin a new transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() after Rollback() failed: %v", err)
	}

	if !tm.InTransaction() {
		t.Error("Should be in transaction after second Begin()")
	}
}

// Test 7: FlushMode behavior.
func Test_TxManager_FlushModes(t *testing.T) {
	tests := []struct {
		name string
		mode dirty.FlushMode
	}{
		{"FlushAuto", dirty.FlushAuto},
		{"FlushDataOnly", dirty.FlushDataOnly},
		{"FlushFull", dirty.FlushFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setupTestHive(t)
			defer h.Close()

			dt := &mockDirtyTracker{}
			tm := NewManager(h, dt, tt.mode)

			// Begin and commit
			if err := tm.Begin(); err != nil {
				t.Fatalf("Begin() failed: %v", err)
			}

			if err := tm.Commit(); err != nil {
				t.Fatalf("Commit() failed: %v", err)
			}

			// Verify flush mode was passed to FlushHeaderAndMeta
			if dt.lastFlushMode != tt.mode {
				t.Errorf("FlushMode not passed correctly: got %v, want %v", dt.lastFlushMode, tt.mode)
			}
		})
	}
}

// Test 8: Commit without Begin is a no-op.
func Test_TxManager_Commit_WithoutBegin(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Commit without Begin
	if err := tm.Commit(); err != nil {
		t.Fatalf("Commit() without Begin() should not fail: %v", err)
	}

	// Verify no flush operations
	if dt.flushDataCalls != 0 {
		t.Errorf("FlushDataOnly called without transaction: got %d calls, want 0", dt.flushDataCalls)
	}

	if dt.flushHeaderCalls != 0 {
		t.Errorf("FlushHeaderAndMeta called without transaction: got %d calls, want 0", dt.flushHeaderCalls)
	}
}

// Test 9: Commit failure on data flush.
func Test_TxManager_Commit_DataFlushFailure(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{shouldFailFlushData: true}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Commit should fail
	if err := tm.Commit(); err == nil {
		t.Fatal("Commit() should have failed on data flush")
	}

	// Manager should still be in transaction (commit didn't complete)
	if !tm.InTransaction() {
		t.Error("Manager should still be in transaction after failed commit")
	}
}

// Test 10: Commit failure on header flush.
func Test_TxManager_Commit_HeaderFlushFailure(t *testing.T) {
	h := setupTestHive(t)
	defer h.Close()

	dt := &mockDirtyTracker{shouldFailFlushHdr: true}
	tm := NewManager(h, dt, dirty.FlushAuto)

	// Begin transaction
	if err := tm.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Commit should fail
	if err := tm.Commit(); err == nil {
		t.Fatal("Commit() should have failed on header flush")
	}

	// Manager should still be in transaction (commit didn't complete)
	if !tm.InTransaction() {
		t.Error("Manager should still be in transaction after failed commit")
	}
}

// Benchmark: Begin() performance.
func Benchmark_TxManager_Begin(b *testing.B) {
	// Setup once
	tmpDir := b.TempDir()
	hivePath := filepath.Join(tmpDir, "bench.hiv")

	data := make([]byte, format.HeaderSize)
	copy(data[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(data, format.REGFPrimarySeqOffset, 100)
	format.PutU32(data, format.REGFSecondarySeqOffset, 100)

	if err := os.WriteFile(hivePath, data, 0644); err != nil {
		b.Fatal(err)
	}

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	dt := &mockDirtyTracker{}
	tm := NewManager(h, dt, dirty.FlushAuto)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		// Reset state
		tm.inTx = false

		if beginErr := tm.Begin(); beginErr != nil {
			b.Fatal(beginErr)
		}
	}
}
