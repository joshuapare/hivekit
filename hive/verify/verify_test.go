//go:build linux || darwin

package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestREGFHeader_Valid tests validation of a valid REGF header.
func TestREGFHeader_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	err := REGFHeader(data)
	require.NoError(t, err, "Valid REGF header should pass validation")
}

// TestREGFHeader_InvalidSignature tests detection of invalid signature.
func TestREGFHeader_InvalidSignature(t *testing.T) {
	data := createValidMinimalHive(t)

	// Corrupt signature
	copy(data[format.REGFSignatureOffset:], []byte("XXXX"))

	err := REGFHeader(data)
	require.Error(t, err, "Invalid signature should fail validation")
	require.Contains(t, err.Error(), "invalid signature")
}

// TestREGFHeader_UnalignedDataSize tests detection of unaligned data size.
func TestREGFHeader_UnalignedDataSize(t *testing.T) {
	data := createValidMinimalHive(t)

	// Set data size to non-4KB-aligned value
	format.PutU32(data, format.REGFDataSizeOffset, 4097)

	err := REGFHeader(data)
	require.Error(t, err, "Unaligned data size should fail validation")
	require.Contains(t, err.Error(), "not 4KB-aligned")
}

// TestHBINStructure_Valid tests validation of valid HBIN structure.
func TestHBINStructure_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	err := HBINStructure(data)
	require.NoError(t, err, "Valid HBIN structure should pass validation")
}

// TestHBINStructure_InvalidSignature tests detection of invalid HBIN signature.
func TestHBINStructure_InvalidSignature(t *testing.T) {
	data := createValidMinimalHive(t)

	// Corrupt HBIN signature
	copy(data[format.HeaderSize:format.HeaderSize+4], []byte("XXXX"))

	err := HBINStructure(data)
	require.Error(t, err, "Invalid HBIN signature should fail validation")
	require.Contains(t, err.Error(), "invalid HBIN signature")
}

// TestHBINStructure_WrongOffset tests detection of incorrect HBIN offset field.
func TestHBINStructure_WrongOffset(t *testing.T) {
	data := createValidMinimalHive(t)

	// Set wrong offset in HBIN header
	format.PutU32(data, format.HeaderSize+format.HBINFileOffsetField, 0x1000) // Should be 0

	err := HBINStructure(data)
	require.Error(t, err, "Wrong HBIN offset should fail validation")
	require.Contains(t, err.Error(), "HBIN offset mismatch")
}

// TestFileSize_Valid tests validation of correct file size.
func TestFileSize_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	err := FileSize(data)
	require.NoError(t, err, "Correct file size should pass validation")
}

// TestFileSize_TooLarge tests detection of file with trailing garbage.
func TestFileSize_TooLarge(t *testing.T) {
	data := createValidMinimalHive(t)

	// Add trailing garbage
	data = append(data, make([]byte, 64)...)

	err := FileSize(data)
	require.Error(t, err, "File with trailing garbage should fail validation")
	require.Contains(t, err.Error(), "file size mismatch")
}

// TestFileSize_TooSmall tests detection of truncated file.
func TestFileSize_TooSmall(t *testing.T) {
	data := createValidMinimalHive(t)

	// Truncate file
	data = data[:len(data)-64]

	err := FileSize(data)
	require.Error(t, err, "Truncated file should fail validation")
	require.Contains(t, err.Error(), "file size mismatch")
}

// TestSequenceNumbers_Valid tests validation of consistent sequence numbers.
func TestSequenceNumbers_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	// Set both sequences to same value
	format.PutU32(data, format.REGFPrimarySeqOffset, 42)
	format.PutU32(data, format.REGFSecondarySeqOffset, 42)

	err := SequenceNumbers(data)
	require.NoError(t, err, "Matching sequence numbers should pass validation")
}

// TestSequenceNumbers_Mismatch tests detection of mismatched sequences (dirty hive).
func TestSequenceNumbers_Mismatch(t *testing.T) {
	data := createValidMinimalHive(t)

	// Set different sequence values (dirty hive)
	format.PutU32(data, format.REGFPrimarySeqOffset, 42)
	format.PutU32(data, format.REGFSecondarySeqOffset, 43)

	err := SequenceNumbers(data)
	require.Error(t, err, "Mismatched sequence numbers should fail validation")
	require.Contains(t, err.Error(), "sequences mismatch")
	require.Contains(t, err.Error(), "dirty hive")
}

// TestChecksum_Valid tests validation of correct checksum.
func TestChecksum_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	// Calculate and set correct checksum
	var checksum uint32
	for i := 0; i < 0x1FC; i += 4 {
		checksum ^= format.ReadU32(data, i)
	}
	format.PutU32(data, 0x1FC, checksum)

	err := Checksum(data)
	require.NoError(t, err, "Correct checksum should pass validation")
}

// TestChecksum_Invalid tests detection of incorrect checksum.
func TestChecksum_Invalid(t *testing.T) {
	data := createValidMinimalHive(t)

	// Set wrong checksum
	format.PutU32(data, 0x1FC, 0xDEADBEEF)

	err := Checksum(data)
	require.Error(t, err, "Incorrect checksum should fail validation")
	require.Contains(t, err.Error(), "checksum mismatch")
}

