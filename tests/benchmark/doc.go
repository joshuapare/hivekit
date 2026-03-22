// Package benchmark provides fixture generators and utilities for benchmarking
// hivekit's merge performance. It programmatically generates Windows Registry
// hive files of various sizes and structures using hivekit's builder API.
//
// The generated fixtures serve as the foundation for all merge performance
// benchmarks, ensuring reproducible and representative test data.
//
// Fixture sizes range from ~5MB (small) to ~500MB (large-realistic), covering
// flat, deep, wide, and mixed tree structures that mirror real-world Windows
// Registry hives.
//
// All generators use deterministic random seeds for reproducibility.
package benchmark
