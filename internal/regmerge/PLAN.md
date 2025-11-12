# regmerge Optimizer: Benchmark & Coverage Implementation Plan

**Status**: In Progress
**Start Date**: 2025-11-06
**Goal**: Achieve 90%+ coverage with comprehensive benchmarks proving optimizer correctness and performance

---

## Current State (Starting Point)

- **Test Coverage**: 74.0%
- **Uncovered Functions**:
  - `ParseAndOptimize`: 0.0%
  - `ParseAndOptimizeSingle`: 0.0%
  - `ParseFiles`: 0.0%
  - `applyR2LOptimization`: 73.8% (OpDeleteValue paths untested)
- **Benchmarks**: None yet
- **Correctness Verification**: None

---

## Phase 1: Test Coverage Improvements

**Goal**: Increase coverage from 74% to 90%+
**Time Estimate**: 30 minutes
**Status**: ⏳ Pending

### File: `multifile_test.go` (NEW)

Tests to add:

1. ✅ **TestParseAndOptimize_SingleFile**
   - Parse single .reg file with duplicates
   - Verify deduplication occurs
   - Check stats accuracy (DedupedSetValue count)

2. ✅ **TestParseAndOptimize_MultipleFiles**
   - Parse 3 .reg files with overlapping operations
   - Verify last-file-wins behavior (file3 ops override file1/file2)
   - Check cross-file deduplication works correctly

3. ✅ **TestParseAndOptimizeSingle**
   - Test convenience wrapper
   - Verify it's equivalent to ParseAndOptimize with 1 file

4. ✅ **TestParseFiles_NoOptimization**
   - Parse without optimization
   - Verify all raw ops preserved (no deduplication)

5. ✅ **TestParseAndOptimize_ErrorHandling**
   - Malformed .reg file (invalid header)
   - Invalid UTF-8 encoding
   - Missing registry header
   - Verify appropriate error messages

6. ✅ **TestOptimizer_OpDeleteValue**
   - Test OpDeleteValue code paths (currently 0% covered)
   - Duplicate deletes (should deduplicate)
   - Deletes under deleted subtrees (should shadow)
   - DeleteValue ordering

**Expected Outcome**: Coverage 74% → 85-90%

---

## Phase 2: Synthetic Test Data

**Goal**: Create realistic .reg files for testing
**Time Estimate**: 15 minutes
**Status**: ✅ In Progress

### Files in: `internal/regmerge/testdata/`

1. ✅ **`base.reg`** (~50 ops)
   - Clean baseline configuration
   - Software installation structure
   - Multiple keys and values
   - No duplicates or conflicts
   - **Use case**: Foundation for patch testing

2. ⏳ **`patch1.reg`** (~30 ops)
   - Updates 10 values from base.reg (tests cross-file dedup)
   - Adds 10 new values (tests merging)
   - Deletes 5 values (tests DeleteValue ops)
   - Adds 5 new keys
   - **Use case**: First update/patch scenario

3. ⏳ **`patch2.reg`** (~20 ops)
   - Updates 5 values from patch1.reg (tests multi-level dedup)
   - Deletes entire subtree (tests delete shadowing)
   - Re-creates keys under deleted subtree (tests replace pattern)
   - **Use case**: Second patch with destructive changes

4. ⏳ **`duplicates.reg`** (100 ops → 10 unique)
   - Same 10 SetValue operations repeated 10 times each
   - Tests worst-case deduplication (90% reduction expected)
   - **Use case**: Stress test deduplication algorithm

5. ⏳ **`deletions.reg`** (~50 ops)
   - Mix of 30 SetValue and 20 DeleteKey operations
   - Operations under deleted subtrees
   - Nested delete hierarchies
   - **Use case**: Stress test delete shadowing optimization

6. ⏳ **`mixed_case.reg`** (~30 ops)
   - Same paths with different cases (HKLM vs hklm vs HkLm)
   - Same value names with different cases
   - Tests case-insensitive normalization
   - **Use case**: Verify path normalization correctness

**Total Test Data**: ~280 operations across 6 files

---

## Phase 3: Benchmark Infrastructure

**Goal**: Create comprehensive table-driven benchmarks
**Time Estimate**: 45 minutes
**Status**: ⏳ Pending

### File: `optimizer_bench_test.go` (NEW)

#### Table-Driven Structure

```go
type benchScenario struct {
    name          string
    regFiles      [][]byte        // Input .reg file(s)
    opts          OptimizerOptions // Optimization settings
    wantReduction float64          // Expected reduction % (for validation)
}
```

