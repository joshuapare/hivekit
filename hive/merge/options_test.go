package merge

import (
	"testing"

	"github.com/joshuapare/hivekit/hive/dirty"
)

// Test 1: Default Options.
func Test_Options_Defaults(t *testing.T) {
	opt := DefaultOptions()

	// Verify Strategy
	if opt.Strategy != StrategyHybrid {
		t.Errorf("Default Strategy: got %v, want %v", opt.Strategy, StrategyHybrid)
	}

	// Verify GrowChunk (1MB)
	expectedGrow := 1 << 20 // 1048576
	if opt.GrowChunk != expectedGrow {
		t.Errorf("Default GrowChunk: got %d, want %d", opt.GrowChunk, expectedGrow)
	}

	// Verify StripeUnit (disabled)
	if opt.StripeUnit != 0 {
		t.Errorf("Default StripeUnit: got %d, want 0", opt.StripeUnit)
	}

	// Verify Flush mode
	if opt.Flush != dirty.FlushAuto {
		t.Errorf("Default Flush: got %v, want %v", opt.Flush, dirty.FlushAuto)
	}

	// Verify HugePages
	if opt.HugePages != false {
		t.Errorf("Default HugePages: got %v, want false", opt.HugePages)
	}

	// Verify WillNeedHint
	if opt.WillNeedHint != false {
		t.Errorf("Default WillNeedHint: got %v, want false", opt.WillNeedHint)
	}

	// Verify HybridSlackPct
	if opt.HybridSlackPct != 12 {
		t.Errorf("Default HybridSlackPct: got %d, want 12", opt.HybridSlackPct)
	}

	// Verify CompactThreshold
	if opt.CompactThreshold != 30 {
		t.Errorf("Default CompactThreshold: got %d, want 30", opt.CompactThreshold)
	}

	t.Log("All default values are correct")
}

// Test 2: Strategy Constants.
func Test_Options_StrategyConstants(t *testing.T) {
	// Verify StrategyInPlace = 0
	if StrategyInPlace != 0 {
		t.Errorf("StrategyInPlace: got %d, want 0", StrategyInPlace)
	}

	// Verify StrategyAppend = 1
	if StrategyAppend != 1 {
		t.Errorf("StrategyAppend: got %d, want 1", StrategyAppend)
	}

	// Verify StrategyHybrid = 2
	if StrategyHybrid != 2 {
		t.Errorf("StrategyHybrid: got %d, want 2", StrategyHybrid)
	}

	// Verify all constants are distinct
	strategies := []StrategyKind{StrategyInPlace, StrategyAppend, StrategyHybrid}
	seen := make(map[StrategyKind]bool)
	for _, s := range strategies {
		if seen[s] {
			t.Errorf("Duplicate strategy value: %d", s)
		}
		seen[s] = true
	}

	t.Logf("Strategy constants: InPlace=%d, Append=%d, Hybrid=%d",
		StrategyInPlace, StrategyAppend, StrategyHybrid)
}

// Test 3: Custom Options.
func Test_Options_CustomValues(t *testing.T) {
	opt := Options{
		Strategy:         StrategyInPlace,
		GrowChunk:        4 << 20,   // 4MB
		StripeUnit:       256 << 10, // 256KB
		Flush:            dirty.FlushFull,
		HugePages:        true,
		WillNeedHint:     true,
		HybridSlackPct:   20,
		CompactThreshold: 50,
	}

	// Verify all custom values
	if opt.Strategy != StrategyInPlace {
		t.Errorf("Custom Strategy: got %v, want %v", opt.Strategy, StrategyInPlace)
	}

	if opt.GrowChunk != (4 << 20) {
		t.Errorf("Custom GrowChunk: got %d, want %d", opt.GrowChunk, 4<<20)
	}

	if opt.StripeUnit != (256 << 10) {
		t.Errorf("Custom StripeUnit: got %d, want %d", opt.StripeUnit, 256<<10)
	}

	if opt.Flush != dirty.FlushFull {
		t.Errorf("Custom Flush: got %v, want %v", opt.Flush, dirty.FlushFull)
	}

	if !opt.HugePages {
		t.Error("Custom HugePages should be true")
	}

	if !opt.WillNeedHint {
		t.Error("Custom WillNeedHint should be true")
	}

	if opt.HybridSlackPct != 20 {
		t.Errorf("Custom HybridSlackPct: got %d, want 20", opt.HybridSlackPct)
	}

	if opt.CompactThreshold != 50 {
		t.Errorf("Custom CompactThreshold: got %d, want 50", opt.CompactThreshold)
	}

	t.Log("All custom values set correctly")
}
