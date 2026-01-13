package merge_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/walker"
)

// =============================================================================
// Context Cancellation Tests
//
// These tests verify that all public APIs respect context cancellation:
// 1. Pre-cancelled context: Should return ctx.Err() immediately
// 2. Mid-operation cancellation: Should exit promptly
// 3. Timeout: Should respect context.WithTimeout deadlines
// =============================================================================

// --- walker.IndexBuilder.Build() Tests ---

func TestIndexBuilder_Build_PreCancelled(t *testing.T) {
	// Create a hive with some data
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Build should return context.Canceled
	ib := walker.NewIndexBuilder(h, 1000, 1000)
	_, err = ib.Build(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestIndexBuilder_Build_Timeout(t *testing.T) {
	// Create a hive with some data
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Use a very short timeout (should still complete for small hive)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ib := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := ib.Build(ctx)

	// For a small hive, this should succeed
	require.NoError(t, err)
	require.NotNil(t, idx)
}

// --- merge.NewSession() Tests ---

func TestNewSession_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// NewSession builds an index internally, should fail with cancelled context
	_, err = merge.NewSession(ctx, h, merge.DefaultOptions())

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestNewSessionWithIndex_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Build index first with valid context
	ib := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := ib.Build(context.Background())
	require.NoError(t, err)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// NewSessionWithIndex should fail with cancelled context
	_, err = merge.NewSessionWithIndex(ctx, h, idx, merge.DefaultOptions())

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

// --- Session.Apply() Tests ---

func TestSession_Apply_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Create session with valid context
	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)
	defer sess.Close(context.Background())

	// Begin transaction
	err = sess.Begin(context.Background())
	require.NoError(t, err)

	// Create a plan with operations
	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})
	plan.AddSetValue([]string{"Software", "Test"}, "Value1", 1, []byte("data"))

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Apply should fail with cancelled context
	_, err = sess.Apply(ctx, plan)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)

	sess.Rollback()
}

func TestSession_ApplyWithTx_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)
	defer sess.Close(context.Background())

	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ApplyWithTx should fail with cancelled context
	_, err = sess.ApplyWithTx(ctx, plan)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

// --- Session.Begin() and Commit() Tests ---

func TestSession_Begin_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)
	defer sess.Close(context.Background())

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Begin should fail with cancelled context
	err = sess.Begin(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestSession_Commit_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)
	defer sess.Close(context.Background())

	// Begin with valid context
	err = sess.Begin(context.Background())
	require.NoError(t, err)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Commit should fail with cancelled context
	err = sess.Commit(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)

	sess.Rollback()
}

func TestSession_Close_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Close should fail with cancelled context
	err = sess.Close(ctx)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

// --- Public API Tests ---

func TestMergePlan_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// MergePlan should fail with cancelled context
	_, err := merge.MergePlan(ctx, hivePath, plan, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestMergeRegText_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	regText := `Windows Registry Editor Version 5.00

[Software\Test]
"Value"="data"
`

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// MergeRegText should fail with cancelled context
	_, err := merge.MergeRegText(ctx, hivePath, regText, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
}

func TestWithSession_PreCancelled(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callbackCalled := false

	// WithSession should fail with cancelled context before calling callback
	err := merge.WithSession(ctx, hivePath, nil, func(s *merge.Session) error {
		callbackCalled = true
		return nil
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
	require.False(t, callbackCalled, "callback should not have been called")
}

// --- Timeout Tests ---

func TestMergePlan_WithTimeout_Success(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})
	plan.AddSetValue([]string{"Software", "Test"}, "Value", 1, []byte("data\x00"))

	// Use a generous timeout - should succeed
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	applied, err := merge.MergePlan(ctx, hivePath, plan, nil)

	require.NoError(t, err)
	require.Equal(t, 1, applied.KeysCreated)
	require.Equal(t, 1, applied.ValuesSet)
}

func TestSession_Apply_ManyOperations_Cancellation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	sess, err := merge.NewSession(context.Background(), h, merge.DefaultOptions())
	require.NoError(t, err)
	defer sess.Close(context.Background())

	// Create a plan with many operations
	plan := merge.NewPlan()
	for i := 0; i < 100; i++ {
		plan.AddEnsureKey([]string{"Software", "Test", "Key" + string(rune('A'+i%26))})
	}

	// Create a context that we'll cancel mid-operation
	ctx, cancel := context.WithCancel(context.Background())

	// Start the operation in a goroutine
	done := make(chan error, 1)
	go func() {
		err := sess.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		_, err = sess.Apply(ctx, plan)
		done <- err
	}()

	// Cancel after a short delay to allow some operations to start
	time.Sleep(1 * time.Millisecond)
	cancel()

	// Wait for completion
	select {
	case err := <-done:
		// Either it completed successfully (fast) or was cancelled
		if err != nil {
			require.True(t, errors.Is(err, context.Canceled),
				"expected context.Canceled or nil, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not respect cancellation within 5 seconds")
	}

	sess.Rollback()
}

// --- DeadlineExceeded Tests ---

func TestMergePlan_DeadlineExceeded(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")
	createTestHive(t, hivePath)

	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})

	// Use an already-expired timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := merge.MergePlan(ctx, hivePath, plan, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded),
		"expected context.DeadlineExceeded, got: %v", err)
}

// --- Helper Functions ---

// createTestHive creates a minimal hive for testing
func createTestHive(t *testing.T, hivePath string) {
	t.Helper()

	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Add some initial data
	err = b.SetString([]string{"Software"}, "Version", "1.0")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
}
