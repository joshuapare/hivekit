# Regtext Benchmark Tracking Guide

## Quick Start

```bash
# 1. Create baseline (first run)
./scripts/bench_regtext_simple.sh "baseline"

# 2. Make your optimizations...
# (edit code, make changes)

# 3. Run benchmarks with new label
./scripts/bench_regtext_simple.sh "optimized-hex-parsing"

# 4. Compare results
./scripts/compare_bench.sh bench_results.csv baseline optimized-hex-parsing
```

## Current Performance Numbers

### Summary Table (Baseline)

| Category | File Size | Throughput | Memory | Notes |
|----------|-----------|------------|--------|-------|
| **Real-World Best** | 48 MB | **88.10 MB/s** | 393 MB | Win8 CP Software |
| **Real-World Average** | 9-43 MB | **82-87 MB/s** | 77-352 MB | Production files |
| **DWORD-optimized** | 1 MB | **138.32 MB/s** | 10 MB | Simple integers |
| **String-optimized** | 1 MB | **104.31 MB/s** | 12 MB | Text values |
| **Binary-heavy** | 1 MB | **66.67 MB/s** | 9 MB | Hex data (bottleneck) |

### Key Metrics
- **Average throughput**: 75-88 MB/s
- **Memory overhead**: ~8-9 bytes per input byte
- **Allocation rate**: ~250-260K allocations per MB
- **Parsing time**: ~10-12 microseconds per operation

## Benchmark Files

All benchmarks are tracked in: `bench_results.csv`

```csv
timestamp,label,benchmark,ns_per_op,mb_per_sec,bytes_per_op,allocs_per_op,file_size_mb
2025-11-04 16:45:59,baseline,BenchmarkParseReg_XP_System-10,116493792,82.17,81173984,2521283,9.1
```

## Usage Examples

### Run Benchmarks with Custom Label

```bash
# Use descriptive labels
./scripts/bench_regtext_simple.sh "before-optimization"
./scripts/bench_regtext_simple.sh "after-string-pooling"
./scripts/bench_regtext_simple.sh "fix-hex-parser-v2"

# Add git commit info to label
./scripts/bench_regtext_simple.sh "commit-$(git rev-parse --short HEAD)"
```

### Compare Two Runs

```bash
# Compare two specific runs
./scripts/compare_bench.sh bench_results.csv baseline after-string-pooling

# Output shows:
# - Current throughput and memory for each benchmark
# - Percentage change (+ or -) between runs
# - Memory allocation changes
```

Example output:
```
Performance Changes (baseline → optimized-hex-parsing):
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Benchmark              |   Throughput Δ |       Memory Δ
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
BinaryHeavy            |        +45.23% |         -23.50%   <- Big win!
StringHeavy            |         +2.10% |          -1.20%
DWORDHeavy             |         +0.50% |           0.00%
```

### View All Runs

```bash
# See all recorded benchmark runs
cat bench_results.csv

# Count runs per label
cut -d',' -f2 bench_results.csv | sort | uniq -c

# View just one benchmark across all runs
grep "BenchmarkParseReg_BinaryHeavy" bench_results.csv
```

### Track Progress Over Time

```bash
# Extract throughput for a specific benchmark over time
grep "BenchmarkParseReg_1MB" bench_results.csv | \
  awk -F',' '{print $1 "," $2 "," $5 " MB/s"}' | \
  column -t -s','
```

Output:
```
2025-11-04 16:45:59  baseline               73.01 MB/s
2025-11-04 17:10:32  pool-utf16-buffers     85.42 MB/s
2025-11-04 17:45:18  optimize-hex-parser    102.15 MB/s
```

## Profiling for Deep Analysis

### CPU Profiling

```bash
# Generate CPU profile
cd internal/regtext
go test -bench=BenchmarkParseReg_BinaryHeavy \
    -cpuprofile=cpu.out -benchtime=1s

# Analyze hotspots
go tool pprof cpu.out
# Commands in pprof:
#   top10        - Show top 10 functions by CPU time
#   list parseHexBytes  - Show line-by-line breakdown
#   web          - Generate visual graph (requires graphviz)
```

