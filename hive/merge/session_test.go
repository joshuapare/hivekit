package merge

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// setupTestSession creates a test session for session testing.
func setupTestSession(t *testing.T) (*Session, func()) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "session-test-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	// Create session with default options
	session, err := NewSession(h, DefaultOptions())
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create session: %v", err)
	}

	cleanup := func() {
		session.Close()
		h.Close()
	}

	return session, cleanup
}

// Test 1: Session Creation.
func Test_Session_NewSession(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Verify session created without error
	if session == nil {
		t.Fatal("Session should not be nil")
	}

	// Verify index built
	if session.idx == nil {
		t.Error("Index should be initialized")
	}

	// Verify allocator initialized
	if session.alloc == nil {
		t.Error("Allocator should be initialized")
	}

	// Verify tx/dirty components initialized
	if session.txMgr == nil {
		t.Error("Transaction manager should be initialized")
	}
	if session.dt == nil {
		t.Error("Dirty tracker should be initialized")
	}

	// Verify strategy initialized (Phase 3)
	if session.strategy == nil {
		t.Error("Strategy should be initialized")
	}

	t.Log("Session created successfully with all components initialized")
}

// Test 2: Begin/Commit.
func Test_Session_BeginCommit(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Read initial sequence numbers
	data := session.h.Bytes()
	if len(data) < format.HeaderSize {
		t.Fatal("Hive data too small")
	}

	initialSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Begin transaction
	if err := session.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Check PrimarySeq incremented
	afterBeginSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	if afterBeginSeq1 != initialSeq1+1 {
		t.Errorf("PrimarySeq should increment after Begin(): got %d, want %d", afterBeginSeq1, initialSeq1+1)
	}

	// Commit transaction
	if err := session.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Check SecondarySeq updated
	afterCommitSeq2 := format.ReadU32(data, format.REGFSecondarySeqOffset)

	if afterCommitSeq2 != afterBeginSeq1 {
		t.Errorf(
			"SecondarySeq should match PrimarySeq after Commit(): got %d, want %d",
			afterCommitSeq2,
			afterBeginSeq1,
		)
	}

	t.Logf(
		"Transaction sequences: Initial=%d, AfterBegin=%d, AfterCommit=%d",
		initialSeq1,
		afterBeginSeq1,
		afterCommitSeq2,
	)
}

// Test 3: Apply Single Operation.
func Test_Session_Apply_SingleKey(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create plan with single key
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "TestKey"})

	// Begin transaction
	if err := session.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Apply plan
	result, err := session.Apply(plan)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Commit transaction
	if commitErr := session.Commit(); commitErr != nil {
		t.Fatalf("Commit() failed: %v", commitErr)
	}

	// Verify statistics
	if result.KeysCreated < 1 {
		t.Errorf("Expected at least 1 key created, got %d", result.KeysCreated)
	}

	// Verify key exists in index
	rootRef := session.h.RootCellOffset()
	sessionTestRef, ok := session.idx.GetNK(rootRef, "_sessiontest")
	if !ok {
		t.Error("_SessionTest key should exist in index")
	} else {
		_, testKeyOk := session.idx.GetNK(sessionTestRef, "testkey")
		if !testKeyOk {
			t.Error("TestKey should exist under _SessionTest in index")
		}
	}

	t.Logf("Single key operation successful: %+v", result)
}

// Test 4: Apply Complex Plan.
func Test_Session_Apply_ComplexPlan(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create complex plan
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "App"})
	plan.AddSetValue([]string{"_SessionTest", "App"}, "Version", format.REGSZ, []byte("1.0\x00"))
	plan.AddSetValue([]string{"_SessionTest", "App"}, "Build", format.REGDWORD, []byte{42, 0, 0, 0})

	// Apply with transaction
	result, err := session.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx() failed: %v", err)
	}

	// Verify statistics
	if result.KeysCreated < 1 {
		t.Errorf("Expected at least 1 key created, got %d", result.KeysCreated)
	}
	if result.ValuesSet != 2 {
		t.Errorf("Expected 2 values set, got %d", result.ValuesSet)
	}

	// Verify keys and values exist in index
	rootRef := session.h.RootCellOffset()
	sessionTestRef, ok := session.idx.GetNK(rootRef, "_sessiontest")
	if !ok {
		t.Fatal("_SessionTest key should exist in index")
	}

	appRef, ok := session.idx.GetNK(sessionTestRef, "app")
	if !ok {
		t.Fatal("App key should exist under _SessionTest")
	}

	// Verify values exist
	_, ok = session.idx.GetVK(appRef, "version")
	if !ok {
		t.Error("Version value should exist")
	}
	_, ok = session.idx.GetVK(appRef, "build")
	if !ok {
		t.Error("Build value should exist")
	}

	t.Logf("Complex plan successful: %+v", result)
}

