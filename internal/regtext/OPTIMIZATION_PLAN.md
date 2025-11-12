# Regtext Parser Optimization Plan

## Overview
Systematic optimization plan to improve regtext parser performance through 10 phases of improvements. Goal: Reduce GC pressure, minimize allocations, and maximize throughput.

## Baseline Performance
- **Throughput:** 82-87 MB/s
- **Memory:** 77-393 MB per file
- **Allocations:** ~250K per MB

---

## Phase 1: Streaming Architecture ✅ COMPLETED
**Status:** +11-17% throughput
**Goal:** Eliminate full file materialization before parsing

### Changes
- Replace `decodeInput()` returning string with `decodeInputToBytes()` returning []byte
- Use `scanner.Bytes()` instead of `scanner.Text()` to avoid line allocations
- Convert all parsing functions to work with []byte instead of string
- Use `bytes.*` functions instead of `strings.*` where possible

### Why It Worked
- Zero-copy for UTF-8 files (no string conversion)
- `scanner.Bytes()` reuses internal buffer - no allocation per line
- Slice operations don't allocate

### Results
- Throughput: 82-87 → 95-102 MB/s
- Memory: -10-15% reduction

---

## Phase 2: UTF-16 Encoding Optimization ✅ COMPLETED
**Status:** +10-18% throughput
**Goal:** Fix per-character allocations in UTF-16 encoding

### Changes
```go
// Before: Per-character encoding
for _, r := range s {
    words = append(words, utf16.Encode([]rune{r})...)  // 1 allocation per char!
}

// After: Batch encoding
runes := []rune(s)              // 1 allocation
words := utf16.Encode(runes)    // 1 allocation
```

### Why It Worked
- Eliminated 1500x more allocations (1 per string vs 1 per character)
- Single batch conversion is more efficient than many small conversions

### Results
- Allocations reduced by ~30%
- Throughput: 95-102 → 110-115 MB/s

---

## Phase 3: Hex Parsing Rewrite ✅ COMPLETED
**Status:** +200-350% throughput (BIGGEST WIN!)
**Goal:** Eliminate string allocations in hex byte parsing

### Changes
```go
// Before: String split + hex.DecodeString per byte
parts := strings.Split(hexStr, ",")      // Allocates []string
for _, p := range parts {
    b, _ := hex.DecodeString(p)          // Allocates per byte!
}

// After: Manual nibble parsing
for i < len(hexBytes) {
    hi := hexCharToNibble(hexBytes[i])   // Direct byte lookup
    lo := hexCharToNibble(hexBytes[i+1])
    result = append(result, (hi<<4)|lo)  // Bit manipulation
}
```

### Why It Worked
- No intermediate string allocations
- Manual parsing 10-50x faster than `hex.DecodeString()` per byte
- Single-pass algorithm (no preprocessing)
- Cache-friendly sequential access

### Results
- Binary parsing: 67 → 309 MB/s (4.6x faster!)
- Overall: 110-115 → 346-372 MB/s
- Memory: -70-80% on binary-heavy workloads

---

## Phase 4: String Unescaping Optimization ✅ COMPLETED
**Status:** +1-8% throughput
**Goal:** Zero-copy fast path for strings without escapes

### Changes
```go
// Before: Always process
s = strings.ReplaceAll(s, "\\\\", "\\")
s = strings.ReplaceAll(s, "\\\"", "\"")

// After: Check first
if strings.IndexByte(s, '\\') == -1 {
    return s  // No escapes, return original
}
// Only process if needed
```

### Why It Worked
- Most registry values don't have escape sequences
- Single `IndexByte()` check is cheaper than two `ReplaceAll()` operations
- Early exit returns original string (zero allocation)

### Results
- Throughput: 346-372 → 357-379 MB/s
- Biggest gains on workloads with simple identifiers

---

## Phase 5: Buffer Pooling ❌ SKIPPED
**Status:** -1-6% throughput (degraded performance)
**Goal:** Use sync.Pool to reduce GC pressure

### Why It DIDN'T Work
- **Overhead exceeds benefits:** sync.Pool Get/Put adds function call overhead
- **Sequential code:** No concurrency, no contention - pooling doesn't help
- **Required copy:** Still need to allocate return value, so pooling just adds overhead
- **Small buffers:** Allocating 10-100 byte buffers is very fast in Go

### Key Lesson
Buffer pooling is for:
- High concurrency scenarios
- Large allocations (MB range)
- Long-lived objects
NOT for sequential parsing with small buffers!