#### A. Parsing Benchmarks

1. **Benchmark_ParseSingleFile**
   - Measures: regtext parsing + Optimize() overhead
   - Scenarios: base.reg, duplicates.reg, deletions.reg

2. **Benchmark_ParseMultiFile/2files**
   - Measures: Multi-file parsing + cross-file optimization

3. **Benchmark_ParseMultiFile/5files**
   - Measures: Scalability with more files

4. **Benchmark_ParseMultiFile/10files**
   - Measures: High-file-count overhead

#### B. Optimization Impact Benchmarks (WITH vs WITHOUT)

1. **Benchmark_Optimize_Dedup**
   - `/Enabled` - Dedup on (measure optimization benefit)
   - `/Disabled` - Dedup off (measure overhead)
   - **Compares**: Time with optimization vs without

2. **Benchmark_Optimize_DeleteShadow**
   - `/Enabled` - Delete shadowing on
   - `/Disabled` - Delete shadowing off

3. **Benchmark_Optimize_Ordering**
   - `/Enabled` - I/O ordering on
   - `/Disabled` - No ordering

4. **Benchmark_Optimize_All**
   - `/AllEnabled` - All optimizations
   - `/AllDisabled` - No optimizations
   - `/DedupOnly` - Just deduplication
   - `/OrderingOnly` - Just ordering

#### C. Scale Benchmarks

1. **Benchmark_Optimize_100Ops**
   - Small scale operations

2. **Benchmark_Optimize_1000Ops**
   - Medium scale

3. **Benchmark_Optimize_10000Ops**
   - Large scale (stress test)

**Total Benchmarks**: ~20 scenarios

**Benchmark Pattern**:
```go
func Benchmark_Optimize_Dedup(b *testing.B) {
    scenarios := []struct {
        name string
        opts OptimizerOptions
    }{
        {"Enabled", OptimizerOptions{EnableDedup: true, ...}},
        {"Disabled", OptimizerOptions{EnableDedup: false, ...}},
    }

    testOps := loadTestOps() // Pre-load outside loop

    for _, s := range scenarios {
        b.Run(s.name, func(b *testing.B) {
            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                _, _ = Optimize(testOps, s.opts)
            }
        })
    }
}
```

---

## Phase 4: Correctness Verification

**Goal**: Prove optimizer doesn't change semantics
**Time Estimate**: 30 minutes
**Status**: ⏳ Pending

### File: `correctness_test.go` (NEW)

#### Test Strategy
1. Create test operations (with known duplicates/shadows)
2. Optimize them
3. Verify semantic equivalence (not byte-for-byte, but outcome)
4. Assert stats are accurate

#### Tests to Create

1. **TestCorrectness_DeduplicationPreservesLastWrite**
   ```go
   // Input: Set(key, "v1"), Set(key, "v2"), Set(key, "v3")
   // Expected: Single Set(key, "v3")
   // Stats: DedupedSetValue == 2
   ```

2. **TestCorrectness_DeleteShadowingPreservesOrder**
   ```go
   // Input: Set(A\B, ...), Delete(A), Set(C, ...)
   // Expected: Delete(A), Set(C) only (Set(A\B) removed)
   // Stats: ShadowedByDelete == 1
   ```

3. **TestCorrectness_OrderingPreservesParentChild**
   ```go
   // Input: Set(A\B\C, ...), Set(A, ...), Set(A\B, ...)
   // Expected: A before A\B, A\B before A\B\C
   ```

4. **TestCorrectness_OpCountReduction**
   - For each test scenario
   - Assert: `stats.InputOps >= stats.OutputOps`
   - Assert: `stats.ReductionPercent()` matches expected

5. **TestCorrectness_AllUniqueOpsPreserved**
   ```go
   // Input: Operations with NO duplicates or shadowing
   // Expected: len(output) == len(input) (no ops lost)
   // Stats: DedupedSetValue == 0, ShadowedByDelete == 0
   ```

6. **TestCorrectness_MultiFile_LastFileWins**
   ```go
   // Input: file1 sets key=v1, file2 sets key=v2, file3 sets key=v3
   // Expected: Single op with key=v3 (last file wins)
   // Stats: DedupedSetValue == 2
   ```

---

## Phase 5: Test Utilities

**Goal**: Helper functions for testing/benchmarking
**Time Estimate**: 20 minutes
**Status**: ⏳ Pending

### File: `testutil.go` (NEW)