// Test 5: Rollback.
func Test_Session_Rollback(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Read initial sequence
	data := session.h.Bytes()
	initialSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Begin transaction
	if err := session.Begin(); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Apply plan (but don't commit)
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "Rollback"})
	if _, err := session.Apply(plan); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Rollback
	session.Rollback()

	// Verify transaction state cleared
	// Note: Since we use mmap, the data changes remain, but sequences should be inconsistent
	// This is expected behavior - rollback is best-effort for crash detection

	afterRollbackSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	afterRollbackSeq2 := format.ReadU32(data, format.REGFSecondarySeqOffset)

	// After rollback, PrimarySeq is incremented but SecondarySeq is not
	// This indicates an incomplete transaction
	if afterRollbackSeq1 != initialSeq1+1 {
		t.Errorf("PrimarySeq should be incremented: got %d, want %d", afterRollbackSeq1, initialSeq1+1)
	}
	if afterRollbackSeq2 == afterRollbackSeq1 {
		t.Error("SecondarySeq should NOT match PrimarySeq after rollback (indicates incomplete transaction)")
	}

	t.Logf(
		"Rollback test: Initial=%d, AfterRollback: Seq1=%d, Seq2=%d",
		initialSeq1,
		afterRollbackSeq1,
		afterRollbackSeq2,
	)
}

// Test 6: ApplyWithTx Convenience.
func Test_Session_ApplyWithTx(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create plan
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "Convenience"})
	plan.AddSetValue([]string{"_SessionTest", "Convenience"}, "Test", format.REGSZ, []byte("Value\x00"))

	// Read initial sequences
	data := session.h.Bytes()
	initialSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	// Apply with single call
	result, err := session.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx() failed: %v", err)
	}

	// Verify results
	if result.KeysCreated < 1 {
		t.Errorf("Expected at least 1 key created, got %d", result.KeysCreated)
	}
	if result.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", result.ValuesSet)
	}

	// Verify transaction committed (sequences should match)
	afterSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	afterSeq2 := format.ReadU32(data, format.REGFSecondarySeqOffset)

	if afterSeq1 != afterSeq2 {
		t.Errorf("Sequences should match after ApplyWithTx: Seq1=%d, Seq2=%d", afterSeq1, afterSeq2)
	}
	if afterSeq1 <= initialSeq1 {
		t.Errorf("Sequences should increment: Initial=%d, After=%d", initialSeq1, afterSeq1)
	}

	t.Logf("ApplyWithTx successful: %+v", result)
}

// Test 7: Large Value Handling.
func Test_Session_Apply_LargeValue(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create plan with 50KB value
	largeData := make([]byte, 50*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "LargeValue"})
	plan.AddSetValue([]string{"_SessionTest", "LargeValue"}, "BigData", format.REGBinary, largeData)

	// Apply with transaction
	result, err := session.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx() with large value failed: %v", err)
	}

	// Verify statistics
	if result.KeysCreated < 1 {
		t.Errorf("Expected at least 1 key created, got %d", result.KeysCreated)
	}
	if result.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", result.ValuesSet)
	}

	// Verify value exists in index
	rootRef := session.h.RootCellOffset()
	sessionTestRef, ok := session.idx.GetNK(rootRef, "_sessiontest")
	if !ok {
		t.Fatal("_SessionTest key should exist")
	}

	largeValueRef, ok := session.idx.GetNK(sessionTestRef, "largevalue")
	if !ok {
		t.Fatal("LargeValue key should exist")
	}

	vkRef, ok := session.idx.GetVK(largeValueRef, "bigdata")
	if !ok {
		t.Fatal("BigData value should exist in index")
	}

	// Verify we can read the value back
	payload, err := session.h.ResolveCellPayload(vkRef)
	if err != nil {
		t.Fatalf("Failed to resolve VK cell: %v", err)
	}

	if len(payload) < 24 {
		t.Fatalf("VK cell too small: %d bytes", len(payload))
	}

	// VK structure: signature(2) + nameLen(2) + dataLen(4) + dataOff(4) + type(4) + flags(2) + spare(2) + name
	dataLen := format.ReadU32(payload, 4)
	if dataLen != uint32(len(largeData)) {
		t.Errorf("Value data length mismatch: got %d, want %d", dataLen, len(largeData))
	}

	t.Logf("Large value test successful: %+v", result)
}