---

## Phase 6: String Interning ❌ SKIPPED
**Status:** Failed twice - both approaches degraded performance significantly
**Goal:** Deduplicate repeated registry paths using string interning

### Attempt 1: Manual Map-Based Interning ❌
**Implementation:**
- Created `stringInterner` struct with cache map
- Added `intern()` method that checks cache, returns existing or stores new
- Updated parser to intern all paths before creating EditOps

**Results:**
- **-2% to -14% throughput regression**
- **+10% to +27% memory increase**

**Why it failed:**
1. Map lookup overhead on every path operation
2. Map stores both keys AND values (double memory)
3. No automatic cleanup mechanism
4. Hot path penalty exceeded deduplication benefit

---

### Attempt 2: Go 1.23 `unique` Package ❌
After the first failure, we researched Go 1.23's `unique` package, specifically designed for string interning. This seemed like the perfect solution.

**Implementation:**
```go
import "unique"

// Changed all Path fields to unique.Handle[string]
type OpSetValue struct {
    Path unique.Handle[string]  // Was: string
    Name string
    Type types.RegType
    Data []byte
}

// Interned at creation
pathHandle := unique.Make(string(section))
ops = append(ops, types.OpCreateKey{Path: pathHandle})

// Used handles in comparisons
seenKeys := make(map[unique.Handle[string]]bool)
```

**Registry duplication analysis showed huge potential:**
- Win8_Software: 563K operations, ~5K unique paths
- Average: Each path used 112 times
- Expected memory savings: ~55 MB

**Results - CATASTROPHIC:**
```
Benchmark              Before → After    Change
─────────────────────────────────────────────────
XP_System:             389 → 179 MB/s   -53%
2003_Software:         376 → 132 MB/s   -64%
Win8_Software:         398 → 109 MB/s   -72%
Win8_Software allocs:  562K → 1.06M     +88%
```

**Why it failed so badly:**

The `unique` package is designed for **long-lived values** that:
- Persist throughout the program lifetime
- Are compared frequently during their lifetime
- Benefit from pointer-based equality checks

**Our use case is the opposite:**
```
Parse → Create EditOp → Apply to Hive → Discard
   ↑         ↑              ↑            ↑
  500µs    Instant       Instant      GC'd

EditOps are SHORT-LIVED and TRANSIENT!
```

**Fundamental architectural mismatch:**
1. **563K `unique.Make()` calls** - Each creates handle wrapper overhead
2. **No comparison benefit** - EditOps barely compared (only path comparison in seenKeys)
3. **Wrapper overhead** - `unique.Handle[string]` is a struct, not a free abstraction
4. **Wrong pattern** - Values created → used once → discarded immediately
5. **GC can't optimize** - Runtime sees 563K unique handles for 5K strings before GC runs

**Comparison to successful `unique` usage:**
- `net/netip`: Zone strings persist in IP objects for entire program lifetime
- Our case: Operations live for microseconds in parser context

---

### Registry File Duplication Pattern
Despite massive duplication, interning doesn't help:
```
HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion
  ↑ This path appears in 1000s of operations

Problem: Each operation is created → used → discarded in milliseconds
Solution: The duplication exists, but lifetime too short to benefit from interning
```

---

### Key Lesson Learned

**String interning is HIGHLY use-case dependent:**

✅ **Good for interning:**
- Long-lived data structures (config, network zones, identifiers)
- Values compared frequently throughout lifetime
- Persistent caches or lookup tables

❌ **Bad for interning:**
- Transient parsing operations (our case!)
- Short-lived objects that are GC'd immediately
- Values used once then discarded
- Hot paths where wrapper/lookup overhead exceeds benefit

**Our optimization success (4.6x) came from:**
- Eliminating allocations (Phases 1-4)
- Pre-allocation (Phase 7)
- NOT from trying to deduplicate short-lived strings!

---

### What We Tried vs What Would Actually Help

**What we tried (both failed):**
- Deduplicate strings in transient operations

**What would actually help (if needed):**
- Keep EditOps in arena/pool for reuse (changes entire architecture)
- Batch operations to keep alive longer (breaks streaming model)
- Store handles in the HIVE itself (wrong layer, API change)

**Conclusion:** String interning isn't applicable to our streaming parser pattern. The 4.6x improvement from other phases is the ceiling for this architecture.

---

## Phase 7: Pre-allocation ✅ COMPLETED
**Status:** +1-9% throughput, -20-27% memory!
**Goal:** Estimate operations slice size to avoid growth overhead

