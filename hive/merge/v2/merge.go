package v2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/merge/v2/flush"
	"github.com/joshuapare/hivekit/hive/merge/v2/plan"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/merge/v2/walk"
	"github.com/joshuapare/hivekit/hive/merge/v2/write"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Merge applies a set of merge operations to the hive using the v2 phase-separated pipeline.
//
// The pipeline consists of five phases:
//  1. Parse: Build a PatchTrie from the operations
//  2. Walk: Annotate the trie with existing hive cell references
//  3. Plan: Estimate space requirements for new cells
//  4. Write: Allocate and write new cells via bump allocator
//  5. Flush: Apply in-place updates and finalize the base block header
//
// Each phase checks ctx.Err() for cancellation between phases.
func Merge(ctx context.Context, h *hive.Hive, ops []merge.Op, _ Options) (Result, error) {
	var result Result
	totalStart := time.Now()

	if len(ops) == 0 {
		result.PhaseTiming.Total = time.Since(totalStart)
		return result, nil
	}

	// Initialize dirty tracker and allocator.
	dt := dirty.NewTracker(h)
	fa, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		return Result{}, fmt.Errorf("v2: create allocator: %w", err)
	}

	// ── Phase 1: Parse ──────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	phaseStart := time.Now()

	root := trie.Build(ops)

	result.PhaseTiming.Parse = time.Since(phaseStart)

	// ── Phase 2: Walk ───────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	phaseStart = time.Now()

	if err := walk.Annotate(h, root); err != nil {
		return Result{}, fmt.Errorf("v2: walk phase: %w", err)
	}

	result.PhaseTiming.Walk = time.Since(phaseStart)

	// ── Phase 3: Plan ───────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	phaseStart = time.Now()

	spacePlan, err := plan.Estimate(root)
	if err != nil {
		return Result{}, fmt.Errorf("v2: plan phase: %w", err)
	}

	if spacePlan.TotalNewBytes > 0 {
		if err := fa.EnableBumpMode(spacePlan.TotalNewBytes); err != nil {
			return Result{}, fmt.Errorf("v2: enable bump mode: %w", err)
		}
		defer func() {
			// FinalizeBumpMode is also called in flush.Apply, but the defer
			// ensures cleanup if we return early due to an error.
			_ = fa.FinalizeBumpMode()
		}()
	}

	result.BytesAllocated = int64(spacePlan.TotalNewBytes)
	result.PhaseTiming.Plan = time.Since(phaseStart)

	// ── Phase 4: Write ──────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	phaseStart = time.Now()

	hiveSizeBefore := h.Size()

	updates, stats, err := write.Execute(h, root, spacePlan, fa)
	if err != nil {
		return Result{}, fmt.Errorf("v2: write phase: %w", err)
	}

	result.KeysCreated = stats.KeysCreated
	result.KeysDeleted = stats.KeysDeleted
	result.ValuesSet = stats.ValuesSet
	result.ValuesDeleted = stats.ValuesDeleted
	result.PhaseTiming.Write = time.Since(phaseStart)

	// ── Phase 5: Flush ──────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	phaseStart = time.Now()

	if err := flush.Apply(h, updates, fa); err != nil {
		return Result{}, fmt.Errorf("v2: flush phase: %w", err)
	}

	result.HiveGrowth = h.Size() - hiveSizeBefore
	result.PhaseTiming.Flush = time.Since(phaseStart)

	result.PhaseTiming.Total = time.Since(totalStart)
	return result, nil
}

// MergeRegText parses .reg file text and applies it to the hive using the v2 pipeline.
//
// This is a convenience wrapper that parses the regtext into merge operations
// and then calls Merge.
func MergeRegText(ctx context.Context, h *hive.Hive, regText string, opts Options) (Result, error) {
	editOps, err := regtext.ParseReg([]byte(regText), types.RegParseOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("v2: parse regtext: %w", err)
	}

	ops := convertEditOps(editOps)
	return Merge(ctx, h, ops, opts)
}

// convertEditOps converts types.EditOp values to merge.Op values.
func convertEditOps(editOps []types.EditOp) []merge.Op {
	ops := make([]merge.Op, 0, len(editOps))
	for _, editOp := range editOps {
		switch op := editOp.(type) {
		case types.OpCreateKey:
			ops = append(ops, merge.Op{
				Type:    merge.OpEnsureKey,
				KeyPath: stripHiveRootAndSplit(op.Path),
			})
		case types.OpSetValue:
			ops = append(ops, merge.Op{
				Type:      merge.OpSetValue,
				KeyPath:   stripHiveRootAndSplit(op.Path),
				ValueName: op.Name,
				ValueType: uint32(op.Type),
				Data:      op.Data,
			})
		case types.OpDeleteValue:
			ops = append(ops, merge.Op{
				Type:      merge.OpDeleteValue,
				KeyPath:   stripHiveRootAndSplit(op.Path),
				ValueName: op.Name,
			})
		case types.OpDeleteKey:
			ops = append(ops, merge.Op{
				Type:    merge.OpDeleteKey,
				KeyPath: stripHiveRootAndSplit(op.Path),
			})
		}
	}
	return ops
}

// stripHiveRootAndSplit removes common hive root prefixes and splits the path
// into components. This mirrors the logic in hive/merge/api.go.
func stripHiveRootAndSplit(path string) []string {
	prefixes := []string{
		"HKEY_LOCAL_MACHINE\\",
		"HKLM\\",
		"HKEY_CURRENT_USER\\",
		"HKCU\\",
		"HKEY_USERS\\",
		"HKU\\",
		"HKEY_CLASSES_ROOT\\",
		"HKCR\\",
		"HKEY_CURRENT_CONFIG\\",
		"HKCC\\",
	}

	for _, prefix := range prefixes {
		if len(path) >= len(prefix) && strings.EqualFold(path[:len(prefix)], prefix) {
			path = path[len(prefix):]
			break
		}
	}

	segments := strings.Split(path, "\\")
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}
	return result
}