// Test 8: Transaction Sequences.
func Test_Session_TransactionSequences(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Helper functions - IMPORTANT: Call h.Bytes() fresh each time!
	// The hive may grow during operations, causing mmap remap, invalidating old pointers.
	getSeq1 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFPrimarySeqOffset)
	}
	getSeq2 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFSecondarySeqOffset)
	}

	initialSeq1 := getSeq1()
	initialSeq2 := getSeq2()

	// Apply first transaction
	plan1 := NewPlan()
	plan1.AddEnsureKey([]string{"_SessionTest", "Tx1"})
	if _, err := session.ApplyWithTx(plan1); err != nil {
		t.Fatalf("First transaction failed: %v", err)
	}

	afterTx1Seq1 := getSeq1()
	afterTx1Seq2 := getSeq2()

	// Verify sequences match after commit
	if afterTx1Seq1 != afterTx1Seq2 {
		t.Errorf("Sequences should match after Tx1: Seq1=%d, Seq2=%d", afterTx1Seq1, afterTx1Seq2)
	}
	// Verify sequences incremented
	if afterTx1Seq1 <= initialSeq1 {
		t.Errorf("Sequences should increment after Tx1: Initial=%d, After=%d", initialSeq1, afterTx1Seq1)
	}

	// Apply second transaction
	plan2 := NewPlan()
	plan2.AddEnsureKey([]string{"_SessionTest", "Tx2"})
	if _, err := session.ApplyWithTx(plan2); err != nil {
		t.Fatalf("Second transaction failed: %v", err)
	}

	afterTx2Seq1 := getSeq1()
	afterTx2Seq2 := getSeq2()

	// Verify sequences match after second commit
	if afterTx2Seq1 != afterTx2Seq2 {
		t.Errorf("Sequences should match after Tx2: Seq1=%d, Seq2=%d", afterTx2Seq1, afterTx2Seq2)
	}
	// Verify sequences incremented again
	if afterTx2Seq1 <= afterTx1Seq1 {
		t.Errorf("Sequences should increment after Tx2: AfterTx1=%d, AfterTx2=%d", afterTx1Seq1, afterTx2Seq1)
	}

	t.Logf("Transaction sequences: Initial=%d/%d, AfterTx1=%d/%d, AfterTx2=%d/%d",
		initialSeq1, initialSeq2, afterTx1Seq1, afterTx1Seq2, afterTx2Seq1, afterTx2Seq2)
}

// Test 9: NewSessionWithIndex (bonus test).
func Test_Session_NewSessionWithIndex(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "session-with-index-test")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session with NewSession (builds index)
	session1, err := NewSession(h, DefaultOptions())
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session1.Close()

	// Get the built index
	idx := session1.Index()

	// Close first session
	session1.Close()

	// Create second session reusing the index
	session2, err := NewSessionWithIndex(h, idx, DefaultOptions())
	if err != nil {
		t.Fatalf("NewSessionWithIndex failed: %v", err)
	}
	defer session2.Close()

	// Verify session works
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "ReuseIndex"})

	result, err := session2.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx with reused index failed: %v", err)
	}

	if result.KeysCreated < 1 {
		t.Errorf("Expected at least 1 key created, got %d", result.KeysCreated)
	}

	t.Log("Session with reused index successful")
}

