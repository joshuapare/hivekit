# Merge Benchmarks Test Data

This directory contains .reg delta files used for benchmarking merge performance across different scenarios.

## Overview

The delta files are designed to work with real Windows Registry hive files from `testdata/` and `testdata/suite/`. They use **actual keys that exist in the hives** to ensure realistic benchmarks (not no-ops).

## Directory Structure

```
deltas/
├── small/              # 5-10 operations per file
├── medium/             # 50-100 operations per file
├── large/              # 500-1000 operations per file
├── sequential/         # 20 files with 5 operations each for sequential merging
└── suite/              # Patches for production-sized hives (2-34MB)
    ├── win2003-system-small.reg
    ├── win2003-system-medium.reg
    ├── win2012-system-small.reg
    ├── win2012-system-medium.reg
    ├── win2012-system-large.reg
    ├── win2012-software-small.reg
    ├── win2012-software-medium.reg
    └── sequential/     # 5 sequential patches for suite hives
```

## Delta Files

### Small Deltas (~5-10 operations)

**Target hive**: `testdata/large` (436 KB)

- `add.reg` - Add 5 new keys with different value types (REG_SZ, REG_DWORD, REG_BINARY, REG_QWORD, REG_MULTI_SZ)
- `modify.reg` - Modify existing values in `\A\A giant\A giant elephant`, `\Another\...`, `\The\...` keys
- `delete.reg` - Delete values and subkeys from existing keys
- `mixed.reg` - Combination of add, modify, and delete operations

### Medium Deltas (~50-100 operations)

**Target hive**: `testdata/large`

- `add.reg` - Add 50 nested keys across 5 levels with various value types
- `modify.reg` - Modify all 12 existing animal keys (A, Another, The × elephant, goat, mongoose, zebra)
- `delete.reg` - Delete values from multiple keys and remove 2 entire subkeys
- `mixed.reg` - 30 adds + 30 modifies + 20 deletes

### Large Deltas (~500-1000 operations)

**Target hive**: `testdata/large`

- `add.reg` - Add 500 new keys with nested structure across 10 levels
- `modify.reg` - 500 value modifications (100 iterations × 12 keys × multiple values)
- `delete.reg` - Delete all values from all 12 existing keys (36 delete operations)
- `mixed.reg` - 400 adds + 400 modifies + 200 deletes

### Sequential Deltas

**Target hive**: `testdata/large` or `testdata/minimal`

20 delta files (`delta_01.reg` through `delta_20.reg`), each adding 5 new unique keys under `\BenchSeq\DeltaXX\`.

- Used for testing sequential merge performance
- Each delta is independent and adds non-overlapping keys
- Total of 100 new keys when all 20 are applied

### Suite Deltas (Production Hive Benchmarks)

**Target hives**: `testdata/suite/` (2-34 MB production-sized hives)

These deltas are designed for benchmarking merge performance against production-sized Windows Registry hives. **They skip automatically when running with `-short` flag**, making them suitable for CI environments while still allowing comprehensive performance testing.

#### System Hive Deltas

**Windows 2003 Server System** (2 MB hive):
- `win2003-system-small.reg` - 10 operations across 7 top-level keys (Select, Setup, WPA, ControlSet001, ControlSet002, LastKnownGoodRecovery, MountedDevices)
- `win2003-system-medium.reg` - 50 operations distributed across entire hive tree

**Windows 2012 System** (9 MB hive):
- `win2012-system-small.reg` - 10 operations across 9 top-level keys (Select, Setup, WPA, ControlSet001, ControlSet002, DriverDatabase, HardwareConfig, MountedDevices, RNG)
- `win2012-system-medium.reg` - 100 operations distributed across Services, Control, Enum, DriverDatabase, HardwareConfig
- `win2012-system-large.reg` - 500+ operations with comprehensive coverage:
  - Select (50 ops), Setup (50 ops), WPA (50 ops)
  - ControlSet001 (125 ops: Services, Control, Enum)
  - ControlSet002 (125 ops: Services, Control, Enum)
  - DriverDatabase (50 ops: DeviceIds, DriverPackages, DriverInfFiles)
  - HardwareConfig (20 ops), MountedDevices (20 ops), RNG (10 ops)

#### Software Hive Deltas

**Windows 2012 Software** (34 MB hive):
- `win2012-software-small.reg` - 10 operations across 8 top-level keys (ATI Technologies, Classes, Clients, Microsoft, ODBC, Policies, RegisteredApplications, Wow6432Node)
- `win2012-software-medium.reg` - 50+ operations:
  - ATI Technologies (5 ops), Classes (10 ops), Clients (5 ops)
  - Microsoft (15 ops: Uninstall, App Paths, CurrentVersion)
  - ODBC (5 ops), Policies (5 ops), RegisteredApplications (5 ops)
  - Wow6432Node (5 ops for 32-bit compatibility)

#### Suite Sequential Patches

**Target hive**: `testdata/suite/windows-2012-system` (9 MB)

Five sequential patches designed to test sequential merge scenarios on production hives:
- `patch_01.reg` - Select and Setup modifications
- `patch_02.reg` - WPA and MountedDevices modifications
- `patch_03.reg` - ControlSet001 Services and Control
- `patch_04.reg` - ControlSet002 and DriverDatabase
- `patch_05.reg` - HardwareConfig and RNG (final patch)

Each patch targets different top-level keys to exercise different parts of the hive tree.

#### Key Design Principles

1. **Top-level key distribution**: Operations are spread across different root keys rather than concentrated under one subtree (e.g., ControlSet001), forcing the merge algorithm to traverse multiple branches
2. **Realistic operations**: Target actual registry paths that exist in production Windows installations
3. **Varied depths**: Operations at different tree depths to test recursive merge performance
4. **CI-friendly**: All suite benchmarks check `testing.Short()` and skip automatically with `-short` flag

## Key Mappings

### Large Hive Structure

The `testdata/large` hive contains:

```
\A\A giant\
  ├── A giant elephant (values: A, B, C)
  ├── A giant goat (values: A, B, C)
  ├── A giant mongoose (values: A, B, C)
  └── A giant zebra (values: A, B, C)

