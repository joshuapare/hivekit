package benchmark

import (
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive/merge"
)

func TestGenerateCreateSparse(t *testing.T) {
	const count = 100
	ops := GenerateCreateSparse(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// All ops must be OpEnsureKey
	for i, op := range ops {
		if op.Type != merge.OpEnsureKey {
			t.Errorf("op[%d]: expected OpEnsureKey, got %s", i, op.Type)
		}
		if len(op.KeyPath) == 0 {
			t.Errorf("op[%d]: KeyPath must not be empty", i)
		}
	}

	// Verify paths are scattered: count unique top-level prefixes.
	// Sparse generation should produce many distinct top-level keys.
	topLevels := make(map[string]bool)
	for _, op := range ops {
		topLevels[op.KeyPath[0]] = true
	}
	if len(topLevels) < 10 {
		t.Errorf("sparse ops should have many distinct top-level keys, got %d", len(topLevels))
	}

	// Verify determinism: same seed produces same ops
	ops2 := GenerateCreateSparse(count, 42)
	for i := range ops {
		if ops[i].KeyPath[0] != ops2[i].KeyPath[0] {
			t.Fatalf("determinism failure at op[%d]: %v != %v", i, ops[i].KeyPath, ops2[i].KeyPath)
		}
	}
}

func TestGenerateCreateDense(t *testing.T) {
	const count = 100
	ops := GenerateCreateDense(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// All ops must be OpEnsureKey
	for i, op := range ops {
		if op.Type != merge.OpEnsureKey {
			t.Errorf("op[%d]: expected OpEnsureKey, got %s", i, op.Type)
		}
		if len(op.KeyPath) == 0 {
			t.Errorf("op[%d]: KeyPath must not be empty", i)
		}
	}

	// Verify paths share prefixes: few unique top-level keys.
	// Dense generation concentrates keys under a small number of parents.
	topLevels := make(map[string]bool)
	for _, op := range ops {
		topLevels[op.KeyPath[0]] = true
	}
	if len(topLevels) > 10 {
		t.Errorf("dense ops should have few distinct top-level keys, got %d", len(topLevels))
	}
}

func TestGenerateCreateDeep(t *testing.T) {
	const count = 50
	ops := GenerateCreateDeep(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpEnsureKey {
			t.Errorf("op[%d]: expected OpEnsureKey, got %s", i, op.Type)
		}
		// Deep paths should be 10-15 levels
		if len(op.KeyPath) < 10 {
			t.Errorf("op[%d]: expected depth >= 10, got %d", i, len(op.KeyPath))
		}
		if len(op.KeyPath) > 15 {
			t.Errorf("op[%d]: expected depth <= 15, got %d", i, len(op.KeyPath))
		}
	}
}

func TestGenerateUpdateExisting(t *testing.T) {
	const count = 50
	existingKeys := sampleExistingKeys(100)
	ops := GenerateUpdateExisting(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpSetValue {
			t.Errorf("op[%d]: expected OpSetValue, got %s", i, op.Type)
		}
		if len(op.Data) == 0 {
			t.Errorf("op[%d]: Data must not be empty", i)
		}
		if op.ValueType == 0 {
			t.Errorf("op[%d]: ValueType must be set", i)
		}
		// Path should come from existingKeys
		if !isKeyInSet(op.KeyPath, existingKeys) {
			t.Errorf("op[%d]: KeyPath %v not found in existingKeys", i, op.KeyPath)
		}
	}
}

func TestGenerateUpdateResize(t *testing.T) {
	const count = 50
	existingKeys := sampleExistingKeys(100)
	ops := GenerateUpdateResize(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpSetValue {
			t.Errorf("op[%d]: expected OpSetValue, got %s", i, op.Type)
		}
		if len(op.Data) == 0 {
			t.Errorf("op[%d]: Data must not be empty", i)
		}
		if !isKeyInSet(op.KeyPath, existingKeys) {
			t.Errorf("op[%d]: KeyPath %v not found in existingKeys", i, op.KeyPath)
		}
	}
}

func TestGenerateDeleteValues(t *testing.T) {
	const count = 50
	existingKeys := sampleExistingKeys(100)
	ops := GenerateDeleteValues(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpDeleteValue {
			t.Errorf("op[%d]: expected OpDeleteValue, got %s", i, op.Type)
		}
		if op.ValueName == "" {
			t.Errorf("op[%d]: ValueName must not be empty", i)
		}
		if !isKeyInSet(op.KeyPath, existingKeys) {
			t.Errorf("op[%d]: KeyPath %v not found in existingKeys", i, op.KeyPath)
		}
	}
}

func TestGenerateDeleteKeysLeaf(t *testing.T) {
	const count = 50
	leafKeys := sampleExistingKeys(100)
	ops := GenerateDeleteKeysLeaf(count, leafKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpDeleteKey {
			t.Errorf("op[%d]: expected OpDeleteKey, got %s", i, op.Type)
		}
		if !isKeyInSet(op.KeyPath, leafKeys) {
			t.Errorf("op[%d]: KeyPath %v not found in leafKeys", i, op.KeyPath)
		}
	}
}

func TestGenerateDeleteKeysSubtree(t *testing.T) {
	const count = 20
	ops := GenerateDeleteKeysSubtree(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpDeleteKey {
			t.Errorf("op[%d]: expected OpDeleteKey, got %s", i, op.Type)
		}
		if len(op.KeyPath) == 0 {
			t.Errorf("op[%d]: KeyPath must not be empty", i)
		}
	}
}

func TestGenerateDeleteHeavyMixed(t *testing.T) {
	const count = 100
	existingKeys := sampleExistingKeys(200)
	ops := GenerateDeleteHeavyMixed(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// Verify approximate distribution: 60% deletes, 40% creates
	var deletes, creates int
	for _, op := range ops {
		switch op.Type {
		case merge.OpDeleteKey, merge.OpDeleteValue:
			deletes++
		case merge.OpEnsureKey:
			creates++
		default:
			t.Errorf("unexpected op type: %s", op.Type)
		}
	}

	// Allow 15% tolerance on distribution
	if deletes < 45 || deletes > 75 {
		t.Errorf("expected ~60 deletes, got %d", deletes)
	}
	if creates < 25 || creates > 55 {
		t.Errorf("expected ~40 creates, got %d", creates)
	}
}

func TestGenerateMixedRealistic(t *testing.T) {
	const count = 200
	existingKeys := sampleExistingKeys(300)
	ops := GenerateMixedRealistic(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// Verify all 4 op types are present
	typeCounts := make(map[merge.OpType]int)
	for _, op := range ops {
		typeCounts[op.Type]++
	}

	expectedTypes := []merge.OpType{
		merge.OpEnsureKey,
		merge.OpSetValue,
		merge.OpDeleteValue,
		merge.OpDeleteKey,
	}
	for _, et := range expectedTypes {
		if typeCounts[et] == 0 {
			t.Errorf("expected at least one %s op, got 0", et)
		}
	}

	// Verify approximate distribution: 30% create, 40% update, 20% delete value, 10% delete key
	ensureCount := typeCounts[merge.OpEnsureKey]
	setCount := typeCounts[merge.OpSetValue]
	delValCount := typeCounts[merge.OpDeleteValue]
	delKeyCount := typeCounts[merge.OpDeleteKey]

	// Allow 15% tolerance
	if ensureCount < 30 || ensureCount > 90 {
		t.Errorf("expected ~60 EnsureKey ops (30%%), got %d", ensureCount)
	}
	if setCount < 50 || setCount > 110 {
		t.Errorf("expected ~80 SetValue ops (40%%), got %d", setCount)
	}
	if delValCount < 20 || delValCount > 60 {
		t.Errorf("expected ~40 DeleteValue ops (20%%), got %d", delValCount)
	}
	if delKeyCount < 5 || delKeyCount > 35 {
		t.Errorf("expected ~20 DeleteKey ops (10%%), got %d", delKeyCount)
	}

	// Verify well-formedness: SetValue ops must have ValueType and Data
	for i, op := range ops {
		if op.Type == merge.OpSetValue {
			if op.ValueType == 0 {
				t.Errorf("op[%d]: SetValue must have ValueType", i)
			}
			if len(op.Data) == 0 {
				t.Errorf("op[%d]: SetValue must have Data", i)
			}
		}
		if op.Type == merge.OpDeleteValue && op.ValueName == "" {
			t.Errorf("op[%d]: DeleteValue must have ValueName", i)
		}
	}
}

func TestGenerateIdempotentReplay(t *testing.T) {
	const count = 50
	existingKeys := sampleExistingKeys(100)
	ops := GenerateIdempotentReplay(count, existingKeys, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// All ops should be EnsureKey (idempotent re-creation of existing keys)
	for i, op := range ops {
		if op.Type != merge.OpEnsureKey {
			t.Errorf("op[%d]: expected OpEnsureKey for idempotent replay, got %s", i, op.Type)
		}
		if !isKeyInSet(op.KeyPath, existingKeys) {
			t.Errorf("op[%d]: KeyPath %v not found in existingKeys", i, op.KeyPath)
		}
	}
}

func TestGenerateLargeValues(t *testing.T) {
	const count = 20
	ops := GenerateLargeValues(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	for i, op := range ops {
		if op.Type != merge.OpSetValue {
			t.Errorf("op[%d]: expected OpSetValue, got %s", i, op.Type)
		}
		// Values should be 4KB-64KB
		if len(op.Data) < 4*1024 {
			t.Errorf("op[%d]: Data too small: %d bytes (min 4KB)", i, len(op.Data))
		}
		if len(op.Data) > 64*1024 {
			t.Errorf("op[%d]: Data too large: %d bytes (max 64KB)", i, len(op.Data))
		}
	}
}

func TestGenerateWorstCaseFragmented(t *testing.T) {
	const count = 100
	ops := GenerateWorstCaseFragmented(count, 42)

	if len(ops) != count {
		t.Fatalf("expected %d ops, got %d", count, len(ops))
	}

	// Should contain a mix of creates and deletes
	var creates, deletes int
	for _, op := range ops {
		switch op.Type {
		case merge.OpEnsureKey:
			creates++
		case merge.OpDeleteKey:
			deletes++
		default:
			t.Errorf("unexpected op type in fragmented workload: %s", op.Type)
		}
	}

	if creates == 0 {
		t.Error("expected at least some EnsureKey ops")
	}
	if deletes == 0 {
		t.Error("expected at least some DeleteKey ops")
	}
}

func TestCollectExistingKeys(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		count       int
		wantLen     int
		checkPrefix string
	}{
		{
			name:        "small-flat keys",
			fixture:     "small-flat",
			count:       50,
			wantLen:     50,
			checkPrefix: "Parent",
		},
		{
			name:        "small-deep keys",
			fixture:     "small-deep",
			count:       30,
			wantLen:     30,
			checkPrefix: "Root",
		},
		{
			name:        "medium-mixed keys",
			fixture:     "medium-mixed",
			count:       40,
			wantLen:     40,
			checkPrefix: "Software",
		},
		{
			name:        "large-wide keys",
			fixture:     "large-wide",
			count:       50,
			wantLen:     50,
			checkPrefix: "Wide",
		},
		{
			name:        "large-realistic keys",
			fixture:     "large-realistic",
			count:       50,
			wantLen:     50,
			checkPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := CollectExistingKeys(tt.fixture, tt.count, 42)

			if len(keys) != tt.wantLen {
				t.Fatalf("expected %d keys, got %d", tt.wantLen, len(keys))
			}

			for i, key := range keys {
				if len(key) == 0 {
					t.Errorf("key[%d]: must not be empty", i)
				}
				if tt.checkPrefix != "" && !strings.HasPrefix(key[0], tt.checkPrefix) {
					t.Errorf("key[%d]: expected top-level prefix %q, got %q", i, tt.checkPrefix, key[0])
				}
			}

			// Verify determinism
			keys2 := CollectExistingKeys(tt.fixture, tt.count, 42)
			for i := range keys {
				if keys[i][0] != keys2[i][0] {
					t.Fatalf("determinism failure at key[%d]", i)
				}
			}
		})
	}
}

// sampleExistingKeys generates a set of key paths for use in tests.
func sampleExistingKeys(count int) [][]string {
	keys := make([][]string, count)
	for i := range count {
		keys[i] = []string{"TestParent", "TestChild" + string(rune('A'+i%26))}
	}
	return keys
}

// isKeyInSet checks whether the given keyPath exists in the provided set.
func isKeyInSet(keyPath []string, set [][]string) bool {
	for _, k := range set {
		if len(k) != len(keyPath) {
			continue
		}
		match := true
		for j := range k {
			if k[j] != keyPath[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
