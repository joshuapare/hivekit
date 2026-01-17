package merge

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/walker"
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
	session, err := NewSession(context.Background(), h, DefaultOptions())
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create session: %v", err)
	}

	cleanup := func() {
		session.Close(context.Background())
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
	if err := session.Begin(context.Background()); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Check PrimarySeq incremented
	afterBeginSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)

	if afterBeginSeq1 != initialSeq1+1 {
		t.Errorf("PrimarySeq should increment after Begin(): got %d, want %d", afterBeginSeq1, initialSeq1+1)
	}

	// Commit transaction
	if err := session.Commit(context.Background()); err != nil {
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
	if err := session.Begin(context.Background()); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Apply plan
	result, err := session.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Commit transaction
	if commitErr := session.Commit(context.Background()); commitErr != nil {
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
	result, err := session.ApplyWithTx(context.Background(), plan)
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
	if err := session.Begin(context.Background()); err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}

	// Apply plan (but don't commit)
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "Rollback"})
	if _, err := session.Apply(context.Background(), plan); err != nil {
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
	result, err := session.ApplyWithTx(context.Background(), plan)
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
	result, err := session.ApplyWithTx(context.Background(), plan)
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
	if _, err := session.ApplyWithTx(context.Background(), plan1); err != nil {
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
	if _, err := session.ApplyWithTx(context.Background(), plan2); err != nil {
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
	session1, err := NewSession(context.Background(), h, DefaultOptions())
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session1.Close(context.Background())

	// Get the built index
	idx := session1.Index()

	// Close first session
	session1.Close(context.Background())

	// Create second session reusing the index
	session2, err := NewSessionWithIndex(context.Background(), h, idx, DefaultOptions())
	if err != nil {
		t.Fatalf("NewSessionWithIndex failed: %v", err)
	}
	defer session2.Close(context.Background())

	// Verify session works
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SessionTest", "ReuseIndex"})

	result, err := session2.ApplyWithTx(context.Background(), plan)
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
	_, err := session.ApplyWithTx(context.Background(), plan)
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
	result, err := session.ApplyWithTx(context.Background(), plan)
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

// =============================================================================
// Stats and Key Enumeration Tests
// =============================================================================

// Test 12: GetStorageStats returns valid stats.
func Test_Session_GetStorageStats(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	stats := session.GetStorageStats()

	// File size should be positive
	if stats.FileSize <= 0 {
		t.Errorf("FileSize should be positive, got %d", stats.FileSize)
	}

	// UsedBytes should be positive
	if stats.UsedBytes <= 0 {
		t.Errorf("UsedBytes should be positive, got %d", stats.UsedBytes)
	}

	// FreePercent should be between 0 and 100
	if stats.FreePercent < 0 || stats.FreePercent > 100 {
		t.Errorf("FreePercent should be 0-100, got %f", stats.FreePercent)
	}

	t.Logf("StorageStats: FileSize=%d, FreeBytes=%d, UsedBytes=%d, FreePercent=%.2f%%",
		stats.FileSize, stats.FreeBytes, stats.UsedBytes, stats.FreePercent)
}

// Test 13: GetHiveStats returns combined stats.
func Test_Session_GetHiveStats(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	stats := session.GetHiveStats()

	// Verify storage stats are populated
	if stats.Storage.FileSize <= 0 {
		t.Errorf("Storage.FileSize should be positive, got %d", stats.Storage.FileSize)
	}

	// Verify efficiency stats are populated
	if stats.Efficiency.TotalCapacity <= 0 {
		t.Errorf("Efficiency.TotalCapacity should be positive, got %d", stats.Efficiency.TotalCapacity)
	}

	t.Logf("HiveStats: FileSize=%d, TotalCapacity=%d, OverallEfficiency=%.2f%%",
		stats.Storage.FileSize, stats.Efficiency.TotalCapacity, stats.Efficiency.OverallEfficiency)
}

// Test 14: WalkKeys enumerates all keys.
func Test_Session_WalkKeys(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	var keyCount int
	var maxDepth int

	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		keyCount++
		depth := len(info.Path)
		if depth > maxDepth {
			maxDepth = depth
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys failed: %v", err)
	}

	// Should find at least the root key
	if keyCount < 1 {
		t.Errorf("Expected at least 1 key, found %d", keyCount)
	}

	t.Logf("WalkKeys: Found %d keys, max depth %d", keyCount, maxDepth)
}

// Test 15: ListAllKeys returns paths.
func Test_Session_ListAllKeys(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	keys, err := session.ListAllKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAllKeys failed: %v", err)
	}

	// Should find at least the root key
	if len(keys) < 1 {
		t.Errorf("Expected at least 1 key, found %d", len(keys))
	}

	// First few keys should have backslash-separated paths
	for i := 0; i < len(keys) && i < 5; i++ {
		t.Logf("Key %d: %s", i, keys[i])
	}

	t.Logf("ListAllKeys: Found %d keys", len(keys))
}

// Test 16: HasKey returns correct results.
func Test_Session_HasKey(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// First, create a known key
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_HasKeyTest", "ExistingKey"})
	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Test existing key
	if !session.HasKey("_HasKeyTest\\ExistingKey") {
		t.Error("HasKey should return true for existing key")
	}

	// Test non-existing key
	if session.HasKey("_HasKeyTest\\NonExistentKey") {
		t.Error("HasKey should return false for non-existing key")
	}

	// Test partial path that exists
	if !session.HasKey("_HasKeyTest") {
		t.Error("HasKey should return true for parent key")
	}
}

// Test 17: HasKeys returns correct results with detailed info.
func Test_Session_HasKeys(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create some known keys
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_HasKeysTest", "Key1"})
	plan.AddEnsureKey([]string{"_HasKeysTest", "Key2"})
	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("Failed to create test keys: %v", err)
	}

	// Test with mix of existing and non-existing keys
	result, err := session.HasKeys(context.Background(),
		"_HasKeysTest\\Key1",
		"_HasKeysTest\\Key2",
		"_HasKeysTest\\NonExistent",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	// AllPresent should be false (one missing)
	if result.AllPresent {
		t.Error("AllPresent should be false when some keys are missing")
	}

	// Should have 2 present
	if len(result.Present) != 2 {
		t.Errorf("Expected 2 present keys, got %d", len(result.Present))
	}

	// Should have 1 missing
	if len(result.Missing) != 1 {
		t.Errorf("Expected 1 missing key, got %d", len(result.Missing))
	}

	// Missing should contain the non-existent key
	found := false
	for _, missing := range result.Missing {
		if missing == "_HasKeysTest\\NonExistent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Missing should contain '_HasKeysTest\\NonExistent'")
	}

	t.Logf("HasKeys: AllPresent=%v, Present=%v, Missing=%v",
		result.AllPresent, result.Present, result.Missing)
}

// Test 18: HasKeys with all present.
func Test_Session_HasKeys_AllPresent(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create known keys
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_HasKeysAllTest", "A"})
	plan.AddEnsureKey([]string{"_HasKeysAllTest", "B"})
	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("Failed to create test keys: %v", err)
	}

	// Check all existing keys
	result, err := session.HasKeys(context.Background(),
		"_HasKeysAllTest\\A",
		"_HasKeysAllTest\\B",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	if !result.AllPresent {
		t.Error("AllPresent should be true when all keys exist")
	}

	if len(result.Missing) != 0 {
		t.Errorf("Expected no missing keys, got %d", len(result.Missing))
	}
}

// Test 19: HasKeys with empty input.
func Test_Session_HasKeys_Empty(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Check with no keys
	result, err := session.HasKeys(context.Background())
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	// AllPresent should be true for empty input (vacuously true)
	if !result.AllPresent {
		t.Error("AllPresent should be true for empty input")
	}

	if len(result.Present) != 0 {
		t.Errorf("Present should be empty, got %d", len(result.Present))
	}

	if len(result.Missing) != 0 {
		t.Errorf("Missing should be empty, got %d", len(result.Missing))
	}
}

// Test 20: WalkKeys with early stop.
func Test_Session_WalkKeys_EarlyStop(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// First count total keys
	var totalKeys int
	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		totalKeys++
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys (count) failed: %v", err)
	}

	// Now test early stop
	var keyCount int
	maxKeys := 5

	err = session.WalkKeys(context.Background(), func(info KeyInfo) error {
		keyCount++
		if keyCount >= maxKeys {
			return walker.ErrStopWalk
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys failed: %v", err)
	}

	// Should stop before processing all keys
	if keyCount >= totalKeys {
		t.Errorf("Expected early stop before %d keys, but processed all %d", totalKeys, keyCount)
	}

	// Should have processed at least maxKeys (the point where we requested stop)
	if keyCount < maxKeys {
		t.Errorf("Expected at least %d keys, got %d", maxKeys, keyCount)
	}

	t.Logf("WalkKeys with early stop: Processed %d of %d total keys", keyCount, totalKeys)
}

// Test 21: WalkKeys with cancellation.
func Test_Session_WalkKeys_Cancelled(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := session.WalkKeys(ctx, func(info KeyInfo) error {
		t.Error("Callback should not be called with cancelled context")
		return nil
	})

	if err == nil {
		t.Error("Expected error with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// Test 22: HasKeys with cancellation.
func Test_Session_HasKeys_Cancelled(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := session.HasKeys(ctx, "SomeKey")
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// =============================================================================
// Additional Comprehensive Tests for Stats and Key Enumeration
// =============================================================================

// Test 23: GetStorageStats after modifications shows changes.
func Test_Session_GetStorageStats_AfterModifications(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Get initial stats
	initialStats := session.GetStorageStats()

	// Make some modifications
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_StatsTest", "Key1"})
	plan.AddEnsureKey([]string{"_StatsTest", "Key2"})
	plan.AddEnsureKey([]string{"_StatsTest", "Key3"})
	plan.AddSetValue([]string{"_StatsTest"}, "Data", format.REGBinary, make([]byte, 1024))

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Get stats after modifications
	afterStats := session.GetStorageStats()

	// UsedBytes should increase
	if afterStats.UsedBytes <= initialStats.UsedBytes {
		t.Errorf("UsedBytes should increase after adding data: before=%d, after=%d",
			initialStats.UsedBytes, afterStats.UsedBytes)
	}

	t.Logf("Stats change: UsedBytes %d -> %d (delta: %d)",
		initialStats.UsedBytes, afterStats.UsedBytes, afterStats.UsedBytes-initialStats.UsedBytes)
}

// Test 24: GetHiveStats consistency check.
func Test_Session_GetHiveStats_Consistency(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	stats := session.GetHiveStats()

	// Verify FreeBytes matches TotalWasted
	if stats.Storage.FreeBytes != stats.Efficiency.TotalWasted {
		t.Errorf("Storage.FreeBytes (%d) should match Efficiency.TotalWasted (%d)",
			stats.Storage.FreeBytes, stats.Efficiency.TotalWasted)
	}

	// Verify UsedBytes matches TotalAllocated
	if stats.Storage.UsedBytes != stats.Efficiency.TotalAllocated {
		t.Errorf("Storage.UsedBytes (%d) should match Efficiency.TotalAllocated (%d)",
			stats.Storage.UsedBytes, stats.Efficiency.TotalAllocated)
	}

	// Verify efficiency is reasonable (0-100%)
	if stats.Efficiency.OverallEfficiency < 0 || stats.Efficiency.OverallEfficiency > 100 {
		t.Errorf("OverallEfficiency should be 0-100, got %.2f", stats.Efficiency.OverallEfficiency)
	}

	// Verify TotalHBINs is positive
	if stats.Efficiency.TotalHBINs <= 0 {
		t.Errorf("TotalHBINs should be positive, got %d", stats.Efficiency.TotalHBINs)
	}

	t.Logf("HiveStats consistency: TotalHBINs=%d, Efficiency=%.2f%%",
		stats.Efficiency.TotalHBINs, stats.Efficiency.OverallEfficiency)
}

// Test 25: WalkKeys verifies KeyInfo fields are correct.
func Test_Session_WalkKeys_KeyInfoFields(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create a key with known subkeys and values
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_KeyInfoTest"})
	plan.AddEnsureKey([]string{"_KeyInfoTest", "Child1"})
	plan.AddEnsureKey([]string{"_KeyInfoTest", "Child2"})
	plan.AddSetValue([]string{"_KeyInfoTest"}, "Value1", format.REGSZ, []byte("test\x00"))
	plan.AddSetValue([]string{"_KeyInfoTest"}, "Value2", format.REGDWORD, []byte{1, 0, 0, 0})

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Find our test key and verify its fields
	var foundKey *KeyInfo
	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		if len(info.Path) >= 1 && info.Path[len(info.Path)-1] == "_KeyInfoTest" {
			// Make a copy
			copy := info
			foundKey = &copy
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys failed: %v", err)
	}

	if foundKey == nil {
		t.Fatal("_KeyInfoTest key not found")
	}

	// Verify SubkeyCount
	if foundKey.SubkeyCount != 2 {
		t.Errorf("Expected SubkeyCount=2, got %d", foundKey.SubkeyCount)
	}

	// Verify ValueCount
	if foundKey.ValueCount != 2 {
		t.Errorf("Expected ValueCount=2, got %d", foundKey.ValueCount)
	}

	// Verify Offset is non-zero
	if foundKey.Offset == 0 {
		t.Error("Expected non-zero Offset")
	}

	t.Logf("KeyInfo verified: Path=%v, SubkeyCount=%d, ValueCount=%d, Offset=%d",
		foundKey.Path, foundKey.SubkeyCount, foundKey.ValueCount, foundKey.Offset)
}

// Test 26: WalkKeys with callback error (non-ErrStopWalk).
func Test_Session_WalkKeys_CallbackError(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	expectedErr := errors.New("test callback error")

	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		return expectedErr
	})

	if err == nil {
		t.Error("Expected error from callback")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected callback error, got: %v", err)
	}
}

// Test 27: ListAllKeys with context cancellation.
func Test_Session_ListAllKeys_Cancelled(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := session.ListAllKeys(ctx)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// Test 28: ListAllKeys path format verification.
func Test_Session_ListAllKeys_PathFormat(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create a nested key structure
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_PathTest", "Level1", "Level2", "Level3"})

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	keys, err := session.ListAllKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAllKeys failed: %v", err)
	}

	// Find our deep key
	expectedPath := "_PathTest\\Level1\\Level2\\Level3"
	found := false
	for _, key := range keys {
		if strings.HasSuffix(key, expectedPath) {
			found = true
			// Verify backslash separation
			if !strings.Contains(key, "\\") {
				t.Errorf("Path should contain backslashes: %s", key)
			}
			break
		}
	}

	if !found {
		t.Errorf("Expected to find key ending with %q", expectedPath)
	}
}

// Test 29: HasKey with first-level subkey.
func Test_Session_HasKey_FirstLevelSubkey(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Get a first-level subkey name from WalkKeys (depth == 2: root + subkey)
	var subkeyName string
	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		if len(info.Path) == 2 {
			subkeyName = info.Path[1] // First subkey under root
			return walker.ErrStopWalk
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys failed: %v", err)
	}

	if subkeyName == "" {
		t.Skip("Could not find a first-level subkey")
	}

	// First-level subkey should exist
	if !session.HasKey(subkeyName) {
		t.Errorf("First-level subkey %q should exist", subkeyName)
	}

	t.Logf("First-level subkey %q exists", subkeyName)
}

// Test 30: HasKey with deeply nested path.
func Test_Session_HasKey_DeepPath(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create a deeply nested key
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_DeepTest", "L1", "L2", "L3", "L4", "L5"})

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Test full path exists
	if !session.HasKey("_DeepTest\\L1\\L2\\L3\\L4\\L5") {
		t.Error("Deep nested key should exist")
	}

	// Test partial paths exist
	if !session.HasKey("_DeepTest\\L1\\L2") {
		t.Error("Partial path should exist")
	}

	// Test path beyond what exists
	if session.HasKey("_DeepTest\\L1\\L2\\L3\\L4\\L5\\L6") {
		t.Error("Non-existent deeper path should not exist")
	}
}

// Test 31: HasKeys with all missing.
func Test_Session_HasKeys_AllMissing(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	result, err := session.HasKeys(context.Background(),
		"_NonExistent1",
		"_NonExistent2",
		"_NonExistent3",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	if result.AllPresent {
		t.Error("AllPresent should be false when all keys are missing")
	}

	if len(result.Present) != 0 {
		t.Errorf("Present should be empty, got %d", len(result.Present))
	}

	if len(result.Missing) != 3 {
		t.Errorf("Missing should have 3 keys, got %d", len(result.Missing))
	}
}

// Test 32: HasKeys preserves order.
func Test_Session_HasKeys_PreservesOrder(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create keys
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_OrderTest", "A"})
	plan.AddEnsureKey([]string{"_OrderTest", "B"})
	plan.AddEnsureKey([]string{"_OrderTest", "C"})

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Check in specific order
	result, err := session.HasKeys(context.Background(),
		"_OrderTest\\C",
		"_OrderTest\\A",
		"_OrderTest\\B",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	// Verify order is preserved
	expectedOrder := []string{"_OrderTest\\C", "_OrderTest\\A", "_OrderTest\\B"}
	if len(result.Present) != len(expectedOrder) {
		t.Fatalf("Expected %d present keys, got %d", len(expectedOrder), len(result.Present))
	}

	for i, expected := range expectedOrder {
		if result.Present[i] != expected {
			t.Errorf("Present[%d] = %q, want %q", i, result.Present[i], expected)
		}
	}
}

// Test 33: WalkKeys depth-first order verification.
// Depth-first guarantees: parent is visited before children.
// Sibling order depends on hive storage (hash-based), so we only verify parent-child ordering.
func Test_Session_WalkKeys_DepthFirst(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Create a structure where depth-first is observable
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_DFSTest"})
	plan.AddEnsureKey([]string{"_DFSTest", "A"})
	plan.AddEnsureKey([]string{"_DFSTest", "A", "A1"})
	plan.AddEnsureKey([]string{"_DFSTest", "B"})

	if _, err := session.ApplyWithTx(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	// Collect keys in our test subtree
	var visitOrder []string
	err := session.WalkKeys(context.Background(), func(info KeyInfo) error {
		lastPart := info.Path[len(info.Path)-1]
		if strings.HasPrefix(lastPart, "_DFSTest") || lastPart == "A" || lastPart == "A1" || lastPart == "B" {
			visitOrder = append(visitOrder, lastPart)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkKeys failed: %v", err)
	}

	t.Logf("Visit order: %v", visitOrder)

	// Find positions
	var dfsPos, aPos, a1Pos, bPos int = -1, -1, -1, -1
	for i, name := range visitOrder {
		switch name {
		case "_DFSTest":
			dfsPos = i
		case "A":
			aPos = i
		case "A1":
			a1Pos = i
		case "B":
			bPos = i
		}
	}

	// Verify all keys were found
	if dfsPos == -1 || aPos == -1 || a1Pos == -1 || bPos == -1 {
		t.Skipf("Not all test keys found in visit order: %v", visitOrder)
	}

	// Depth-first invariants:
	// 1. _DFSTest must come before all its children (A, B, A1)
	// 2. A must come before A1 (parent before child)
	// Note: Order of A vs B depends on hive storage, so not tested

	if dfsPos >= aPos {
		t.Errorf("_DFSTest(%d) should come before A(%d)", dfsPos, aPos)
	}
	if dfsPos >= bPos {
		t.Errorf("_DFSTest(%d) should come before B(%d)", dfsPos, bPos)
	}
	if dfsPos >= a1Pos {
		t.Errorf("_DFSTest(%d) should come before A1(%d)", dfsPos, a1Pos)
	}
	if aPos >= a1Pos {
		t.Errorf("A(%d) should come before A1(%d)", aPos, a1Pos)
	}
}

// Test 34: GetStorageStats non-zero values.
func Test_Session_GetStorageStats_NonZeroValues(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	stats := session.GetStorageStats()

	// Verify all stats are non-zero for a real hive
	if stats.FileSize == 0 {
		t.Error("FileSize should be non-zero")
	}

	if stats.UsedBytes == 0 {
		t.Error("UsedBytes should be non-zero")
	}

	// FreeBytes can be zero for a well-packed hive, but let's log it
	t.Logf("Stats: FileSize=%d, UsedBytes=%d, FreeBytes=%d, FreePercent=%.2f%%",
		stats.FileSize, stats.UsedBytes, stats.FreeBytes, stats.FreePercent)

	// FreePercent should be in valid range [0, 100]
	if stats.FreePercent < 0 || stats.FreePercent > 100 {
		t.Errorf("FreePercent should be in range [0,100], got %.2f", stats.FreePercent)
	}

	// Sanity check: UsedBytes + FreeBytes should be <= FileSize
	// (they may not equal exactly due to header space)
	if stats.UsedBytes+stats.FreeBytes > stats.FileSize {
		t.Errorf("UsedBytes(%d) + FreeBytes(%d) > FileSize(%d)",
			stats.UsedBytes, stats.FreeBytes, stats.FileSize)
	}
}

// =============================================================================
// ApplyRegText / ApplyRegTextWithPrefix Tests
// =============================================================================

// Test 35: ApplyRegTextWithPrefix basic functionality.
func Test_Session_ApplyRegTextWithPrefix(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	regText := `Windows Registry Editor Version 5.00

[TestApp\Settings]
"Version"="1.0.0"
"Enabled"=dword:00000001
`

	// Apply with prefix
	applied, err := session.ApplyRegTextWithPrefix(context.Background(), regText, "_RegTextTest")
	if err != nil {
		t.Fatalf("ApplyRegTextWithPrefix failed: %v", err)
	}

	// Should have created keys and set values
	if applied.KeysCreated == 0 {
		t.Error("Expected keys to be created")
	}
	if applied.ValuesSet == 0 {
		t.Error("Expected values to be set")
	}

	// Verify key exists
	if !session.HasKey("_RegTextTest\\TestApp\\Settings") {
		t.Error("Key _RegTextTest\\TestApp\\Settings should exist")
	}

	t.Logf("ApplyRegTextWithPrefix: KeysCreated=%d, ValuesSet=%d",
		applied.KeysCreated, applied.ValuesSet)
}

// Test 36: ApplyRegTextWithPrefix then check stats (key use case).
func Test_Session_ApplyRegTextWithPrefix_ThenStats(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Get initial stats
	initialStats := session.GetStorageStats()

	regText := `Windows Registry Editor Version 5.00

[LargeKey\Sub1]
"Value1"="data1"

[LargeKey\Sub2]
"Value2"="data2"

[LargeKey\Sub3]
"Value3"="data3"
`

	// Apply regtext
	applied, err := session.ApplyRegTextWithPrefix(context.Background(), regText, "_StatsTest")
	if err != nil {
		t.Fatalf("ApplyRegTextWithPrefix failed: %v", err)
	}

	// Get stats after apply
	afterStats := session.GetStorageStats()

	// Stats should reflect changes
	if afterStats.UsedBytes <= initialStats.UsedBytes {
		t.Errorf("UsedBytes should increase after apply: before=%d, after=%d",
			initialStats.UsedBytes, afterStats.UsedBytes)
	}

	t.Logf("Stats: before UsedBytes=%d, after UsedBytes=%d, applied KeysCreated=%d",
		initialStats.UsedBytes, afterStats.UsedBytes, applied.KeysCreated)
}

// Test 37: ApplyRegTextWithPrefix then check keys (key use case).
func Test_Session_ApplyRegTextWithPrefix_ThenHasKeys(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	regText := `Windows Registry Editor Version 5.00

[Microsoft\App1]
"Name"="App1"

[Microsoft\App2]
"Name"="App2"
`

	// Apply regtext
	_, err := session.ApplyRegTextWithPrefix(context.Background(), regText, "_HasKeysCheck")
	if err != nil {
		t.Fatalf("ApplyRegTextWithPrefix failed: %v", err)
	}

	// Check keys using HasKeys
	result, err := session.HasKeys(context.Background(),
		"_HasKeysCheck\\Microsoft\\App1",
		"_HasKeysCheck\\Microsoft\\App2",
		"_HasKeysCheck\\Microsoft\\NonExistent",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	// Should have 2 present, 1 missing
	if len(result.Present) != 2 {
		t.Errorf("Expected 2 present keys, got %d: %v", len(result.Present), result.Present)
	}
	if len(result.Missing) != 1 {
		t.Errorf("Expected 1 missing key, got %d: %v", len(result.Missing), result.Missing)
	}
	if result.AllPresent {
		t.Error("AllPresent should be false")
	}

	t.Logf("HasKeys result: Present=%v, Missing=%v", result.Present, result.Missing)
}

// Test 38: ApplyRegText (without prefix).
func Test_Session_ApplyRegText(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	regText := `Windows Registry Editor Version 5.00

[_NoPrefix\SubKey]
"TestValue"="test"
`

	// Apply without prefix
	applied, err := session.ApplyRegText(context.Background(), regText)
	if err != nil {
		t.Fatalf("ApplyRegText failed: %v", err)
	}

	if applied.KeysCreated == 0 {
		t.Error("Expected keys to be created")
	}

	// Verify key exists at root level (no prefix)
	if !session.HasKey("_NoPrefix\\SubKey") {
		t.Error("Key _NoPrefix\\SubKey should exist")
	}
}

// Test 39: ApplyRegTextWithPrefix with invalid regtext.
func Test_Session_ApplyRegTextWithPrefix_InvalidRegtext(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	// Invalid regtext (missing header)
	regText := `[InvalidKey]
"Value"="test"
`

	_, err := session.ApplyRegTextWithPrefix(context.Background(), regText, "PREFIX")
	if err == nil {
		t.Error("Expected error for invalid regtext")
	}
}

// Test 40: ApplyRegTextWithPrefix with HKEY prefix stripping.
func Test_Session_ApplyRegTextWithPrefix_HKEYStripping(t *testing.T) {
	session, cleanup := setupTestSession(t)
	defer cleanup()

	regText := `Windows Registry Editor Version 5.00

[TestKey]
"Value"="data"
`

	// Prefix includes HKLM\ which should be stripped
	applied, err := session.ApplyRegTextWithPrefix(context.Background(), regText, "HKLM\\_HKEYTest")
	if err != nil {
		t.Fatalf("ApplyRegTextWithPrefix failed: %v", err)
	}

	if applied.KeysCreated == 0 {
		t.Error("Expected keys to be created")
	}

	// Key should be at _HKEYTest\TestKey (HKLM\ stripped)
	if !session.HasKey("_HKEYTest\\TestKey") {
		t.Error("Key _HKEYTest\\TestKey should exist (HKLM\\ should be stripped)")
	}
}

// Test 41: Single-pass mode works correctly.
func Test_Session_SinglePassMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "single-pass-test-hive")
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

	// Create a small plan (should trigger single-pass mode with IndexModeAuto)
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_SinglePassTest", "SubKey"})
	plan.AddSetValue([]string{"_SinglePassTest", "SubKey"}, "TestValue", format.REGSZ, []byte("hello\x00"))

	ctx := context.Background()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session for plan with explicit single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session, err := NewSessionForPlan(ctx, h, plan, opts)
	if err != nil {
		t.Fatalf("NewSessionForPlan failed: %v", err)
	}
	defer session.Close(ctx)

	// Verify single-pass mode is active
	if !session.IsSinglePassMode() {
		t.Error("Session should be in single-pass mode")
	}

	// Apply plan using single-pass direct method
	applied, err := session.ApplyPlanDirect(ctx, plan)
	if err != nil {
		t.Fatalf("ApplyPlanDirect failed: %v", err)
	}

	// Verify results
	if applied.KeysCreated != 2 {
		t.Errorf("Expected 2 keys created, got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", applied.ValuesSet)
	}

	// GetStorageStats should work in single-pass mode
	stats := session.GetStorageStats()
	if stats.FileSize == 0 {
		t.Error("FileSize should be non-zero")
	}
	if stats.UsedBytes == 0 {
		t.Error("UsedBytes should be non-zero")
	}

	t.Logf("Single-pass mode: Keys=%d, Values=%d, FileSize=%d, UsedBytes=%d",
		applied.KeysCreated, applied.ValuesSet, stats.FileSize, stats.UsedBytes)
}

// Test 42: Auto index mode selects single-pass for small plans.
func Test_Session_AutoIndexMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "auto-mode-test-hive")
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

	ctx := context.Background()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Small plan (< threshold) should use single-pass
	smallPlan := NewPlan()
	smallPlan.AddEnsureKey([]string{"_AutoTest"})
	smallPlan.AddSetValue([]string{"_AutoTest"}, "Val", format.REGDWORD, []byte{1, 0, 0, 0})

	opts := DefaultOptions()
	opts.IndexMode = IndexModeAuto
	opts.IndexThreshold = 10 // Threshold of 10 ops

	session, err := NewSessionForPlan(ctx, h, smallPlan, opts)
	if err != nil {
		t.Fatalf("NewSessionForPlan failed: %v", err)
	}
	defer session.Close(ctx)

	// Small plan should trigger single-pass mode
	if !session.IsSinglePassMode() {
		t.Error("Small plan should trigger single-pass mode in Auto mode")
	}

	t.Log("Auto mode correctly selected single-pass for small plan")
}

// Test 43: Explicit IndexMode overrides auto-selection.
func Test_Session_ExplicitIndexModeOverride(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	t.Run("ForceFull_SmallPlan", func(t *testing.T) {
		// Copy to temp directory
		tempHivePath := filepath.Join(t.TempDir(), "force-full-hive")
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

		ctx := context.Background()

		h, err := hive.Open(tempHivePath)
		if err != nil {
			t.Fatalf("Failed to open hive: %v", err)
		}
		defer h.Close()

		// Small plan that would normally use single-pass
		smallPlan := NewPlan()
		smallPlan.AddEnsureKey([]string{"_ForceFullTest"})

		// Force full index mode explicitly
		opts := DefaultOptions()
		opts.IndexMode = IndexModeFull

		session, err := NewSessionForPlan(ctx, h, smallPlan, opts)
		if err != nil {
			t.Fatalf("NewSessionForPlan failed: %v", err)
		}
		defer session.Close(ctx)

		// Should NOT be single-pass mode because we forced full
		if session.IsSinglePassMode() {
			t.Error("Session should NOT be in single-pass mode when IndexModeFull is explicit")
		}

		t.Log("IndexModeFull correctly overrides auto-selection for small plan")
	})

	t.Run("ForceSinglePass_LargePlan", func(t *testing.T) {
		// Copy to temp directory
		tempHivePath := filepath.Join(t.TempDir(), "force-single-hive")
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

		ctx := context.Background()

		h, err := hive.Open(tempHivePath)
		if err != nil {
			t.Fatalf("Failed to open hive: %v", err)
		}
		defer h.Close()

		// Large plan that would normally use full index
		largePlan := NewPlan()
		for i := 0; i < 200; i++ { // > default threshold of 100
			largePlan.AddEnsureKey([]string{"_ForceSingleTest", string(rune('A' + (i % 26)))})
		}

		// Force single-pass mode explicitly
		opts := DefaultOptions()
		opts.IndexMode = IndexModeSinglePass

		session, err := NewSessionForPlan(ctx, h, largePlan, opts)
		if err != nil {
			t.Fatalf("NewSessionForPlan failed: %v", err)
		}
		defer session.Close(ctx)

		// Should be single-pass mode because we forced it
		if !session.IsSinglePassMode() {
			t.Error("Session SHOULD be in single-pass mode when IndexModeSinglePass is explicit")
		}

		t.Log("IndexModeSinglePass correctly overrides auto-selection for large plan")
	})
}

// Test 44: NewSession respects IndexModeSinglePass.
func Test_NewSession_RespectsIndexMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	t.Run("SinglePassMode", func(t *testing.T) {
		// Copy to temp directory
		tempHivePath := filepath.Join(t.TempDir(), "newsession-single-pass-hive")
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

		ctx := context.Background()

		h, err := hive.Open(tempHivePath)
		if err != nil {
			t.Fatalf("Failed to open hive: %v", err)
		}
		defer h.Close()

		// Create session with explicit single-pass mode
		opts := DefaultOptions()
		opts.IndexMode = IndexModeSinglePass

		session, err := NewSession(ctx, h, opts)
		if err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
		defer session.Close(ctx)

		// Session should be in single-pass mode
		if !session.IsSinglePassMode() {
			t.Error("NewSession with IndexModeSinglePass should create single-pass session")
		}

		// ApplyWithTx should work in single-pass mode
		plan := NewPlan()
		plan.AddEnsureKey([]string{"_NewSessionSinglePassTest"})
		plan.AddSetValue([]string{"_NewSessionSinglePassTest"}, "Val", format.REGDWORD, []byte{1, 0, 0, 0})

		applied, err := session.ApplyWithTx(ctx, plan)
		if err != nil {
			t.Fatalf("ApplyWithTx failed: %v", err)
		}

		if applied.KeysCreated != 1 {
			t.Errorf("Expected 1 key created, got %d", applied.KeysCreated)
		}
		if applied.ValuesSet != 1 {
			t.Errorf("Expected 1 value set, got %d", applied.ValuesSet)
		}

		t.Log("NewSession respects IndexModeSinglePass")
	})

	t.Run("FullMode", func(t *testing.T) {
		// Copy to temp directory
		tempHivePath := filepath.Join(t.TempDir(), "newsession-full-hive")
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

		ctx := context.Background()

		h, err := hive.Open(tempHivePath)
		if err != nil {
			t.Fatalf("Failed to open hive: %v", err)
		}
		defer h.Close()

		// Create session with explicit full mode
		opts := DefaultOptions()
		opts.IndexMode = IndexModeFull

		session, err := NewSession(ctx, h, opts)
		if err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
		defer session.Close(ctx)

		// Session should NOT be in single-pass mode
		if session.IsSinglePassMode() {
			t.Error("NewSession with IndexModeFull should NOT create single-pass session")
		}

		t.Log("NewSession respects IndexModeFull")
	})
}

// Test 45: Session.ApplyRegText respects IndexMode.
func Test_Session_ApplyRegText_RespectsIndexMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "applyregtext-single-pass-hive")
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

	ctx := context.Background()

	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session with single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session, err := NewSession(ctx, h, opts)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close(ctx)

	// ApplyRegText should work in single-pass mode
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\_ApplyRegTextSinglePass]
"TestValue"="works"
`

	applied, err := session.ApplyRegText(ctx, regText)
	if err != nil {
		t.Fatalf("ApplyRegText failed: %v", err)
	}

	if applied.KeysCreated != 1 {
		t.Errorf("Expected 1 key created, got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", applied.ValuesSet)
	}

	t.Log("Session.ApplyRegText works with IndexModeSinglePass")
}

// Test 46: HasKey and HasKeys work in single-pass mode.
func Test_Session_HasKey_SinglePassMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "haskey-single-pass-hive")
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

	ctx := context.Background()

	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session with explicit single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session, err := NewSession(ctx, h, opts)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close(ctx)

	// Verify single-pass mode is active (idx is nil)
	if !session.IsSinglePassMode() {
		t.Fatal("Session should be in single-pass mode")
	}

	// First, create a test key using single-pass apply
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_HasKeySinglePassTest", "SubKey"})
	plan.AddSetValue([]string{"_HasKeySinglePassTest", "SubKey"}, "Val", format.REGDWORD, []byte{1, 0, 0, 0})

	applied, err := session.ApplyWithTx(ctx, plan)
	if err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}
	t.Logf("Applied: %d keys created, %d values set", applied.KeysCreated, applied.ValuesSet)

	// Test HasKey in single-pass mode (uses tree walking)
	if !session.HasKey("_HasKeySinglePassTest") {
		t.Error("HasKey should find _HasKeySinglePassTest in single-pass mode")
	}
	if !session.HasKey("_HasKeySinglePassTest\\SubKey") {
		t.Error("HasKey should find _HasKeySinglePassTest\\SubKey in single-pass mode")
	}
	if session.HasKey("_HasKeySinglePassTest\\NonExistent") {
		t.Error("HasKey should NOT find _HasKeySinglePassTest\\NonExistent")
	}

	// Test HasKeys in single-pass mode
	result, err := session.HasKeys(ctx,
		"_HasKeySinglePassTest",
		"_HasKeySinglePassTest\\SubKey",
		"_HasKeySinglePassTest\\NonExistent",
	)
	if err != nil {
		t.Fatalf("HasKeys failed: %v", err)
	}

	if len(result.Present) != 2 {
		t.Errorf("Expected 2 present keys, got %d: %v", len(result.Present), result.Present)
	}
	if len(result.Missing) != 1 {
		t.Errorf("Expected 1 missing key, got %d: %v", len(result.Missing), result.Missing)
	}
	if result.AllPresent {
		t.Error("AllPresent should be false")
	}

	t.Log("HasKey and HasKeys work correctly in single-pass mode")
}

// Test 47: All session methods work without panic in single-pass mode.
func Test_Session_SinglePassMode_NoPanics(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "single-pass-no-panic-hive")
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

	ctx := context.Background()

	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session with explicit single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session, err := NewSession(ctx, h, opts)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close(ctx)

	// Verify single-pass mode
	if !session.IsSinglePassMode() {
		t.Fatal("Session should be in single-pass mode")
	}

	// Test Index() - should return nil without panic
	idx := session.Index()
	if idx != nil {
		t.Error("Index() should return nil in single-pass mode")
	}

	// Test HasKey() - should work without panic
	exists := session.HasKey("ControlSet001")
	t.Logf("HasKey('ControlSet001'): %v", exists)

	// Test HasKeys() - should work without panic
	result, err := session.HasKeys(ctx, "ControlSet001", "NonExistent")
	if err != nil {
		t.Errorf("HasKeys failed: %v", err)
	}
	t.Logf("HasKeys result: Present=%v, Missing=%v", result.Present, result.Missing)

	// Test EnableDeferredMode() - should be no-op without panic
	session.EnableDeferredMode()

	// Test DisableDeferredMode() - should return nil without panic
	if err := session.DisableDeferredMode(); err != nil {
		t.Errorf("DisableDeferredMode failed: %v", err)
	}

	// Test FlushDeferredSubkeys() - should return (0, nil) without panic
	count, err := session.FlushDeferredSubkeys()
	if err != nil {
		t.Errorf("FlushDeferredSubkeys failed: %v", err)
	}
	if count != 0 {
		t.Errorf("FlushDeferredSubkeys should return 0 in single-pass mode, got %d", count)
	}

	// Test GetStorageStats() - should work without panic
	stats := session.GetStorageStats()
	if stats.FileSize == 0 {
		t.Error("GetStorageStats().FileSize should be non-zero")
	}
	t.Logf("StorageStats: FileSize=%d, UsedBytes=%d", stats.FileSize, stats.UsedBytes)

	// Test GetEfficiencyStats() - should work without panic
	effStats := session.GetEfficiencyStats()
	t.Logf("EfficiencyStats: TotalCapacity=%d", effStats.TotalCapacity)

	// Test GetHiveStats() - should work without panic
	hiveStats := session.GetHiveStats()
	t.Logf("HiveStats: FileSize=%d", hiveStats.Storage.FileSize)

	// Test Apply() - should return error (not panic) in single-pass mode
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_TestKey"})
	_, err = session.Apply(ctx, plan)
	if err == nil {
		t.Error("Apply() should return error in single-pass mode")
	} else {
		t.Logf("Apply() correctly returned error: %v", err)
	}

	// Test ApplyWithTx() - should work (uses ApplyPlanDirect internally)
	plan2 := NewPlan()
	plan2.AddEnsureKey([]string{"_NoPanicTest"})
	plan2.AddSetValue([]string{"_NoPanicTest"}, "Val", format.REGDWORD, []byte{1, 0, 0, 0})
	applied, err := session.ApplyWithTx(ctx, plan2)
	if err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}
	t.Logf("ApplyWithTx: KeysCreated=%d, ValuesSet=%d", applied.KeysCreated, applied.ValuesSet)

	// Test ApplyPlanDirect() - should work without panic
	plan3 := NewPlan()
	plan3.AddEnsureKey([]string{"_NoPanicTest2"})
	applied2, err := session.ApplyPlanDirect(ctx, plan3)
	if err != nil {
		t.Fatalf("ApplyPlanDirect failed: %v", err)
	}
	t.Logf("ApplyPlanDirect: KeysCreated=%d", applied2.KeysCreated)

	// Test WalkKeys() - should work without panic
	keyCount := 0
	err = session.WalkKeys(ctx, func(info KeyInfo) error {
		keyCount++
		if keyCount > 10 {
			return walker.ErrStopWalk
		}
		return nil
	})
	if err != nil {
		t.Errorf("WalkKeys failed: %v", err)
	}
	t.Logf("WalkKeys: counted %d keys (stopped at 10)", keyCount)

	t.Log("All session methods work without panic in single-pass mode")
}

// Test 48: MergeRegText uses single-pass mode for small regtext.
func Test_MergeRegText_UsesSinglePassMode(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "regtext-single-pass-hive")
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

	ctx := context.Background()

	// Small regtext (should trigger single-pass mode with default IndexModeAuto)
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\_RegTextSinglePass]
"TestValue"="hello"
"DwordValue"=dword:00000042
`

	// Use default options (IndexModeAuto with threshold 100)
	applied, err := MergeRegText(ctx, tempHivePath, regText, nil)
	if err != nil {
		t.Fatalf("MergeRegText failed: %v", err)
	}

	// Verify operations succeeded
	if applied.KeysCreated != 1 {
		t.Errorf("Expected 1 key created, got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 2 {
		t.Errorf("Expected 2 values set, got %d", applied.ValuesSet)
	}

	// Verify by reopening and checking the key exists
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Build index to verify key exists
	sess, err := NewSession(ctx, h, DefaultOptions())
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sess.Close(ctx)

	if !sess.HasKey("_RegTextSinglePass") {
		t.Error("Key _RegTextSinglePass should exist after MergeRegText")
	}

	t.Log("MergeRegText successfully applied regtext (single-pass mode for small plans)")
}

// Test 49: Verify sibling values are not corrupted in single-pass mode.
// This test specifically guards against the Go slice append bug where
// append(currentPath, childName) can reuse the underlying array,
// causing path corruption between sibling nodes during tree walking.
func Test_SinglePassMode_SiblingValueCorrectness(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "sibling-correctness-hive")
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

	ctx := context.Background()

	// Create multiple sibling keys with distinct DWORD values.
	// If slice corruption occurs, values will be written to wrong keys.
	// Using explicit hex values that are easy to verify.
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\_SiblingTest\Alpha]
"Value"=dword:00000001

[HKEY_LOCAL_MACHINE\_SiblingTest\Beta]
"Value"=dword:00000002

[HKEY_LOCAL_MACHINE\_SiblingTest\Gamma]
"Value"=dword:00000003

[HKEY_LOCAL_MACHINE\_SiblingTest\Delta]
"Value"=dword:00000004

[HKEY_LOCAL_MACHINE\_SiblingTest\Epsilon]
"Value"=dword:00000005
`

	// Force single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	applied, err := MergeRegText(ctx, tempHivePath, regText, &opts)
	if err != nil {
		t.Fatalf("MergeRegText failed: %v", err)
	}

	// Verify 5 sibling keys created (plus parent), 5 values set
	if applied.KeysCreated != 6 {
		t.Errorf("Expected 6 keys created (parent + 5 siblings), got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 5 {
		t.Errorf("Expected 5 values set, got %d", applied.ValuesSet)
	}

	// Re-open and verify each sibling has the correct value
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Build full index to read values (we're verifying correctness, not testing single-pass)
	fullOpts := DefaultOptions()
	fullOpts.IndexMode = IndexModeFull
	sess, err := NewSession(ctx, h, fullOpts)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sess.Close(ctx)

	// Expected values for each sibling
	expectedValues := map[string]uint32{
		"_SiblingTest\\Alpha":   1,
		"_SiblingTest\\Beta":    2,
		"_SiblingTest\\Gamma":   3,
		"_SiblingTest\\Delta":   4,
		"_SiblingTest\\Epsilon": 5,
	}

	rootOffset := h.RootCellOffset()
	errCount := 0
	for keyPath, expectedDword := range expectedValues {
		// Get the key reference
		keyParts := strings.Split(keyPath, "\\")
		nkRef, exists := index.WalkPath(sess.idx, rootOffset, keyParts...)
		if !exists {
			t.Errorf("Key %s should exist but doesn't", keyPath)
			errCount++
			continue
		}

		// Read the value - GetVK returns (vkOffset, ok)
		vkOff, found := sess.idx.GetVK(nkRef, "value") // lowercase for case-insensitive lookup
		if !found {
			t.Errorf("Value 'Value' not found at key %s", keyPath)
			errCount++
			continue
		}

		// Parse the VK cell to get the DWORD value
		vkPayload, resolveErr := h.ResolveCellPayload(vkOff)
		if resolveErr != nil {
			t.Errorf("Failed to resolve VK payload for %s: %v", keyPath, resolveErr)
			errCount++
			continue
		}

		vk, parseErr := hive.ParseVK(vkPayload)
		if parseErr != nil {
			t.Errorf("Failed to parse VK for %s: %v", keyPath, parseErr)
			errCount++
			continue
		}

		data, dataErr := vk.Data(h.Bytes())
		if dataErr != nil {
			t.Errorf("Failed to read value data for %s: %v", keyPath, dataErr)
			errCount++
			continue
		}

		if len(data) < 4 {
			t.Errorf("Value data too short for %s: got %d bytes", keyPath, len(data))
			errCount++
			continue
		}

		actualDword := binary.LittleEndian.Uint32(data[:4])
		if actualDword != expectedDword {
			t.Errorf("CRITICAL: Value mismatch at %s: expected %d, got %d (sibling corruption!)",
				keyPath, expectedDword, actualDword)
			errCount++
		}
	}

	if errCount == 0 {
		t.Log("All sibling values are correct - no slice corruption detected")
	} else {
		t.Fatalf("Detected %d value corruption errors - slice append bug may be present", errCount)
	}
}

// Test 50: Verify nested sibling paths don't get corrupted across multiple levels.
// This tests deeper nesting to catch potential corruption in multi-level recursion.
func Test_SinglePassMode_NestedSiblingCorrectness(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "nested-sibling-hive")
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

	ctx := context.Background()

	// Create a tree with multiple levels, each with siblings.
	// The path depth increases risk of slice capacity reuse.
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubA]
"ID"=dword:0000000A

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubB]
"ID"=dword:0000000B

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubC]
"ID"=dword:0000000C

[HKEY_LOCAL_MACHINE\_NestedTest\Level2\SubA]
"ID"=dword:0000001A

[HKEY_LOCAL_MACHINE\_NestedTest\Level2\SubB]
"ID"=dword:0000001B

[HKEY_LOCAL_MACHINE\_NestedTest\Level2\SubC]
"ID"=dword:0000001C

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubA\Deep1]
"ID"=dword:000000D1

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubA\Deep2]
"ID"=dword:000000D2

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubB\Deep1]
"ID"=dword:000000E1

[HKEY_LOCAL_MACHINE\_NestedTest\Level1\SubB\Deep2]
"ID"=dword:000000E2
`

	// Force single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	applied, err := MergeRegText(ctx, tempHivePath, regText, &opts)
	if err != nil {
		t.Fatalf("MergeRegText failed: %v", err)
	}

	t.Logf("Applied: %d keys created, %d values set", applied.KeysCreated, applied.ValuesSet)

	// Re-open and verify each key has the correct value
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	fullOpts := DefaultOptions()
	fullOpts.IndexMode = IndexModeFull
	sess, err := NewSession(ctx, h, fullOpts)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sess.Close(ctx)

	// Expected values for each path
	expectedValues := map[string]uint32{
		"_NestedTest\\Level1\\SubA":        0x0A,
		"_NestedTest\\Level1\\SubB":        0x0B,
		"_NestedTest\\Level1\\SubC":        0x0C,
		"_NestedTest\\Level2\\SubA":        0x1A,
		"_NestedTest\\Level2\\SubB":        0x1B,
		"_NestedTest\\Level2\\SubC":        0x1C,
		"_NestedTest\\Level1\\SubA\\Deep1": 0xD1,
		"_NestedTest\\Level1\\SubA\\Deep2": 0xD2,
		"_NestedTest\\Level1\\SubB\\Deep1": 0xE1,
		"_NestedTest\\Level1\\SubB\\Deep2": 0xE2,
	}

	rootOffset := h.RootCellOffset()
	errCount := 0
	for keyPath, expectedDword := range expectedValues {
		keyParts := strings.Split(keyPath, "\\")
		nkRef, exists := index.WalkPath(sess.idx, rootOffset, keyParts...)
		if !exists {
			t.Errorf("Key %s should exist but doesn't", keyPath)
			errCount++
			continue
		}

		// Read the value - GetVK returns (vkOffset, ok)
		vkOff, found := sess.idx.GetVK(nkRef, "id") // lowercase for case-insensitive lookup
		if !found {
			t.Errorf("Value 'ID' not found at key %s", keyPath)
			errCount++
			continue
		}

		// Parse the VK cell to get the DWORD value
		vkPayload, resolveErr := h.ResolveCellPayload(vkOff)
		if resolveErr != nil {
			t.Errorf("Failed to resolve VK payload for %s: %v", keyPath, resolveErr)
			errCount++
			continue
		}

		vk, parseErr := hive.ParseVK(vkPayload)
		if parseErr != nil {
			t.Errorf("Failed to parse VK for %s: %v", keyPath, parseErr)
			errCount++
			continue
		}

		data, dataErr := vk.Data(h.Bytes())
		if dataErr != nil {
			t.Errorf("Failed to read value data for %s: %v", keyPath, dataErr)
			errCount++
			continue
		}

		if len(data) < 4 {
			t.Errorf("Value data too short for %s: got %d bytes", keyPath, len(data))
			errCount++
			continue
		}

		actualDword := binary.LittleEndian.Uint32(data[:4])
		if actualDword != expectedDword {
			t.Errorf("CRITICAL: Value mismatch at %s: expected 0x%X, got 0x%X (path corruption!)",
				keyPath, expectedDword, actualDword)
			errCount++
		}
	}

	if errCount == 0 {
		t.Log("All nested sibling values are correct - no path corruption detected")
	} else {
		t.Fatalf("Detected %d value corruption errors in nested siblings", errCount)
	}
}

// Test 51: Verify that multiple SetValue operations for the same key/value result in the LAST value.
// This tests the value mismatch bug where Phase 2 re-applies ops that were already applied in Phase 1,
// causing the first delta's value to overwrite the second delta's value.
func Test_SinglePassMode_MultipleSetValueLastWins(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "setvalue-order-hive")
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

	ctx := context.Background()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create session with single-pass mode
	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session, err := NewSession(ctx, h, opts)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close(ctx)

	// Create a plan with MULTIPLE SetValue ops for the same value.
	// This simulates applying two deltas where both touch the same value.
	// The LAST value (0x62961562) should win.
	plan := NewPlan()
	plan.AddEnsureKey([]string{"_ValueOrderTest"})

	// First SetValue - value from "first delta"
	firstValue := []byte{0x95, 0xf8, 0x95, 0x62} // 0x6295f895 - the WRONG value
	plan.AddSetValue([]string{"_ValueOrderTest"}, "TestDword", format.REGDWORD, firstValue)

	// Second SetValue - value from "second delta" (should win)
	secondValue := []byte{0x62, 0x15, 0x96, 0x62} // 0x62961562 - the CORRECT value
	plan.AddSetValue([]string{"_ValueOrderTest"}, "TestDword", format.REGDWORD, secondValue)

	// Also test REG_SZ with different string values
	firstString := []byte("1653919924\x00")     // First delta string (WRONG)
	secondString := []byte("1653997021\x00")    // Second delta string (CORRECT - should win)
	plan.AddSetValue([]string{"_ValueOrderTest"}, "TestString", format.REGSZ, firstString)
	plan.AddSetValue([]string{"_ValueOrderTest"}, "TestString", format.REGSZ, secondString)

	// Apply with transaction
	result, err := session.ApplyWithTx(ctx, plan)
	if err != nil {
		t.Fatalf("ApplyWithTx failed: %v", err)
	}

	t.Logf("Applied: %d keys created, %d values set", result.KeysCreated, result.ValuesSet)

	// Re-open hive to verify the actual persisted values
	h.Close()

	h2, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h2.Close()

	// Build full index to read values
	fullOpts := DefaultOptions()
	fullOpts.IndexMode = IndexModeFull
	readSession, err := NewSession(ctx, h2, fullOpts)
	if err != nil {
		t.Fatalf("Failed to create read session: %v", err)
	}
	defer readSession.Close(ctx)

	// Find the key
	rootOffset := h2.RootCellOffset()
	nkRef, exists := index.WalkPath(readSession.idx, rootOffset, "_ValueOrderTest")
	if !exists {
		t.Fatal("_ValueOrderTest key should exist")
	}

	// Test DWORD value - should be the SECOND value (0x62961562)
	vkOff, found := readSession.idx.GetVK(nkRef, "testdword")
	if !found {
		t.Fatal("TestDword value should exist")
	}

	vkPayload, err := h2.ResolveCellPayload(vkOff)
	if err != nil {
		t.Fatalf("Failed to resolve VK payload: %v", err)
	}

	vk, err := hive.ParseVK(vkPayload)
	if err != nil {
		t.Fatalf("Failed to parse VK: %v", err)
	}

	data, err := vk.Data(h2.Bytes())
	if err != nil {
		t.Fatalf("Failed to read value data: %v", err)
	}

	if len(data) < 4 {
		t.Fatalf("DWORD data too short: got %d bytes", len(data))
	}

	actualDword := binary.LittleEndian.Uint32(data[:4])
	expectedDword := uint32(0x62961562) // The SECOND (correct) value

	if actualDword != expectedDword {
		t.Errorf("DWORD value mismatch: expected dword:0x%08x, got dword:0x%08x (first delta's value overwrote second!)",
			expectedDword, actualDword)
	} else {
		t.Logf("DWORD value correct: 0x%08x", actualDword)
	}

	// Test REG_SZ value - should be the SECOND string
	vkOffStr, found := readSession.idx.GetVK(nkRef, "teststring")
	if !found {
		t.Fatal("TestString value should exist")
	}

	vkPayloadStr, err := h2.ResolveCellPayload(vkOffStr)
	if err != nil {
		t.Fatalf("Failed to resolve VK payload for string: %v", err)
	}

	vkStr, err := hive.ParseVK(vkPayloadStr)
	if err != nil {
		t.Fatalf("Failed to parse VK for string: %v", err)
	}

	dataStr, err := vkStr.Data(h2.Bytes())
	if err != nil {
		t.Fatalf("Failed to read string value data: %v", err)
	}

	// Convert to string (remove null terminator if present)
	actualStr := string(bytes.TrimRight(dataStr, "\x00"))
	expectedStr := "1653997021" // The SECOND (correct) string

	if actualStr != expectedStr {
		t.Errorf("STRING value mismatch: expected \"%s\", got \"%s\" (first delta's value overwrote second!)",
			expectedStr, actualStr)
	} else {
		t.Logf("STRING value correct: %q", actualStr)
	}
}