\Another\Another giant\
  ├── Another giant elephant
  ├── Another giant goat
  ├── Another giant mongoose
  └── Another giant zebra

\The\The giant\
  ├── The giant elephant
  ├── The giant goat
  ├── The giant mongoose
  └── The giant zebra
```

All **modify** and **delete** operations target these existing keys to ensure they perform actual work rather than being no-ops.

## Usage in Benchmarks

### Single Merge Benchmarks
```
BenchmarkMergeSingleDelta/{implementation}/{hive_size}/{delta_size}
```

Examples:
- `BenchmarkMergeSingleDelta/gohivex/large/small` - Merge small delta into large hive
- `BenchmarkMergeSingleDelta/gohivex/large/large` - Merge large delta into large hive

### Sequential Merge Benchmarks
```
BenchmarkMergeSequential/{implementation}/{hive_size}/{num_deltas}
```

Examples:
- `BenchmarkMergeSequential/gohivex/large/5` - Apply 5 sequential deltas
- `BenchmarkMergeSequential/gohivex/large/20` - Apply all 20 sequential deltas

### Full Hive Merge Benchmarks
```
BenchmarkMergeFullHive/{implementation}/{hive_name}
```

Uses complete .reg exports from `testdata/suite/*.reg`:
- `windows-2003-server-system.reg` (2.6 MB, realistic Windows Server 2003 System hive)
- `windows-2012-system.reg` (12 MB, realistic Windows Server 2012 System hive)

### Suite Hive Benchmarks (Production-Sized)

**IMPORTANT**: These benchmarks skip when running with `-short` flag.

```
BenchmarkMergeSuiteHives_System/{hive}/{delta_size}
BenchmarkMergeSuiteHives_Software/{hive}/{delta_size}
BenchmarkMergeSuiteSequential/{hive}
```

Examples:
- `BenchmarkMergeSuiteHives_System/win2003-system/small` - 2MB hive, 10 ops
- `BenchmarkMergeSuiteHives_System/win2012-system/large` - 9MB hive, 500+ ops
- `BenchmarkMergeSuiteHives_Software/win2012-software/medium` - 34MB hive, 50 ops
- `BenchmarkMergeSuiteSequential/win2012-system` - Apply 5 sequential patches

#### Running Suite Benchmarks

**Fast development (skip production hives)**:
```bash
go test -short -bench=.
```

**Full benchmarking (includes production hives)**:
```bash
go test -bench=.
```

**Only suite benchmarks**:
```bash
go test -bench=BenchmarkMergeSuite
```

## Performance Testing Scenarios

1. **Varying delta sizes**: Test how merge performance scales with operation count
   - Small: 5-10 operations
   - Medium: 50-100 operations
   - Large: 500-1000 operations

2. **Varying hive sizes**: Test against different scales
   - Minimal: 8 KB (testdata/minimal)
   - Large: 436 KB (testdata/large)
   - Production System: 2-9 MB (testdata/suite/windows-*-system)
   - Production Software: 34 MB (testdata/suite/windows-2012-software)

3. **Sequential merges**: Simulate real-world scenarios of applying multiple patches
   - 20 sequential deltas for small/large hives
   - 5 sequential patches for production suite hives

4. **Operation types**: Separate benchmarks for add, modify, delete, and mixed operations

5. **Full hive rebuilds**: Test merging complete registry exports

6. **Top-level key distribution**: Suite benchmarks test merge performance across different registry tree branches (ControlSet001/002, Select, Setup, WPA, DriverDatabase, etc.)

## Metrics Tracked

- Time per operation (ns/op)
- Memory allocations (allocs/op)
- Bytes allocated (B/op)
- Throughput (MB/s) via `b.SetBytes()`

## Notes

- All deltas use UTF-8 encoding with Windows-style line endings
- Hex values are properly formatted per .reg file specification
- String values use UTF-16LE encoding (hex format)
- Delete operations use `-` for value deletion and `[-key]` for key deletion
