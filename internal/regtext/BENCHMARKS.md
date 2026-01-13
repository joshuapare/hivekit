# Regtext Parser Performance Benchmarks

## Current Performance (After Phase 4 Optimizations) 🚀

**UPDATED: 2025-11-04 - After implementing streaming, UTF-16 optimization, hex parsing rewrite, and string unescaping**

### Dramatic Improvements Achieved
- **Throughput: +310-350%** (4.3x faster than baseline!)
- **Memory: -75-89% reduction** (especially binary-heavy workloads)
- **Allocations: -93% reduction** (250K/MB → 17K/MB)

See [OPTIMIZATION_LESSONS.md](./OPTIMIZATION_LESSONS.md) for detailed explanations of WHY these optimizations worked.

---

## Current Performance (After Phase 4)

### Real-World File Parsing

| File | Size | Throughput (Old → New) | Memory/op (Old → New) | Allocs/op (Old → New) | Time |
|------|------|------------------------|----------------------|----------------------|------|
| Windows XP System | 9.1 MB | 82→**368 MB/s** (+348%) | 77→**15 MB** (-81%) | 2.5M→**175K** (-93%) | ~26ms |
| Windows XP Software | 3.1 MB | 79→**304 MB/s** (+285%) | 28→**7 MB** (-75%) | 898K→**81K** (-91%) | ~11ms |
| Windows 2003 System | 2.6 MB | 76→**337 MB/s** (+343%) | 23→**4 MB** (-82%) | 769K→**56K** (-93%) | ~8ms |
| Windows 2003 Software | 18 MB | 83→**358 MB/s** (+331%) | 152→**36 MB** (-76%) | 4.8M→**352K** (-93%) | ~52ms |
| Windows 8 System | 9.1 MB | 83→**368 MB/s** (+343%) | 77→**15 MB** (-81%) | 2.5M→**175K** (-93%) | ~26ms |
| Windows 8 Software | 30 MB | 87→**374 MB/s** (+330%) | 243→**55 MB** (-77%) | 7.7M→**563K** (-93%) | ~83ms |
| Windows 2012 System | 12 MB | 85→**378 MB/s** (+345%) | 103→**21 MB** (-80%) | 3.4M→**245K** (-93%) | ~34ms |
| Windows 2012 Software | 43 MB | 87→**379 MB/s** (+336%) | 352→**79 MB** (-78%) | 11.1M→**770K** (-93%) | ~118ms |
| Windows 8 CP Software | 48 MB | 88→**373 MB/s** (+324%) | 393→**96 MB** (-76%) | 12.3M→**901K** (-93%) | ~134ms |

**Key Observations:**
- **Throughput increased 4.3x**: Now consistently 337-379 MB/s (was 75-88 MB/s)
- **Memory reduced 75-82%**: Much better memory efficiency across all files
- **Allocations reduced 91-93%**: Massive reduction in GC pressure
- **Real-world impact**: 48MB file parses in 134ms vs 571ms (4.3x faster!)

### Generated Matrix Benchmarks

| Profile | Size | Throughput | Memory/op | Allocs/op | Notes |
|---------|------|------------|-----------|-----------|-------|
| Small (1KB) | 1 KB | 52.03 MB/s | 73 KB | 287 | Small file overhead |
| Medium (1MB) | 1 MB | 73.01 MB/s | 10 MB | 275K | Representative |
| Large (10MB) | 10 MB | 80.07 MB/s | 105 MB | 2.7M | Good scaling |

### Value Type Performance

| Type | Throughput | Memory/op | Allocs/op | Notes |
|------|------------|-----------|-----------|-------|
| **DWORD-heavy** | **138.32 MB/s** | 10 MB | 75K | Fastest (simple integer parsing) |
| **String-heavy** | **104.31 MB/s** | 12 MB | 121K | UTF-16LE encoding overhead |
| Binary-heavy | 66.67 MB/s | 9 MB | 335K | Hex parsing bottleneck |
| Heavy escaping | 59.34 MB/s | 10 MB | 263K | String replacement overhead |

**Performance Insights:**
- DWORD parsing is **2.1x faster** than binary/hex parsing
- String parsing is **1.6x faster** than binary parsing
- Hex byte parsing is the primary bottleneck for binary-heavy workloads
- Escape sequence handling adds ~20% overhead

## Micro-Benchmark Hotspots

### String Operations
- **unescapeRegString**: 25-260 ns/op (0-2 allocs depending on escapes)
- **findClosingQuote**: 27-199 ns/op (0 allocs, linear scan)
- **encodeUTF16LE**: 60-287 ns/op (3-4 allocs)

### Hex Parsing
- **parseHexBytes** (small, 4 bytes): 220 ns/op, 88 B, 7 allocs
- **parseHexBytes** (medium, 64 bytes): 2,789 ns/op, 1,472 B, 67 allocs
- **parseHexBytes** (large, 256 bytes): 73 ns/op, 32 B, 4 allocs ⚠️ *suspicious - needs verification*

### Value Line Parsing
- **String values**: 210 ns/op, 240 B, 4 allocs
- **DWORD values**: 100 ns/op, 68 B, 2 allocs (fastest)
- **Binary values**: 472 ns/op, 232 B, 12 allocs
- **MultiSZ values**: 748 ns/op, 365 B, 18 allocs (slowest)

## Optimization Opportunities

### High-Impact Optimizations
1. **Hex byte parsing** (66% of binary parsing time)
   - Current: Split string, decode each byte separately
   - Proposal: Use lookup table or streaming decoder
   - Expected gain: 30-50% for binary-heavy workloads

2. **UTF-16LE encoding** (affects all string values)
   - Current: Allocates new slice for each value
   - Proposal: Buffer pooling with sync.Pool
   - Expected gain: 15-25% for string-heavy workloads

