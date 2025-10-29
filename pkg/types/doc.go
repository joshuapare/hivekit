// Package hive defines a high-performance, Go-idiomatic API for reading,
// querying, editing, and writing Windows Registry hive ("regf") files.
//
// This package only exposes interfaces and core types. A separate internal
// implementation will provide mmap-backed, zero-copy parsing and a
// copy-on-write editor that emits consistent hives.
//
// Design goals:
//   - Zero-copy where safe; explicit copying where requested.
//   - Small, copyable handles (NodeID/ValueID) instead of large object graphs.
//   - Paranoid bounds checking; never panic on malformed input.
//   - Typed errors with stable categories (format/corrupt/unsupported/...).
//
// This package has no dependencies beyond the standard library.
package types
