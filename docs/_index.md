---
title: "HiveKit Documentation"
date: 2025-01-05
weight: 1
---

# HiveKit Documentation

HiveKit is a high-performance Go library for reading, writing, and modifying Windows Registry hive files.

## Documentation Sections

### [Architecture]({{< ref "/architecture" >}})
Core architectural patterns, design principles, and component interactions.

### [Components]({{< ref "/components" >}})
Detailed documentation for each major component of the system.

### [Testing]({{< ref "/testing" >}})
Testing strategies, coverage, and debugging techniques.

## Quick Links

- [Transaction Protocol]({{< ref "/architecture/transaction-protocol" >}})
- [Allocator Design]({{< ref "/components/allocator" >}})
- [Sequence Number Management]({{< ref "/architecture/sequence-numbers" >}})

## Overview

HiveKit implements the Windows Registry hive format specification, providing:

- **Zero-copy operations** via memory-mapped I/O
- **Transaction safety** with ACID guarantees
- **Spec-compliant growth** using 4KB page alignment
- **Efficient allocation** with segregated free lists
- **Index-based lookups** for fast key/value access

## Key Features

- ✅ Full Windows Registry hive format support (REG_SZ, REG_DWORD, REG_BINARY, etc.)
- ✅ Transaction management with ordered flush protocol
- ✅ Memory-mapped file I/O for performance
- ✅ Dirty page tracking for minimal write amplification
- ✅ Multiple merge strategies (InPlace, Append, Hybrid)
- ✅ Compatible with Windows tools (hivexsh, regedit)
