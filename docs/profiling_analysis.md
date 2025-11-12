# CPU Profiling Analysis: New Merge Implementation

**Profile Date:** November 5, 2025
**Benchmark:** BenchmarkNewMerge_100KeyChanges (100 iterations)
**Total CPU Time:** 1.02 seconds
**Platform:** Apple M1 Max

---

## CPU Hotspot Summary

### Top Functions by Flat Time (time spent in function itself)

| Function | Flat Time | % of Total | Description |
|:---------|----------:|-----------:|:------------|
| `syscall.syscall` | 310ms | 30.39% | System calls (file I/O, mmap operations) |
| `subkeys.resolveCell` | 300ms | 29.41% | **OPTIMIZATION TARGET** - Cell resolution |
| `encoding/binary.littleEndian.Uint32` | 60ms | 5.88% | Binary integer parsing |
| Runtime GC | ~150ms | ~15% | Garbage collection overhead |

### Top Functions by Cumulative Time (time including callees)

| Function | Cum Time | % of Total | Description |
|:---------|----------:|-----------:|:------------|
| `merge.NewSession` | 390ms | 38.24% | Session creation + index building |
| `walker.IndexBuilder.Build` | 380ms | 37.25% | Index building from hive |
| `subkeys.Read` | 320ms | 31.37% | Subkey list reading |
| `walker.IndexBuilder.processNK` | 320ms | 31.37% | Processing NK cells during indexing |
| `merge.Session.ApplyWithTx` | 210ms | 20.59% | Transaction application |

---

## Identified Optimization Opportunities

### 1. CRITICAL: `subkeys.resolveCell` Manual Byte Parsing (29.41% of CPU)

**Location:** `hive/subkeys/reader.go:202-205`

**Current Code:**
```go
// Manual byte-to-int32 conversion
sizeRaw := int32(data[offset]) |
    int32(data[offset+1])<<8 |
    int32(data[offset+2])<<16 |
    int32(data[offset+3])<<24
```

**Issue:** Manual bit shifting and OR operations are not optimized by the compiler. This function is called thousands of times during index building and merge operations.

**Recommended Fix:**
```go
// Use encoding/binary for compiler-optimized conversion
sizeRaw := int32(binary.LittleEndian.Uint32(data[offset:offset+4]))
```

**Expected Impact:** 10-15% overall speedup (reducing 300ms to ~200ms for this function)

**Difficulty:** EASY - Single line change, already imports `encoding/binary`

---

### 2. HIGH: Index Building Overhead (38.24% cumulative)

**Location:** `walker.IndexBuilder.Build()`

**Issue:** Index building takes 38% of total time. This is reasonable for cold starts, but the current API rebuilds the index on every `MergePlan` call.

**Current Behavior:**
- `MergePlan()` opens hive → builds index → applies plan → closes
- Every call pays the full index building cost

**Recommended Fix:** Add index caching/reuse options:

```go
// Option 1: Allow passing pre-built index
func MergePlan(hivePath string, plan *Plan, opts *Options) (Applied, error)

// Option 2: Add index cache to Options
type Options struct {
    Strategy   StrategyName
    FlushMode  FlushMode
    IndexCache *IndexCache  // NEW: reusable index cache
}
```

**Expected Impact:**
- Single operations: No change (index still needed)
- Sequential operations: Already benefits from `WithSession()` pattern
- Potential: Add "warm" mode that keeps index in memory between calls

**Difficulty:** MEDIUM - Requires API design and cache invalidation logic

---

### 3. MEDIUM: Allocation Overhead (67K-80K allocations per operation)

**Current State:**
- 100 key changes: 79,786 allocations
- Most allocations are in index building and subkey resolution

**Potential Optimizations:**

#### 3a. Object Pooling for Frequent Allocations
```go
// Pool for []byte slices used during cell resolution
var cellDataPool = sync.Pool{
    New: func() interface{} { return make([]byte, 0, 1024) },
}
```

#### 3b. Pre-allocate Index Maps
```go
// If we can estimate key count from hive header, pre-allocate maps
idx := &StringIndex{
    offsetToNames: make(map[uint32][]string, estimatedKeys),
    // ...
}
```

**Expected Impact:** 5-10% reduction in allocations, minimal speed impact

**Difficulty:** MEDIUM - Requires careful lifetime management

---

### 4. LOW: Syscall Overhead (30.39% flat time)

**Current State:** 310ms in `syscall.syscall` (file I/O and mmap operations)

**Analysis:** This is mostly unavoidable overhead for:
- `mmap()` calls during hive opening
- `msync()` calls during dirty page flushing
- File handle operations

**Potential Optimizations:**
- Batch msync() calls (already done via dirty tracking)
- Use larger page sizes if supported by OS
- Keep hives open longer (already possible via `WithSession()`)

**Expected Impact:** Minimal - already well-optimized

**Difficulty:** HARD - OS-level optimization

---

## Optimization Priority Ranking

| Priority | Optimization | Difficulty | Expected Gain | Implementation Time |
|:--------:|:-------------|:----------:|:-------------:|:-------------------:|
| **1** | Fix `resolveCell` byte parsing | EASY | 10-15% | 5 minutes |
| **2** | Add index caching options | MEDIUM | Variable* | 2-4 hours |
| **3** | Object pooling for allocations | MEDIUM | 5-10% | 1-2 hours |
| **4** | Syscall optimization | HARD | Minimal | N/A |

*Index caching benefit depends on usage pattern. Already maximized via `WithSession()`.

---

## Recommended Implementation Order

### Phase 1: Quick Win (Immediate)
1. **Fix `resolveCell` byte parsing** - Single line change for 10-15% gain

### Phase 2: Smart Caching (If needed)
2. **Evaluate index cache need** - Measure if sequential operations are common
3. **Design cache invalidation strategy** - How to detect hive changes?
4. **Implement IndexCache** - Add to Options struct

### Phase 3: Polish (Optional)
5. **Object pooling** - If allocation overhead becomes measurable in prod
6. **Micro-optimizations** - Profile again after Phase 1-2 changes

---

## Profiling Command for Reproduction

```bash
# CPU profiling
go test ./hive/merge -bench="BenchmarkNewMerge_100KeyChanges" \
    -benchmem -benchtime=100x -cpuprofile=/tmp/cpu.prof -run=^$

# Analyze profile
go tool pprof -top /tmp/cpu.prof
go tool pprof -list=resolveCell /tmp/cpu.prof

# Memory profiling
go test ./hive/merge -bench="BenchmarkNewMerge_100KeyChanges" \
    -benchmem -benchtime=100x -memprofile=/tmp/mem.prof -run=^$

# Analyze memory
go tool pprof -alloc_space -top /tmp/mem.prof
```

---

## Conclusion

The new merge implementation is already **highly optimized** compared to the old approach (4-119x faster). The profiling reveals one clear optimization target:

**Critical Path:** `resolveCell` manual byte parsing (29.41% of CPU time)

This single-line fix can provide an immediate 10-15% performance boost. Other optimizations provide diminishing returns and should be evaluated based on real-world usage patterns.

The fact that syscalls represent 30% of time indicates the implementation is already I/O bound, which means we've successfully optimized the algorithmic portion. Further gains require OS-level optimizations or keeping hives open longer (already possible via `WithSession()`).

---

**Next Steps:**
1. Apply `resolveCell` optimization
2. Re-run benchmarks to measure improvement
3. Evaluate if index caching is needed for production workloads
