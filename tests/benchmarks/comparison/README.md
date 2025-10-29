# Performance Comparison Benchmarks

## Overview

This directory contains **side-by-side performance benchmarks** comparing gohivex against the original hivex library.

The goal is to **measure and compare performance** across all hivex API functions, tracking:
- **Execution time** (ns/op)
- **Memory allocations** (allocs/op)
- **Bytes allocated** (B/op)

## Purpose

- **Validate performance goals**: Ensure gohivex meets or exceeds hivex performance
- **Identify bottlenecks**: Find operations that need optimization
- **Track progress**: Monitor performance improvements over time
- **Guide optimization**: Prioritize work based on comparative metrics

## Benchmark Structure

Each benchmark file covers a category of operations:

| File | Operations Covered | Status |
|------|-------------------|--------|
| `open_bench_test.go` | Open, close, open+root | ✅ Implemented |
| `navigation_bench_test.go` | Root, children, get_child, name, full tree walk | ✅ Implemented |
| `metadata_bench_test.go` | Timestamp, nr_children, nr_values, StatKey, DetailKey | ✅ Implemented |
| `values_bench_test.go` | Node values, value key, value type, value bytes, get value, StatValue | ✅ Implemented |
| `typed_values_bench_test.go` | Value DWORD, decode vs raw bytes comparison | ✅ Implemented |
| `introspection_bench_test.go` | Last modified, name lengths, struct sizes, data cell offsets (hivex-only baseline) | ✅ Implemented |

## Running Benchmarks

### Run all comparison benchmarks
```bash
go test -bench=. ./tests/benchmarks/comparison/...
```

### Run specific benchmark
```bash
go test -bench=BenchmarkOpen ./tests/benchmarks/comparison/
```

### Run with memory allocation tracking
```bash
go test -bench=. -benchmem ./tests/benchmarks/comparison/...
```

### Run multiple iterations for accuracy
```bash
go test -bench=. -benchtime=10s ./tests/benchmarks/comparison/...
```

### Save results for comparison
```bash
go test -bench=. -benchmem ./tests/benchmarks/comparison/... > bench_new.txt
```

## Benchmark Output Example

```
BenchmarkOpen/gohivex/small-8         50000    28453 ns/op     1024 B/op    12 allocs/op
BenchmarkOpen/hivex/small-8           20000    65432 ns/op     2048 B/op    24 allocs/op
BenchmarkOpen/gohivex/medium-8        45000    29876 ns/op     1024 B/op    12 allocs/op
BenchmarkOpen/hivex/medium-8          18000    67891 ns/op     2048 B/op    24 allocs/op
BenchmarkOpen/gohivex/large-8         10000   145234 ns/op     8192 B/op    45 allocs/op
BenchmarkOpen/hivex/large-8            5000   312456 ns/op    16384 B/op    89 allocs/op
```

### Reading the Output

- **`50000`**: Number of iterations run
- **`28453 ns/op`**: Nanoseconds per operation (lower is better)
- **`1024 B/op`**: Bytes allocated per operation (lower is better)
- **`12 allocs/op`**: Number of allocations per operation (lower is better)

### Quick Comparison

From the example above:
- ✅ gohivex is **2.3x faster** than hivex for small hives
- ✅ gohivex uses **50% less memory** (1024 vs 2048 bytes)
- ✅ gohivex makes **50% fewer allocations** (12 vs 24)

## Using benchstat for Analysis