```go
package regmerge

// CreateTestOps generates synthetic EditOp slices for testing
func CreateTestOps(scenario string, count int) []types.EditOp {
    switch scenario {
    case "duplicates":
        // Return slice with many duplicates
    case "deletes":
        // Return slice with delete shadowing
    case "mixed":
        // Return realistic mixed operations
    case "sequential":
        // Return operations on different keys (no conflicts)
    }
}

// AssertOpSlicesEquivalent checks semantic equivalence
func AssertOpSlicesEquivalent(t *testing.T, expected, actual []types.EditOp) {
    // Normalize both slices (sort, normalize paths)
    // Compare lengths
    // Compare each op with detailed diff on failure
}

// LoadTestRegFile loads .reg file from testdata
func LoadTestRegFile(t *testing.T, filename string) []byte {
    path := filepath.Join("testdata", filename)
    data, err := os.ReadFile(path)
    require.NoError(t, err, "failed to load %s", filename)
    return data
}

// BenchmarkScenario represents a benchmark test case
type BenchmarkScenario struct {
    Name          string
    RegFiles      [][]byte
    Opts          OptimizerOptions
    ExpectedStats Stats // For validation
}

// LoadBenchmarkScenarios returns all standard benchmark scenarios
func LoadBenchmarkScenarios() []BenchmarkScenario {
    return []BenchmarkScenario{
        {
            Name: "NoDuplicates",
            RegFiles: [][]byte{LoadTestRegFile(nil, "base.reg")},
            Opts: DefaultOptimizerOptions(),
            ExpectedStats: Stats{ReductionPercent: 0}, // No optimization
        },
        {
            Name: "90%Duplicates",
            RegFiles: [][]byte{LoadTestRegFile(nil, "duplicates.reg")},
            Opts: DefaultOptimizerOptions(),
            ExpectedStats: Stats{ReductionPercent: 90}, // 100 → 10 ops
        },
        // ... more scenarios
    }
}
```

---

## Phase 6: Optional - End-to-End Integration

**Goal**: Measure optimizer impact on actual hive merges
**Time Estimate**: 1 hour
**Status**: ⏳ Optional (Future Work)

### File: `e2e_bench_test.go` (NEW)

**Approach**:
1. Apply same .reg operations WITH and WITHOUT optimizer
2. Measure total time (parse + optimize + merge)
3. Verify final hives are byte-for-byte identical

**Benchmarks**:
- `Benchmark_E2E_SingleFile/Optimized`
- `Benchmark_E2E_SingleFile/Unoptimized`
- `Benchmark_E2E_MultiFile_2files/Optimized`
- `Benchmark_E2E_MultiFile_2files/Unoptimized`
- `Benchmark_E2E_MultiFile_10files/Optimized`
- `Benchmark_E2E_MultiFile_10files/Unoptimized`

**Correctness Test**:
```go
func TestE2E_OptimizerProducesSameHive(t *testing.T) {
    // Apply WITH optimizer
    hive1 := applyWithOptimizer(baseHive, regFile)

    // Apply WITHOUT optimizer
    hive2 := applyWithoutOptimizer(baseHive, regFile)

    // Compare hives byte-for-byte or semantically
    assertHivesEqual(t, hive1, hive2)
}
```

---

## Implementation Checklist

### Phase 1: Coverage (30 min)
- [ ] Create `multifile_test.go`
- [ ] Add TestParseAndOptimize_SingleFile
- [ ] Add TestParseAndOptimize_MultipleFiles
- [ ] Add TestParseAndOptimizeSingle
- [ ] Add TestParseFiles_NoOptimization
- [ ] Add TestParseAndOptimize_ErrorHandling
- [ ] Add TestOptimizer_OpDeleteValue
- [ ] Run `go test -cover ./internal/regmerge`
- [ ] Verify coverage ≥ 85%

### Phase 2: Test Data (15 min)
- [x] Create `testdata/` directory
- [x] Create `base.reg` (50 ops)
- [ ] Create `patch1.reg` (30 ops)
- [ ] Create `patch2.reg` (20 ops)
- [ ] Create `duplicates.reg` (100 ops)
- [ ] Create `deletions.reg` (50 ops)
- [ ] Create `mixed_case.reg` (30 ops)

### Phase 3: Benchmarks (45 min)
- [ ] Create `optimizer_bench_test.go`
- [ ] Add parsing benchmarks (4 scenarios)
- [ ] Add optimization impact benchmarks (4 groups)
- [ ] Add scale benchmarks (3 sizes)
- [ ] Add combined scenario benchmarks
- [ ] Run `go test -bench=. -benchmem ./internal/regmerge`
- [ ] Verify all benchmarks run without errors