3. **String escaping** (affects 20-30% of values)
   - Current: Two ReplaceAll calls per value name
   - Proposal: Single-pass escape detection and handling
   - Expected gain: 10-20% for escaping-heavy workloads

### Medium-Impact Optimizations
4. **Scanner buffer allocation**
   - Current: Creates new buffer for each parse
   - Proposal: Reuse buffers across parses
   - Expected gain: 5-10% reduction in allocations

5. **Operation slice pre-allocation**
   - Current: Appends to empty slice
   - Proposal: Estimate size based on file size
   - Expected gain: 5-10% reduction in allocations

## Tracking Performance Changes

### Running Benchmarks

```bash
# Run all benchmarks and save to CSV
./scripts/bench_regtext_simple.sh "my-optimization"

# View comparison report
./scripts/compare_bench.sh

# Compare two specific runs
./scripts/compare_bench.sh bench_results.csv baseline my-optimization

# Run specific benchmark category
cd internal/regtext
go test -bench=BenchmarkParseReg_StringHeavy -benchmem

# Profile for CPU hotspots
go test -bench=BenchmarkParseReg_1MB -cpuprofile=cpu.out
go tool pprof cpu.out

# Profile for memory allocations
go test -bench=BenchmarkParseReg_1MB -memprofile=mem.out
go tool pprof mem.out
```

### Benchmark Results Format

Results are saved to `bench_results.csv`:
```csv
timestamp,label,benchmark,ns_per_op,mb_per_sec,bytes_per_op,allocs_per_op,file_size_mb
2025-11-04 16:45:59,baseline,BenchmarkParseReg_XP_System-10,116493792,82.17,81173984,2521283,9.1
```

### Performance Goals

For optimization efforts, target these improvements:
- **Throughput**: 100+ MB/s for real-world files
- **Memory**: Reduce memory/op by 25% (target: 6-7 bytes per input byte)
- **Allocations**: Reduce allocs/op by 30% (target: ~170K per MB)

## Test Data Generator

Generate synthetic .reg files for testing:

```go
import "github.com/joshuapare/hivekit/internal/regtext"

// Generate 1MB string-heavy file
profile := regtext.ProfileStringHeavy()
profile.Seed = 42  // Reproducible
data := regtext.GenerateRegFile(profile)

// Custom profile
profile := regtext.Profile{
    TargetSize:      10 * 1024 * 1024,  // 10MB
    KeyDepth:        8,
    KeysPerLevel:    20,
    MinValuesPerKey: 5,
    MaxValuesPerKey: 15,
    MinValueSize:    100,
    MaxValueSize:    1000,
    StringValuePct:  0.6,
    BinaryValuePct:  0.3,
    DWORDValuePct:   0.1,
    EscapeFrequency: 0.2,
}
data := regtext.GenerateRegFile(profile)
```

## Benchmark Matrix Coverage

The benchmark suite covers:

### File Sizes
- ✅ Tiny: 1KB
- ✅ Small: 10KB, 100KB
- ✅ Medium: 1MB, 2.6MB, 3.1MB
- ✅ Large: 9.1MB, 10MB, 12MB, 18MB
- ✅ Very Large: 30MB, 43MB, 48MB

### Structural Variations
- ✅ Shallow + Narrow
- ✅ Deep + Narrow
- ✅ Shallow + Wide
- ✅ Real-world (complex hierarchies)

### Value Type Distributions
- ✅ String-heavy (80% REG_SZ)
- ✅ Binary-heavy (80% hex data)
- ✅ DWORD-heavy (80% dword)
- ✅ Mixed (even distribution)

### Special Characteristics
- ✅ Heavy escaping (30% escape sequences)
- ✅ Delete operations (20% deletions)
- ✅ Large values (1KB-10KB per value)
- ✅ Multi-line hex data

### Hotspot Functions
- ✅ unescapeRegString
- ✅ findClosingQuote
- ✅ parseHexBytes
- ✅ encodeUTF16LE
- ✅ decodeInput
- ✅ parseValueLine (all value types)

---

## Optimization Phase History

### Phase 4: String Unescaping (+1-8% throughput)
**What:** Added fast-path check for strings without escape sequences
**Key insight:** Most registry values don't have escapes - check for backslash before processing
**Impact:** 2012_Software improved 8%, most others gained 1-3%
**Lesson:** Small optimizations compound - every 1% matters at peak performance

### Phase 3: Hex Parsing Rewrite (+3-4x throughput)
**What:** Manual nibble-based hex parsing, eliminated all intermediate allocations
**Key insight:** `hex.DecodeString()` is general-purpose but slow - manual parsing is 10-50x faster per byte
**Impact:** Binary parsing went from 67 MB/s → 309 MB/s (4.6x faster!)
**Lesson:** Manual parsing beats standard library for hot paths

### Phase 2: UTF-16 Encoding (+10-18% throughput)
**What:** Batch UTF-16 encoding instead of per-character
**Key insight:** Per-character encoding caused 1500x more allocations than needed
**Impact:** Reduced allocations by 30%, improved throughput 10-18%
**Lesson:** Batch operations > many small operations

### Phase 1: Streaming Architecture (+11-17% throughput)
**What:** Eliminated full file string materialization, used `scanner.Bytes()` instead of `scanner.Text()`
**Key insight:** `scanner.Bytes()` reuses buffer - no line allocation overhead
**Impact:** Saved ~40% memory, improved throughput 11-17%
**Lesson:** Work with []byte, not string - slice operations are free

---

**Last Updated**: 2025-11-04
**Baseline Label**: baseline
**Current Label**: phase4-single-pass
**Platform**: Apple M1 Max (darwin/arm64)
**Go Version**: go version output from benchmarks