### Memory Profiling

```bash
# Generate memory profile
go test -bench=BenchmarkParseReg_StringHeavy \
    -memprofile=mem.out -benchtime=1s

# Analyze allocations
go tool pprof mem.out
# Commands:
#   top10 -cum   - Top allocators
#   list encodeUTF16LEZeroTerminated
```

### Allocation Tracing

```bash
# See every allocation
go test -bench=BenchmarkParseReg_1KB \
    -benchmem -memprofile=mem.out \
    -benchtime=10x  # Run exactly 10 iterations

go tool pprof -alloc_space mem.out
```

## Interpreting Results

### Throughput (MB/s)
- **Higher is better**
- Real-world files should be 75-90 MB/s
- Generated files may vary based on complexity
- **Target**: 100+ MB/s for common cases

### Memory per Operation (B/op)
- **Lower is better**
- Should scale linearly with file size
- Current: ~8-9 bytes per input byte
- **Target**: <7 bytes per input byte

### Allocations per Operation (allocs/op)
- **Lower is better**
- High allocation counts = GC pressure
- Current: ~250K per MB
- **Target**: <170K per MB

### Variance
- Re-run benchmarks 2-3 times
- Changes <5% are likely noise
- Changes >10% are significant
- Use `-benchtime=1s` for more stable results

## Tips for Optimization Workflow

### 1. Establish Baseline
```bash
# Always start with a clean baseline
./scripts/bench_regtext_simple.sh "baseline-$(date +%Y%m%d)"
```

### 2. Make One Change at a Time
```bash
# After each optimization:
./scripts/bench_regtext_simple.sh "opt-hex-lookup-table"
./scripts/compare_bench.sh bench_results.csv baseline opt-hex-lookup-table
```

### 3. Test Multiple Scenarios
```bash
# Run specific benchmarks
go test -bench='BenchmarkParseReg_(BinaryHeavy|StringHeavy|DWORDHeavy)$' -benchmem
```

### 4. Profile Before Optimizing
```bash
# Find the real bottleneck first
go test -bench=BenchmarkParseReg_BinaryHeavy -cpuprofile=cpu.out
go tool pprof cpu.out
```

### 5. Verify with Real-World Data
```bash
# Always test with real Windows registry exports
go test -bench='BenchmarkParseReg_(XP|Win8|2012)' -benchmem
```

## Benchmark Matrix Coverage

The suite includes:

- ✅ **9 real-world files** (2.6 MB to 48 MB)
- ✅ **5 size variants** (1 KB to 10 MB generated)
- ✅ **4 value type profiles** (string/binary/dword/mixed)
- ✅ **3 structural profiles** (shallow/deep/wide)
- ✅ **3 special cases** (escaping/deletions/large values)
- ✅ **7 micro-benchmarks** (hotspot functions)

Total: **31+ benchmarks** covering all scenarios

## File Locations

```
internal/regtext/
├── parse_bench_test.go      # All benchmark definitions
├── testdata_gen.go           # Synthetic .reg file generator
├── BENCHMARKS.md             # Current performance analysis
└── BENCHMARK_USAGE.md        # This file

scripts/
├── bench_regtext_simple.sh   # Run benchmarks → CSV
└── compare_bench.sh          # Compare results

bench_results.csv             # All tracked results
```

## Next Steps

1. Run baseline: `./scripts/bench_regtext_simple.sh "baseline"`
2. Identify bottleneck: See `BENCHMARKS.md` "Optimization Opportunities"
3. Make optimization
4. Re-run: `./scripts/bench_regtext_simple.sh "optimized-xyz"`
5. Compare: `./scripts/compare_bench.sh bench_results.csv baseline optimized-xyz`
6. Iterate until goals met (100+ MB/s, <7 B/op, <170K allocs/MB)

---

**Questions?** See `BENCHMARKS.md` for detailed performance analysis and optimization opportunities.