### Changes
```go
// Before: Let slice grow dynamically
var ops []types.EditOp  // Starts nil, grows on each append

// After: Pre-allocate based on input size
estimatedOps := len(textBytes) / 50  // ~20 ops/KB from real data
ops := make([]types.EditOp, 0, estimatedOps)
```

### Why It Worked
- Slice growth requires reallocation + copying all elements
- For 100K ops: ~17 reallocations, ~200K element copies
- Pre-allocation eliminates all growth overhead

### Results
- Throughput: 357-379 → 377-398 MB/s
- **Memory: -20-27%** (huge win!)
- Win8_Software: 374 → 398 MB/s (+6%)

### Real-World Pattern
Analysis showed consistent ratio across all files:
- XP_System (9.1 MB): 175K ops = 19 ops/KB
- 2003_Software (18 MB): 352K ops = 20 ops/KB
- Win8_Software (30 MB): 563K ops = 19 ops/KB

---

## Phase 8: Byte-Oriented Operations ❌ SKIPPED
**Status:** Failed - manual optimization slower than stdlib
**Goal:** Replace remaining string operations with byte operations

### What We Tried

**Analysis of remaining string operations:**
1. **Path handling** - Must convert to string (EditOp API + map keys require strings)
2. **Value name parsing** - Already optimized with `unescapeRegStringBytes`
3. **String unescaping** - Targeted `unescapeRegStringBytes` for optimization

**Implementation attempt:**
```go
// Original: Use strings.ReplaceAll (Phase 4)
func unescapeRegStringBytes(b []byte) string {
    if bytes.IndexByte(b, '\\') == -1 {
        return string(b) // Fast path
    }
    s := string(b)
    s = strings.ReplaceAll(s, EscapedBackslash, Backslash)
    s = strings.ReplaceAll(s, EscapedQuote, Quote)
    return s
}

// Phase 8 attempt: Manual byte-by-byte loop
func unescapeRegStringBytes(b []byte) string {
    if bytes.IndexByte(b, '\\') == -1 {
        return string(b)
    }
    result := make([]byte, 0, len(b))
    for i := 0; i < len(b); i++ {
        if b[i] == '\\' && i+1 < len(b) {
            if b[i+1] == '\\' {
                result = append(result, '\\')
                i++
            } else if b[i+1] == '"' {
                result = append(result, '"')
                i++
            }
        } else {
            result = append(result, b[i])
        }
    }
    return string(result)
}
```

---

### Results - SIGNIFICANT REGRESSION

**Benchmark comparison (phase7-stable → phase8-manual-loop):**
```
Benchmark              Throughput Δ   Memory Δ
────────────────────────────────────────────────
XP_System:                   -6%        +27%
2003_Software:              -10%        +35%
Win8_Software:               -5%        +38%
2012_Software:               -4%        +26%
Generated_1KB:              -20%         -3%
BinaryHeavy:                 -2%        -19% (only bright spot)
DWORDHeavy:                  -9%        +12%
```

**Impact:** -5% to -10% throughput, +27% to +38% memory on real workloads!

---

### Why It Failed

**Hypothesis was wrong:** We thought avoiding `strings.ReplaceAll` would reduce allocations.

**Reality:** Go's standard library is HIGHLY optimized:

1. **strings.ReplaceAll is battle-tested:**
   - Uses optimized assembly for string operations on many platforms
   - Employs Boyer-Moore or similar algorithms for efficient searching
   - Compiler can inline and optimize standard library calls
   - Decades of tuning by Go team

2. **Manual loop is naive:**
   - Byte-by-byte appending has overhead
   - Each `append` may trigger bounds checks
   - No SIMD or assembly optimizations
   - Potential buffer growth reallocations

3. **BinaryHeavy was the exception:**
   - -2% throughput but **-19% memory**
   - Binary data rarely has escapes → hits fast path more
   - Shows our manual loop hurts the slow path specifically

---

### Key Lessons Learned

**Don't try to out-optimize the standard library:**
- `strings.ReplaceAll`, `bytes.Index`, etc. are professionally optimized
- Manual loops rarely beat stdlib implementations
- When stdlib works well, use it!

**When stdlib optimization works:**
- Our Phase 1-4 optimizations worked because we:
  - Changed algorithms (streaming vs full-file load)
  - Eliminated unnecessary work (Phase 3: manual hex parsing vs hex.DecodeString per byte)
  - Changed allocation patterns (Phase 7: pre-allocation)
