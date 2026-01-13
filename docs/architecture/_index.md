---
title: "Architecture"
weight: 10
---

# HiveKit Architecture

This section documents the core architectural patterns and design principles of HiveKit.

## Core Principles

### 1. Separation of Concerns

HiveKit maintains clear boundaries between different responsibilities:

- **Structural Management** (Allocator): HBINs, cells, data layout
- **Protocol Management** (tx.Manager): Sequences, timestamps, transaction lifecycle
- **Metadata Management** (Index): Key/value lookups, hierarchy navigation
- **Strategy Management** (merge.Strategy): Operation execution patterns

### 2. Zero-Copy Operations

Memory-mapped I/O enables direct manipulation of hive data without intermediate buffers:

```go
// Unix/Darwin: mmap-backed
data, err := syscall.Mmap(fd, 0, size, PROT_READ|PROT_WRITE, MAP_SHARED)

// Other platforms: in-memory slice
data := make([]byte, size)
io.ReadFull(f, data)
```

### 3. Minimal Write Amplification

Dirty page tracking ensures only modified regions are flushed to disk:

```go
dt.Add(offset, length)  // Mark region dirty
dt.FlushDataOnly()      // Flush only dirty pages
```

## Component Interaction

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                       │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┼─────────────────────────────────┐
│                     merge.Session                            │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  tx.Manager  │  Strategy  │  Index  │  Allocator   │    │
│  └─────────────────────────────────────────────────────┘    │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┼─────────────────────────────────┐
│                       hive.Hive                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Memory-mapped data  │  File handle  │  Base block  │   │
│  └──────────────────────────────────────────────────────┘   │
└───────────────────────────┬─────────────────────────────────┘
                            │
                    ┌───────┴────────┐
                    │                │
            ┌───────▼──────┐   ┌────▼─────┐
            │  OS mmap     │   │  OS I/O  │
            └──────────────┘   └──────────┘
```

## Design Documents

- [Transaction Protocol]({{< ref "transaction-protocol" >}}) - ACID guarantees via sequence numbers
- [Sequence Number Management]({{< ref "sequence-numbers" >}}) - Protocol vs structural field separation
- [Memory Management]({{< ref "memory-management" >}}) - mmap vs slice-based approaches
- [Merge Strategies]({{< ref "merge-strategies" >}}) - InPlace vs Append vs Hybrid patterns