// Test 10: Begin/Commit Idempotency - validates that Begin() is called exactly once per ApplyWithTx.
func Test_Session_BeginCommitIdempotency(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Helper functions - IMPORTANT: Call h.Bytes() fresh each time!
	// The hive may grow during operations, causing mmap remap, invalidating old pointers.
	getSeq1 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFPrimarySeqOffset)
	}
	getSeq2 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFSecondarySeqOffset)
	}

	// Check initial state after setupTestSession
	initialSeq1 := getSeq1()
	initialSeq2 := getSeq2()
	t.Logf("Initial state after setupTestSession: Seq1=%d, Seq2=%d", initialSeq1, initialSeq2)

	// Verify sequences are in sync initially
	if initialSeq1 != initialSeq2 {
		t.Errorf(
			"Initial sequences should match: Seq1=%d, Seq2=%d (indicates incomplete prior transaction)",
			initialSeq1,
			initialSeq2,
		)
	}

	// Create simple plan
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_IdempotencyTest"})

	// Apply with transaction - should call Begin() exactly once
	_, err := session.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Check final sequences
	finalSeq1 := getSeq1()
	finalSeq2 := getSeq2()
	t.Logf("After ApplyWithTx: Seq1=%d, Seq2=%d", finalSeq1, finalSeq2)

	// CRITICAL: Verify Begin() was called exactly once
	expectedSeq1 := initialSeq1 + 1
	if finalSeq1 != expectedSeq1 {
		extraBegins := finalSeq1 - expectedSeq1
		t.Errorf("Begin() called %d extra times! Expected Seq1=%d, got %d", extraBegins, expectedSeq1, finalSeq1)
	}

	// CRITICAL: Verify Commit() was called (Seq2 should match Seq1)
	if finalSeq2 != finalSeq1 {
		t.Errorf(
			"Commit() did not update SecondarySeq: Seq1=%d, Seq2=%d (transaction incomplete)",
			finalSeq1,
			finalSeq2,
		)
	}
}

// Test 11: Multiple operations in single transaction.
func Test_Session_MultipleOperations(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Helper functions - IMPORTANT: Call h.Bytes() fresh each time!
	// The hive may grow during operations, causing mmap remap, invalidating old pointers.
	getSeq1 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFPrimarySeqOffset)
	}
	getSeq2 := func() uint32 {
		data := session.h.Bytes()
		return format.ReadU32(data, format.REGFSecondarySeqOffset)
	}

	initialSeq1 := getSeq1()
	initialSeq2 := getSeq2()
	t.Logf("Initial sequences: Seq1=%d, Seq2=%d", initialSeq1, initialSeq2)

	// Create complex plan with many operations
	plan := NewPlan()

	// Create key hierarchy
	plan.AddEnsureKey([]string{"_SessionTest", "Multi", "Level1"})
	plan.AddEnsureKey([]string{"_SessionTest", "Multi", "Level2"})

	// Add values at different levels
	plan.AddSetValue([]string{"_SessionTest", "Multi"}, "Root", format.REGSZ, []byte("RootValue\x00"))
	plan.AddSetValue([]string{"_SessionTest", "Multi", "Level1"}, "Value1", format.REGDWORD, []byte{1, 0, 0, 0})
	plan.AddSetValue([]string{"_SessionTest", "Multi", "Level2"}, "Value2", format.REGDWORD, []byte{2, 0, 0, 0})

	// Add large value
	largeData := bytes.Repeat([]byte{0xAB}, 20*1024)
	plan.AddSetValue([]string{"_SessionTest", "Multi"}, "Large", format.REGBinary, largeData)

	// Apply all operations in single transaction
	result, err := session.ApplyWithTx(plan)
	if err != nil {
		t.Fatalf("ApplyWithTx with multiple operations failed: %v", err)
	}

	t.Logf("Multiple operations result: %+v", result)

	// Verify all operations succeeded
	if result.KeysCreated < 2 {
		t.Errorf("Expected at least 2 keys created, got %d", result.KeysCreated)
	}
	if result.ValuesSet != 4 {
		t.Errorf("Expected 4 values set, got %d", result.ValuesSet)
	}

	// CRITICAL: Verify transaction idempotency
	finalSeq1 := getSeq1()
	finalSeq2 := getSeq2()
	t.Logf("Final sequences: Seq1=%d, Seq2=%d", finalSeq1, finalSeq2)

	// Verify Begin() called exactly once
	expectedSeq1 := initialSeq1 + 1
	if finalSeq1 != expectedSeq1 {
		extraBegins := finalSeq1 - expectedSeq1
		t.Errorf("IDEMPOTENCY VIOLATION: Begin() called %d extra times! Expected Seq1=%d, got %d",
			extraBegins, expectedSeq1, finalSeq1)
	}

	// Verify Commit() completed (sequences match)
	if finalSeq2 != finalSeq1 {
		t.Errorf(
			"TRANSACTION INCOMPLETE: Seq1=%d, Seq2=%d (Commit() did not update SecondarySeq)",
			finalSeq1,
			finalSeq2,
		)
	}
}
