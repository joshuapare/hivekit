package merge

import (
	"context"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestWalkApply_DeleteKeyThenWalkChildren verifies that walkAndApply does not
// attempt to walk children of a key that was just deleted by an OpDeleteKey.
//
// Bug: After applyOpAtNode processes OpDeleteKey (which recursively deletes the
// key and all its children), walkAndApply continues to lines 174-203 which read
// nk.SubkeyCount() and nk.SubkeyListOffsetRel() from the now-freed NK cell.
// This reads stale/freed memory (SIGSEGV on mmap, garbage on non-mmap).
// Even when it "works", walking children of a deleted key is logically wrong.
//
// Fix: After the ops loop, check if any op was OpDeleteKey. If so, skip walking
// children. Also re-resolve the NK from the hive to get fresh data.
func TestWalkApply_DeleteKeyThenWalkChildren(t *testing.T) {
	_, session, _, cleanup := setupTestHive(t)
	defer cleanup()

	// Phase 1: Create a parent key with a child key and values
	setup := NewPlan()
	setup.AddEnsureKey([]string{"_WalkApplyBug", "Parent"})
	setup.AddEnsureKey([]string{"_WalkApplyBug", "Parent", "Child"})
	setup.AddSetValue([]string{"_WalkApplyBug", "Parent", "Child"}, "Val1", format.REGSZ, []byte("hello\x00"))

	_, err := session.ApplyWithTx(context.Background(), setup)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Phase 2: Apply a plan that deletes the parent key.
	// The walk-apply engine visits _WalkApplyBug → Parent → applies DeleteKey.
	// DeleteKey(recursive=true) deletes Parent AND Child.
	// Bug: after DeleteKey returns, walkAndApply tries to walk Parent's children
	//       (looking for "child" in neededChildren), reading freed memory.
	//
	// To trigger the walk-apply path, use ApplyPlanDirect (single-pass mode).
	deletePlan := NewPlan()
	deletePlan.AddDeleteKey([]string{"_WalkApplyBug", "Parent"})
	// Also add an op targeting the child — this ensures the child is in
	// childrenByParent, so walkAndApply WOULD try to descend into it.
	deletePlan.AddSetValue([]string{"_WalkApplyBug", "Parent", "Child"}, "Val2", format.REGSZ, []byte("world\x00"))

	result, err := session.ApplyPlanDirect(context.Background(), deletePlan)
	if err != nil {
		t.Fatalf("ApplyPlanDirect failed (stale walk after delete?): %v", err)
	}

	t.Logf("Result: KeysDeleted=%d, ValuesSet=%d", result.KeysDeleted, result.ValuesSet)

	// The key should be deleted
	if result.KeysDeleted != 1 {
		t.Errorf("Expected 1 key deleted, got %d", result.KeysDeleted)
	}
}
