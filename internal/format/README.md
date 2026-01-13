# Binary Encoding Package

Fast, safe little-endian integer encoding for Windows Registry hive format.

## Quick Start

```go
import "github.com/joshuapare/hivekit/internal/format"

// Reading values
sequence := format.ReadU32(data, 0x04)
count := format.ReadU16(data, offset)
cellSize := format.ReadI32(data, cellOffset)
timestamp := format.ReadU64(data, 0x28)

// Writing values
format.PutU32(buffer, 0x20, value)
format.PutU16(buffer, offset, port)
format.PutI32(buffer, 0x10, signedValue)
format.PutU64(buffer, 0x30, largeValue)
```

## Implementation

Uses Go's standard `encoding/binary.LittleEndian` for maximum:
- **Performance** - Highly optimized by modern Go compilers
- **Safety** - Full bounds checking, no undefined behavior
- **Portability** - Works on all architectures
- **Simplicity** - Clean, maintainable code

## Performance

After comprehensive benchmarking (see PERFORMANCE_TESTING.md), we determined that:

- ✅ `binary.LittleEndian` is **already optimal** for this workload
- ❌ `unsafe.Pointer` implementations provided **no benefit** (actually slower!)
- 🎯 Modern Go compilers inline and optimize these calls extremely well

**Benchmark Results (E2E Merge Tests):**
- Unsafe with bounds checks: +1.70% SLOWER, +1.67% more allocations
- Raw unsafe (no checks): Only 0.98% faster, huge security risk

**Conclusion:** There is no performance justification for using unsafe code.

## Files

- `encoding.go` - Production implementation (encoding/binary)
- `PERFORMANCE_TESTING.md` - Detailed benchmark analysis
- `README.md` - This file
