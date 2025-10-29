# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - TBD

### Added
- Initial release of hivekit
- `hivectl` - Command-line tool for Windows Registry hive operations
  - Read, write, modify, and analyze registry hives
  - Export to .reg format
  - Merge .reg files into hives
  - Diff comparison between hives
  - Validation and diagnostics
- `hiveexplorer` - Interactive TUI for exploring registry hive files
  - Split-pane layout with tree view and value table
  - Keyboard navigation with vim-style keys
  - Search functionality
  - Bookmark support
  - Diff mode
- Pure Go implementation with no external dependencies
- Transaction-based editing with copy-on-write semantics
- Full support for all list types (LF, LH, LI, RI) including indirect subkey lists
- Multi-HBIN support with proper boundary handling
- Unicode and special character support
- Differential analysis and merging
- Comprehensive benchmarks showing 5.28x average speedup over hivex
- C bindings for libguestfs hivex integration
- Validated against 146MB of real Windows registry data

[Unreleased]: https://github.com/joshuapare/hivekit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/joshuapare/hivekit/releases/tag/v0.1.0