- NOT because we rewrote stdlib functions

**Phase 8 conclusion:**
The parser is already well-optimized. The remaining string conversions are:
- **Necessary:** EditOp API requires strings, map keys require strings
- **Minimal:** Only convert at the last moment (EditOp creation)
- **Unavoidable:** Can't defer further without API changes

**Verdict:** Phase 8 has no viable optimizations. Parser has hit its optimization ceiling at 4.6x improvement.

---

## Phase 9: mmap Support (Optional) 📋 PENDING
**Status:** Not started
**Goal:** Memory-map large files instead of reading into memory

### Benefits
- OS manages paging
- Multiple parsers can share same memory
- Lazy loading - only pages actually accessed are loaded

### Challenges
- Platform-specific (Unix vs Windows)
- Requires syscall package
- May not help for files that are fully read anyway
- Adds complexity

### Implementation Plan
```go
import "golang.org/x/sys/unix"  // Or windows

func mmapFile(path string) ([]byte, error) {
    f, err := os.Open(path)
    defer f.Close()

    stat, _ := f.Stat()
    size := int(stat.Size())

    data, err := unix.Mmap(int(f.Fd()), 0, size,
        unix.PROT_READ, unix.MAP_SHARED)
    return data, err
}
```

### Decision Criteria
- Only implement if benchmarks show 5%+ improvement
- Focus on very large files (>50MB)
- Consider maintenance burden vs benefit

---

## Phase 10: Micro-optimizations Polish 📋 PENDING
**Status:** Not started
**Goal:** Final pass for small improvements

### Areas to Investigate
1. **Inline hints** - Add `//go:noinline` or `//go:inline` where beneficial
2. **Bounds check elimination** - Restructure loops to eliminate bounds checks
3. **Switch to jump table** - Ensure `hexCharToNibble` compiles to jump table
4. **Loop unrolling** - Manually unroll critical loops if beneficial
5. **SIMD opportunities** - Identify vectorizable operations

### Example: Bounds Check Elimination
```go
// Before: Bounds check on every access
for i := 0; i < len(data); i++ {
    process(data[i])  // Bounds check
}

// After: Single bounds check
if len(data) > 0 {
    for i := range data {
        process(data[i])  // Compiler eliminates check
    }
}
```

### Approach
1. Generate assembly output: `go build -gcflags="-S"`
2. Look for unnecessary bounds checks
3. Restructure hot paths to eliminate checks
4. Validate with benchmarks (may be noise-level gains)

---

## Current Status Summary

| Phase | Status | Throughput Δ | Memory Δ | Notes |
|-------|--------|--------------|----------|-------|
| 1. Streaming | ✅ Done | +11-17% | -10-15% | Eliminated file materialization |
| 2. UTF-16 | ✅ Done | +10-18% | -30% allocs | Batch encoding |
| 3. Hex Parsing | ✅ Done | +200-350% | -70-80% | BIGGEST WIN - manual parsing |
| 4. Unescaping | ✅ Done | +1-8% | 0% | Fast path for common case |
| 5. Buffer Pool | ❌ Skipped | -1-6% | N/A | Sequential code hurt by overhead |
| 6. String Intern | ❌ Skipped | -50-72% | +88% | Wrong pattern: short-lived transient data |
| 7. Pre-allocation | ✅ Done | +1-9% | -20-27% | Huge memory win! |
| 8. Byte-oriented | ❌ Skipped | -5-10% | +27-38% | Can't beat stdlib: strings.ReplaceAll |
| 9. mmap | 📋 Pending | TBD | TBD | Optional, for very large files |
| 10. Micro-opts | 📋 Pending | TBD | TBD | Final polish pass |

## Overall Progress
- **Baseline:** 82-87 MB/s, 77-393 MB, 250K allocs/MB
- **Current (Phase 7):** 377-398 MB/s, 11-62 MB, 17K allocs/MB
- **Improvement:** **4.6x faster, 80-85% less memory, 93% fewer allocations**

## Next Steps
1. ⚠️ Evaluate Phase 9: mmap (optional, benchmark first - likely not worth it)
2. ⚠️ Evaluate Phase 10: Micro-optimizations (optional - diminishing returns expected)

**Recommendation:** Stop here. 4.6x improvement is excellent. Phases 9-10 have low probability of significant gains and add complexity.

---

**Last Updated:** 2025-11-04
**Current Phase:** Complete (Phase 7 is final optimization)
**Final Result:** 4.6x faster, 80-85% less memory, 93% fewer allocations