// Test 52: Verify sequential plan applications where second plan updates existing value.
// This tests the scenario where a value is set in one plan, then updated in a second plan.
func Test_SinglePassMode_SequentialPlanUpdatesValue(t *testing.T) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "sequential-plans-hive")
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

	ctx := context.Background()

	// ===== First Plan: Create key and set initial value =====
	h1, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive for first plan: %v", err)
	}

	opts := DefaultOptions()
	opts.IndexMode = IndexModeSinglePass

	session1, err := NewSession(ctx, h1, opts)
	if err != nil {
		h1.Close()
		t.Fatalf("NewSession failed for first plan: %v", err)
	}

	plan1 := NewPlan()
	plan1.AddEnsureKey([]string{"_SequentialTest"})
	plan1.AddSetValue([]string{"_SequentialTest"}, "Counter", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00}) // 1

	result1, err := session1.ApplyWithTx(ctx, plan1)
	if err != nil {
		session1.Close(ctx)
		h1.Close()
		t.Fatalf("First ApplyWithTx failed: %v", err)
	}
	t.Logf("First plan: %d keys created, %d values set", result1.KeysCreated, result1.ValuesSet)

	session1.Close(ctx)
	h1.Close()

	// ===== Second Plan: Update existing value =====
	h2, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive for second plan: %v", err)
	}

	session2, err := NewSession(ctx, h2, opts)
	if err != nil {
		h2.Close()
		t.Fatalf("NewSession failed for second plan: %v", err)
	}

	plan2 := NewPlan()
	plan2.AddSetValue([]string{"_SequentialTest"}, "Counter", format.REGDWORD, []byte{0x02, 0x00, 0x00, 0x00}) // 2

	result2, err := session2.ApplyWithTx(ctx, plan2)
	if err != nil {
		session2.Close(ctx)
		h2.Close()
		t.Fatalf("Second ApplyWithTx failed: %v", err)
	}
	t.Logf("Second plan: %d keys created, %d values set", result2.KeysCreated, result2.ValuesSet)

	session2.Close(ctx)
	h2.Close()

	// ===== Verify: Value should be 2 (from second plan) =====
	h3, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive for verification: %v", err)
	}
	defer h3.Close()

	fullOpts := DefaultOptions()
	fullOpts.IndexMode = IndexModeFull
	readSession, err := NewSession(ctx, h3, fullOpts)
	if err != nil {
		t.Fatalf("Failed to create read session: %v", err)
	}
	defer readSession.Close(ctx)

	rootOffset := h3.RootCellOffset()
	nkRef, exists := index.WalkPath(readSession.idx, rootOffset, "_SequentialTest")
	if !exists {
		t.Fatal("_SequentialTest key should exist")
	}

	vkOff, found := readSession.idx.GetVK(nkRef, "counter")
	if !found {
		t.Fatal("Counter value should exist")
	}

	vkPayload, err := h3.ResolveCellPayload(vkOff)
	if err != nil {
		t.Fatalf("Failed to resolve VK payload: %v", err)
	}

	vk, err := hive.ParseVK(vkPayload)
	if err != nil {
		t.Fatalf("Failed to parse VK: %v", err)
	}

	data, err := vk.Data(h3.Bytes())
	if err != nil {
		t.Fatalf("Failed to read value data: %v", err)
	}

	actualValue := binary.LittleEndian.Uint32(data[:4])
	expectedValue := uint32(2) // Should be 2 from second plan

	if actualValue != expectedValue {
		t.Errorf("Sequential plan value mismatch: expected %d, got %d", expectedValue, actualValue)
	} else {
		t.Logf("Sequential plan value correct: %d", actualValue)
	}
}