### Phase 4: Correctness (30 min)
- [ ] Create `correctness_test.go`
- [ ] Add TestCorrectness_DeduplicationPreservesLastWrite
- [ ] Add TestCorrectness_DeleteShadowingPreservesOrder
- [ ] Add TestCorrectness_OrderingPreservesParentChild
- [ ] Add TestCorrectness_OpCountReduction
- [ ] Add TestCorrectness_AllUniqueOpsPreserved
- [ ] Add TestCorrectness_MultiFile_LastFileWins
- [ ] Run tests and verify all pass

### Phase 5: Utilities (20 min)
- [ ] Create `testutil.go`
- [ ] Add CreateTestOps function
- [ ] Add AssertOpSlicesEquivalent function
- [ ] Add LoadTestRegFile function
- [ ] Add BenchmarkScenario types
- [ ] Add LoadBenchmarkScenarios function

### Phase 6: Optional E2E (1 hour)
- [ ] Create `e2e_bench_test.go`
- [ ] Add E2E benchmarks
- [ ] Add hive comparison test
- [ ] Run and verify

---

## Expected Outcomes

### After Core Phases (1-5):
- **Coverage**: 90%+ (up from 74%)
- **Benchmarks**: 20+ scenarios measuring optimizer impact
- **Correctness**: Proven that optimizer doesn't change semantics
- **Experimentation**: Easy to toggle opts and measure impact

### Benchmark Output Example:
```
Benchmark_ParseSingleFile/base.reg-10            50000    25000 ns/op   12000 B/op   45 allocs/op
Benchmark_ParseMultiFile/2files-10               30000    45000 ns/op   24000 B/op   90 allocs/op
Benchmark_Optimize_Dedup/Enabled-10             100000     1250 ns/op     480 B/op   12 allocs/op
Benchmark_Optimize_Dedup/Disabled-10            120000     1050 ns/op     380 B/op   10 allocs/op
Benchmark_Optimize_100Ops-10                     50000     2500 ns/op    1200 B/op   45 allocs/op
Benchmark_Optimize_1000Ops-10                     5000    25000 ns/op   12000 B/op  450 allocs/op
```

### Success Criteria:
1. ✅ Coverage ≥ 90%
2. ✅ All tests pass (correctness verified)
3. ✅ Benchmarks show measurable optimization benefits
4. ✅ Overhead of optimization < 20% for most scenarios
5. ✅ Reduction percentage matches expectations for each scenario

---

## Files to Create

1. **`multifile_test.go`** (~150 lines) - Coverage tests
2. **`correctness_test.go`** (~200 lines) - Equivalence verification
3. **`optimizer_bench_test.go`** (~400 lines) - Performance benchmarks
4. **`testutil.go`** (~100 lines) - Helper functions
5. **`testdata/base.reg`** (~50 lines) - Base configuration
6. **`testdata/patch1.reg`** (~30 lines) - First patch
7. **`testdata/patch2.reg`** (~20 lines) - Second patch
8. **`testdata/duplicates.reg`** (~100 lines) - Dedup stress test
9. **`testdata/deletions.reg`** (~50 lines) - Delete shadowing test
10. **`testdata/mixed_case.reg`** (~30 lines) - Case normalization test
11. **`e2e_bench_test.go`** [OPTIONAL] (~200 lines) - E2E integration

**Total**: ~1,330 lines (or ~1,030 for core phases 1-5)

---

## Progress Tracking

**Last Updated**: 2025-11-06
**Current Phase**: Phase 2 (Test Data Creation)
**Overall Progress**: 8% complete (1/12 files done)

### Completed:
- [x] Plan document created
- [x] testdata/ directory created
- [x] base.reg created (50 ops)

### In Progress:
- [ ] Phase 2: Creating remaining test .reg files

### Next Steps:
1. Complete Phase 2 (test data)
2. Start Phase 1 (coverage tests)
3. Run coverage check
4. Proceed to benchmarks

---

## Notes & Observations

- **Coverage Gap**: Multi-file parsing completely untested (0%)
- **Benchmark Strategy**: Table-driven with WITH/WITHOUT comparisons
- **Correctness Approach**: Semantic equivalence, not byte comparison
- **Test Data**: Synthetic .reg files cover all optimization scenarios

---

*This plan is a living document. Update as implementation progresses.*