[`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) provides statistical comparison of benchmark results.

### Install benchstat
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

### Compare two benchmark runs
```bash
# Run baseline
go test -bench=. -benchmem ./tests/benchmarks/comparison/... > old.txt

# Make changes...

# Run new version
go test -bench=. -benchmem ./tests/benchmarks/comparison/... > new.txt

# Compare
benchstat old.txt new.txt
```

### Example benchstat output
```
name                    old time/op    new time/op    delta
Open/gohivex/small-8      28.5µs ± 2%    25.3µs ± 1%   -11.23%  (p=0.000 n=10+10)
Open/hivex/small-8        65.4µs ± 3%    64.8µs ± 2%      ~     (p=0.123 n=10+10)

name                    old alloc/op   new alloc/op   delta
Open/gohivex/small-8      1.02kB ± 0%    0.89kB ± 0%   -12.75%  (p=0.000 n=10+10)
Open/hivex/small-8        2.05kB ± 0%    2.05kB ± 0%      ~     (all equal)

name                    old allocs/op  new allocs/op  delta
Open/gohivex/small-8        12.0 ± 0%      10.0 ± 0%   -16.67%  (p=0.000 n=10+10)
Open/hivex/small-8          24.0 ± 0%      24.0 ± 0%      ~     (all equal)
```

## Test Hives

Benchmarks use hives of different sizes to test scalability:

| Name | Size | Description | Purpose |
|------|------|-------------|---------|
| **small** | ~8KB | Minimal hive, empty | Measure overhead |
| **medium** | ~8KB | 3 keys, special chars | Test encoding paths |
| **large** | ~446KB | Many keys/values | Test scalability |

## Benchmark Categories

### 1. Open/Close Operations

**What's tested:**
- `hivex_open` - Opening hive file
- `hivex_close` - Closing hive
- Combined operations (open + root, open + metadata)

**Key metrics:**
- Cold start performance
- Memory allocation during open
- Cleanup efficiency

### 2. Navigation Operations (TODO)

**What's tested:**
- `hivex_root` - Get root node
- `hivex_node_children` - Enumerate children
- `hivex_node_parent` - Get parent
- `hivex_node_get_child` - Find child by name
- Path lookups

**Key metrics:**
- Tree traversal speed
- Lookup efficiency
- Cache effectiveness

### 3. Metadata Operations (TODO)

**What's tested:**
- `hivex_node_name` - Get node name
- `hivex_node_timestamp` - Get timestamp
- Count queries (children, values)

**Key metrics:**
- Decode performance
- Metadata access speed

### 4. Value Operations (TODO)

**What's tested:**
- Value enumeration
- Value reading
- Type-specific decoding (string, dword, qword)

**Key metrics:**
- Data access patterns
- Decoding overhead (UTF-16LE)
- Allocation for value data

### 5. Full Traversal (TODO)

**What's tested:**
- Walking entire tree
- Reading all metadata
- Reading all values

**Key metrics:**
- End-to-end performance
- Cumulative allocation
- Real-world usage patterns

## Performance Goals

gohivex aims to **outperform** hivex across all operations:

| Operation | Target | Rationale |
|-----------|--------|-----------|
| **Open** | 2-3x faster | Optimized parsing, zero-copy |
| **Navigation** | 1.5-2x faster | Better data structures |
| **Metadata** | 2x faster | Efficient decoding |
| **Values** | 1.5-2x faster | Optimized UTF-16LE |
| **Memory** | 50% less | Fewer allocations, zero-copy |

## Benchmark Pattern

All benchmarks follow this pattern:

```go
func BenchmarkOperation(b *testing.B) {
    for _, hf := range BenchmarkHives {
        // gohivex benchmark
        b.Run("gohivex/"+hf.Name, func(b *testing.B) {
            // Setup (not benchmarked)
            setup()

            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                result := performOperation()
                benchResult = result // Prevent optimization
            }
        })

        // hivex benchmark
        b.Run("hivex/"+hf.Name, func(b *testing.B) {
            // Setup (not benchmarked)
            setup()

            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                result := performOperation()
                benchResult = result // Prevent optimization
            }
        })
    }
}
```

## Preventing Compiler Optimizations

Benchmark results are assigned to global variables in `common.go` to prevent the compiler from eliminating "dead code":

```go
var benchGoNodeID hive.NodeID
var benchHivexNode bindings.NodeHandle

// In benchmark:
for i := 0; i < b.N; i++ {
    node := getNode()
    benchGoNodeID = node // Prevents elimination
}
```

## Adding New Benchmarks

1. Create or edit the appropriate benchmark file
2. Follow the naming convention: `Benchmark<Operation>`
3. Test both implementations with same inputs
4. Use `b.ReportAllocs()` to track allocations
5. Use `b.SetBytes()` for throughput benchmarks
6. Assign results to prevent optimization

Example:

```go
func BenchmarkNewOperation(b *testing.B) {
    for _, hf := range BenchmarkHives {
        b.Run("gohivex/"+hf.Name, func(b *testing.B) {
            // gohivex implementation
            b.ReportAllocs()
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // ... benchmark code
            }
        })

        b.Run("hivex/"+hf.Name, func(b *testing.B) {
            // hivex implementation
            b.ReportAllocs()
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // ... benchmark code
            }
        })
    }
}
```

## Continuous Performance Monitoring

### Track over time
```bash
# Save baseline
go test -bench=. -benchmem ./tests/benchmarks/comparison/... > baseline.txt

# After each major change
go test -bench=. -benchmem ./tests/benchmarks/comparison/... > current.txt
benchstat baseline.txt current.txt
```

### CI Integration
Consider running benchmarks in CI and failing if performance regresses beyond threshold.

## Optimization Workflow

1. **Identify**: Run benchmarks to find slow operations
2. **Profile**: Use `go test -cpuprofile` or `-memprofile`
3. **Optimize**: Improve implementation
4. **Verify**: Re-run benchmarks with benchstat
5. **Repeat**: Continue until goals are met

## Related Documentation

- **Acceptance Tests**: See `../../acceptance/README.md` for correctness testing
- **Profiling Guide**: (TODO) How to use pprof for deeper analysis
- **Optimization Log**: (TODO) Track of optimizations and their impact