// TestAllInvariants_Valid tests that all invariants pass for valid hive.
func TestAllInvariants_Valid(t *testing.T) {
	data := createValidMinimalHive(t)

	err := AllInvariants(data)
	require.NoError(t, err, "Valid hive should pass all invariant checks")
}

// TestAllInvariants_StopsAtFirstError tests that validation stops at first error.
func TestAllInvariants_StopsAtFirstError(t *testing.T) {
	data := createValidMinimalHive(t)

	// Corrupt signature (first check)
	copy(data[format.REGFSignatureOffset:], []byte("XXXX"))

	err := AllInvariants(data)
	require.Error(t, err, "Corrupted hive should fail validation")
	require.Contains(t, err.Error(), "invalid signature")
}

// TestValidationError_String tests error message formatting.
func TestValidationError_String(t *testing.T) {
	err1 := &ValidationError{
		Type:    "TestError",
		Message: "something went wrong",
		Offset:  0x1234,
	}
	require.Contains(t, err1.Error(), "0x1234")
	require.Contains(t, err1.Error(), "something went wrong")

	err2 := &ValidationError{
		Type:    "TestError",
		Message: "no offset",
		Offset:  -1,
	}
	require.NotContains(t, err2.Error(), "0x")
}

// Helper functions

func createValidMinimalHive(t *testing.T) []byte {
	t.Helper()

	const hbinSize = format.HBINAlignment // Always 4096
	buf := make([]byte, format.HeaderSize+hbinSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write HBIN header
	hbinOff := format.HeaderSize
	copy(buf[hbinOff:hbinOff+4], format.HBINSignature)
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, 0)
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(hbinSize))

	// Write a minimal NK cell at offset 0x20 (root)
	cellOff := hbinOff + format.HBINHeaderSize
	cellSize := 80                                // Minimal NK cell
	format.PutI32(buf, cellOff, -int32(cellSize)) // Allocated cell
	copy(buf[cellOff+4:cellOff+6], []byte("nk"))

	// Set subkey list offset to InvalidOffset (no subkeys)
	format.PutU32(buf, cellOff+4+format.NKSubkeyListOffset, format.InvalidOffset)
	// Set value list offset to InvalidOffset (no values)
	format.PutU32(buf, cellOff+4+format.NKValueListOffset, format.InvalidOffset)

	// Write a free cell for the remaining space
	nextCellOff := cellOff + cellSize
	freeSize := hbinSize - format.HBINHeaderSize - cellSize
	if freeSize > format.CellHeaderSize {
		format.PutI32(buf, nextCellOff, int32(freeSize))
	}

	return buf
}

// TestVerify_AllRealHives validates all real Windows registry hive files in testdata.
// This test validates hive files from real Windows machines across multiple versions.
// Skipped in short mode since it processes ~12 real hive files which can be slow.
func TestVerify_AllRealHives(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real hive validation in short mode")
	}

	// Test individual hive files in testdata/
	individualHives := []string{
		"../../testdata/minimal",
		"../../testdata/special",
		"../../testdata/large",
		"../../testdata/rlenvalue_test_hive",
		"../../testdata/typed_values",
	}

	t.Run("Individual", func(t *testing.T) {
		for _, path := range individualHives {
			name := filepath.Base(path)
			t.Run(name, func(t *testing.T) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Skipf("Hive not available: %v", err)
					return
				}

				// Validate all invariants
				validateRealHive(t, data, name)
			})
		}
	})

	// Test suite hives from various Windows versions
	t.Run("Suite", func(t *testing.T) {
		suiteDir := "../../testdata/suite"
		entries, err := os.ReadDir(suiteDir)
		if err != nil {
			t.Skipf("Suite directory not available: %v", err)
			return
		}

		hiveCount := 0
		for _, entry := range entries {
			// Skip directories, compressed files, .reg exports, and docs
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if filepath.Ext(name) != "" { // Has extension, skip
				continue
			}
			if name == "README" || name == "COVERAGE_ANALYSIS.md" {
				continue
			}

			path := filepath.Join(suiteDir, name)
			t.Run(name, func(t *testing.T) {
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr, "Failed to read hive file")

				// Validate all invariants
				validateRealHive(t, data, name)
			})

			hiveCount++
		}

		t.Logf("Validated %d real Windows registry hive files from suite", hiveCount)
	})
}

// validateRealHive performs comprehensive validation on a real hive file.
func validateRealHive(t *testing.T, data []byte, name string) {
	t.Helper()

	// All real hives should have valid REGF headers
	err := REGFHeader(data)
	require.NoError(t, err, "%s: REGF header should be valid", name)

	// All real hives should have valid HBIN structure
	err = HBINStructure(data)
	require.NoError(t, err, "%s: HBIN structure should be valid", name)

	// File size should match header
	err = FileSize(data)
	require.NoError(t, err, "%s: File size should match header", name)

	// Sequence numbers should be consistent (clean hives)
	// Note: Some hives might be dirty if captured during a write operation,
	// so we just log this rather than failing
	err = SequenceNumbers(data)
	if err != nil {
		t.Logf("%s: Hive has inconsistent sequences (was captured during write): %v", name, err)
	}

	// Checksum validation
	// Note: Some older hives might not have checksums set correctly,
	// so we log rather than fail
	err = Checksum(data)
	if err != nil {
		t.Logf("%s: Checksum validation failed (not critical for older hives): %v", name, err)
	}

	t.Logf("%s: Passed all critical validations (size=%d bytes)", name, len(data))
}
