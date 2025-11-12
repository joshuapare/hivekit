// Package verify provides validation functions for Windows Registry hive structures.
//
// # Overview
//
// This package implements comprehensive validation checks for hive files to ensure
// structural integrity and adherence to the Windows Registry hive format specification.
// It is primarily used in tests to verify that modifications maintain hive invariants.
//
// Validation categories:
//   - REGF header: Signature, version, data size, alignment
//   - HBIN structure: Blocks, offsets, sizes, cell boundaries
//   - File size: Matches header data size field
//   - Sequence numbers: Transaction consistency (Seq1 == Seq2)
//   - Checksum: REGF header integrity
//
// # Quick Start
//
// Validate all invariants in one call:
//
//	data, _ := os.ReadFile("system.hive")
//	if err := verify.AllInvariants(data); err != nil {
//	    fmt.Printf("Validation failed: %v\n", err)
//	}
//
// Validate specific aspects:
//
//	if err := verify.REGFHeader(data); err != nil {
//	    fmt.Printf("Header invalid: %v\n", err)
//	}
//
//	if err := verify.HBINStructure(data); err != nil {
//	    fmt.Printf("HBIN structure invalid: %v\n", err)
//	}
//
//	if err := verify.SequenceNumbers(data); err != nil {
//	    fmt.Printf("Warning: dirty hive (incomplete transaction)\n")
//	}
//
// # ValidationError
//
// All validation functions return ValidationError on failure:
//
//	type ValidationError struct {
//	    Type    string                 // Error category (e.g., "REGFHeader")
//	    Message string                 // Human-readable description
//	    Offset  int                    // File offset where error occurred (-1 if N/A)
//	    Details map[string]interface{} // Additional context
//	}
//
// Example:
//
//	err := verify.Checksum(data)
//	if err != nil {
//	    if verr, ok := err.(*verify.ValidationError); ok {
//	        fmt.Printf("Type: %s\n", verr.Type)
//	        fmt.Printf("Offset: 0x%X\n", verr.Offset)
//	        fmt.Printf("Message: %s\n", verr.Message)
//	        if verr.Details != nil {
//	            fmt.Printf("Calculated: 0x%08X\n", verr.Details["calculated"])
//	            fmt.Printf("Stored: 0x%08X\n", verr.Details["stored"])
//	        }
//	    }
//	}
//
// # REGF Header Validation
//
// REGFHeader checks the base block (first 4KB):
//
//	err := verify.REGFHeader(data)
//
// Validates:
//   - File size ≥ 4096 bytes
//   - Signature is "regf" (0x72656766)
//   - Major version == 1
//   - Minor version in range 3-6 (typical values)
//   - Data size is 4KB-aligned (multiple of 0x1000)
//
// Example errors:
//   - "invalid signature: got \"regx\", expected \"regf\""
//   - "unexpected major version: 2 (expected 1)"
//   - "data size not 4KB-aligned: 0x1234"
//
// # HBIN Structure Validation
//
// HBINStructure validates all HBIN blocks:
//
//	err := verify.HBINStructure(data)
//
// Validates:
//   - Each HBIN has valid signature "hbin" (0x6862696E)
//   - Offset field matches actual position
//   - Size is positive and 4KB-aligned
//   - HBIN doesn't exceed file size
//   - Cells within HBIN:
//   - Don't cross HBIN boundaries
//   - Have 8-byte aligned sizes
//   - Cell headers are valid
//
// Example errors:
//   - "invalid HBIN signature: got \"hbix\", expected \"hbin\""
//   - "HBIN offset mismatch: field=0x5000, expected=0x4000"
//   - "cell crosses HBIN boundary: cell_end=0x9000, hbin_end=0x8000"
//   - "cell size not 8-byte aligned: 13 bytes"
//
// # File Size Validation
//
// FileSize checks that file size matches header:
//
//	err := verify.FileSize(data)
//
// Validates:
//   - File size = 4096 + data_size (from REGF header)
//   - No truncation or extra data
//
// Example error:
//
//	err := verify.FileSize(data)
//	if verr, ok := err.(*verify.ValidationError); ok {
//	    fmt.Printf("File size mismatch:\n")
//	    fmt.Printf("  Actual: 0x%X\n", verr.Details["actual"])
//	    fmt.Printf("  Expected: 0x%X\n", verr.Details["expected"])
//	    fmt.Printf("  Header: 0x%X\n", verr.Details["header_size"])
//	    fmt.Printf("  Data: 0x%X\n", verr.Details["data_size"])
//	}
//
// # Sequence Number Validation
//
// SequenceNumbers checks transaction consistency:
//
//	err := verify.SequenceNumbers(data)
//	if err != nil {
//	    // Hive is dirty (incomplete transaction)
//	}
//
// Validates:
//   - PrimarySeq (offset 0x04) == SecondarySeq (offset 0x08)
//
// Interpretation:
//   - Seq1 == Seq2: Clean hive, no incomplete transactions
//   - Seq1 != Seq2: Dirty hive, transaction was not committed
//
// Example:
//
//	err := verify.SequenceNumbers(data)
//	if err != nil {
//	    if verr, ok := err.(*verify.ValidationError); ok {
//	        primary := verr.Details["primary"].(uint32)
//	        secondary := verr.Details["secondary"].(uint32)
//	        fmt.Printf("Warning: Incomplete transaction\n")
//	        fmt.Printf("  Primary: %d\n", primary)
//	        fmt.Printf("  Secondary: %d\n", secondary)
//	    }
//	}
//
// # Checksum Validation
//
// Checksum validates REGF header integrity:
//
//	err := verify.Checksum(data)
//
// Algorithm:
//   - XOR first 508 bytes (127 dwords)
//   - Compare with checksum field at offset 0x1FC
//
// Example:
//
//	err := verify.Checksum(data)
//	if err != nil {
//	    if verr, ok := err.(*verify.ValidationError); ok {
//	        fmt.Printf("Checksum mismatch:\n")
//	        fmt.Printf("  Calculated: 0x%08X\n", verr.Details["calculated"])
//	        fmt.Printf("  Stored: 0x%08X\n", verr.Details["stored"])
//	    }
//	}
//
// # AllInvariants
//
// AllInvariants runs all core validations:
//
//	err := verify.AllInvariants(data)
//
// Checks performed (in order):
//  1. REGFHeader
//  2. HBINStructure
//  3. FileSize
//
// Returns first error encountered, or nil if all pass.
//
// Note: Does NOT check SequenceNumbers or Checksum (those are warnings, not errors).
//
// # Usage in Tests
//
// Typical test pattern:
//
//	func TestHiveModification(t *testing.T) {
//	    // Load hive
//	    data, _ := os.ReadFile("test.hive")
//
//	    // Verify initial state
//	    if err := verify.AllInvariants(data); err != nil {
//	        t.Fatalf("Initial hive invalid: %v", err)
//	    }
//
//	    // Perform modifications
//	    h, _ := hive.Open("test.hive")
//	    // ... modify hive ...
//	    h.Close()
//
//	    // Re-load and verify
//	    data, _ = os.ReadFile("test.hive")
//	    if err := verify.AllInvariants(data); err != nil {
//	        t.Fatalf("Modified hive invalid: %v", err)
//	    }
//
//	    // Check transaction was committed
//	    if err := verify.SequenceNumbers(data); err != nil {
//	        t.Errorf("Transaction not committed: %v", err)
//	    }
//
//	    // Check checksum
//	    if err := verify.Checksum(data); err != nil {
//	        t.Errorf("Checksum invalid: %v", err)
//	    }
//	}
//
// # Cell Validation
//
// validateHBINCells (internal) checks cells within an HBIN:
//
// Validates:
//   - Cell headers are within HBIN bounds
//   - Cell sizes are reasonable (> 4 bytes)
//   - Cells don't cross HBIN boundaries
//   - Cell sizes are 8-byte aligned
//
// Cell structure:
//
//	[Size: 4 bytes, signed int32]
//	[Payload: (abs(size) - 4) bytes]
//
// Negative size = allocated cell
// Positive size = free cell
//
// # Validation Levels
//
// Conservative (fails on any issue):
//   - Use AllInvariants() for strict validation
//   - Rejects any structural problems
//
// Permissive (warnings only):
//   - Check SequenceNumbers separately (dirty hive is acceptable)
//   - Check Checksum separately (may be outdated)
//
// Example:
//
//	// Strict: reject any issues
//	if err := verify.AllInvariants(data); err != nil {
//	    return fmt.Errorf("hive corrupted: %w", err)
//	}
//
//	// Permissive: warn but continue
//	if err := verify.SequenceNumbers(data); err != nil {
//	    log.Printf("Warning: %v", err)
//	}
//	if err := verify.Checksum(data); err != nil {
//	    log.Printf("Warning: %v", err)
//	}
//
// # Performance Characteristics
//
// Validation costs:
//   - REGFHeader: O(1), ~1μs (reads header fields)
//   - HBINStructure: O(n), ~100μs per MB (scans all HBINs and cells)
//   - FileSize: O(1), ~1μs (reads header field)
//   - SequenceNumbers: O(1), ~1μs (reads two uint32s)
//   - Checksum: O(1), ~5μs (XOR 508 bytes)
//   - AllInvariants: O(n), ~100μs per MB
//
// Typical hive (10MB):
//   - AllInvariants: ~1ms
//   - Full validation (including Seq + Checksum): ~1.1ms
//
// # Error Recovery
//
// When validation fails:
//
// Corrupted header:
//   - Cannot recover automatically
//   - May require manual hex editing
//   - Check backups or shadow copies
//
// HBIN structure issues:
//   - May be recoverable with partial read
//   - Skip corrupted HBINs, read valid ones
//   - Use specialized recovery tools
//
// Sequence mismatch:
//   - Recoverable: set SecondarySeq = PrimarySeq
//   - Requires write access and transaction
//   - Loses incomplete transaction data
//
// Checksum mismatch:
//   - Recoverable: recalculate and update
//   - Non-critical (hive may still be usable)
//   - Update with tx.Manager during next commit
//
// # Limitations
//
// The verify package does NOT check:
//   - Cell payload contents (NK, VK, etc. signatures)
//   - Reference validity (NK subkey lists, VK data offsets)
//   - Subkey list hash correctness
//   - Value data integrity
//   - Big-data (DB) structure validity
//   - Root cell reachability
//
// For deeper validation, use specialized tools or implement custom checks.
//
// # Integration with Other Packages
//
// The verify package is typically used:
//   - In tests after hive modifications
//   - Before accepting user-provided hive files
//   - In diagnostic/repair tools
//   - For regression testing
//
// Example integration:
//
//	// After editing
//	editor.EnsureKeyPath(...)
//	session.Commit()
//
//	// Verify invariants maintained
//	data := hive.Bytes()
//	if err := verify.AllInvariants(data); err != nil {
//	    t.Fatalf("Invariants violated: %v", err)
//	}
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive parsing and structures
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package verify
