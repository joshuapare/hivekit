package hive_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestDiagnosticFormatters tests all output formatters.
func TestDiagnosticFormatters(t *testing.T) {
	// Create a sample diagnostic report
	report := hive.NewDiagnosticReport()
	report.FilePath = "test.hive"
	report.FileSize = 8192

	// Add various diagnostics
	report.Add(hive.Diagnostic{
		Severity:  hive.SevCritical,
		Category:  hive.DiagStructure,
		Offset:    0x1000,
		Structure: "HBIN",
		Issue:     "HBIN signature invalid",
		Expected:  "hbin",
		Actual:    "XXXX",
		Repair: &hive.RepairAction{
			Type:        hive.RepairReplace,
			Description: "Replace with correct signature",
			Confidence:  1.0,
			Risk:        hive.RiskMedium,
			AutoApply:   false,
		},
	})

	report.Add(hive.Diagnostic{
		Severity:  hive.SevError,
		Category:  hive.DiagData,
		Offset:    0x2000,
		Structure: "VK",
		Issue:     "Value data truncated",
		Expected:  100,
		Actual:    50,
		Context: &hive.DiagContext{
			KeyPath:   `HKLM\Software\Test`,
			ValueName: "TestValue",
		},
		Repair: &hive.RepairAction{
			Type:        hive.RepairTruncate,
			Description: "Update length field to match actual data",
			Confidence:  0.95,
			Risk:        hive.RiskLow,
			AutoApply:   true,
		},
	})

	report.Add(hive.Diagnostic{
		Severity:  hive.SevWarning,
		Category:  hive.DiagIntegrity,
		Offset:    0x3000,
		Structure: "NK",
		Issue:     "Orphaned cell not referenced by tree",
	})

	report.Finalize()

	t.Run("JSON", func(t *testing.T) {
		jsonStr, err := report.FormatJSON()
		require.NoError(t, err, "FormatJSON should not error")
		assert.NotEmpty(t, jsonStr, "JSON output should not be empty")

		// Verify it's valid JSON
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err, "JSON should be valid")

		// Verify key fields are present
		assert.Contains(t, jsonStr, `"file_path"`)
		assert.Contains(t, jsonStr, `"file_size"`)
		assert.Contains(t, jsonStr, `"summary"`)
		assert.Contains(t, jsonStr, `"diagnostics"`)
		assert.Contains(t, jsonStr, `"critical": 1`)
		assert.Contains(t, jsonStr, `"errors": 1`)
		assert.Contains(t, jsonStr, `"warnings": 1`)

		t.Logf("JSON output (%d bytes):\n%s", len(jsonStr), jsonStr)
	})

	t.Run("Text", func(t *testing.T) {
		text := report.FormatText()
		assert.NotEmpty(t, text, "Text output should not be empty")

		// Verify key sections are present
		assert.Contains(t, text, "Registry Hive Diagnostic Report")
		assert.Contains(t, text, "SUMMARY")
		assert.Contains(t, text, "DIAGNOSTICS")
		assert.Contains(t, text, "test.hive")
		assert.Contains(t, text, "8192 bytes")

		// Verify summary numbers
		assert.Contains(t, text, "Critical: 1")
		assert.Contains(t, text, "Errors:   1")
		assert.Contains(t, text, "Warnings: 1")

		// Verify diagnostic details
		assert.Contains(t, text, "HBIN signature invalid")
		assert.Contains(t, text, "Value data truncated")
		assert.Contains(t, text, "Orphaned cell")

		// Verify repair info
		assert.Contains(t, text, "Repair:")
		assert.Contains(t, text, "Risk:")
		assert.Contains(t, text, "Auto-apply: YES")

		// Verify context info
		assert.Contains(t, text, `HKLM\Software\Test`)
		assert.Contains(t, text, "TestValue")

		t.Logf("Text output (%d bytes):\n%s", len(text), text)
	})

	t.Run("TextCompact", func(t *testing.T) {
		compact := report.FormatTextCompact()
		assert.NotEmpty(t, compact, "Compact text output should not be empty")

		// Should have one line per diagnostic (sorted by offset)
		lines := strings.Split(strings.TrimSpace(compact), "\n")
		assert.Len(t, lines, 3, "Should have 3 lines for 3 diagnostics")

		// Verify format: offset [severity/structure/category] issue
		assert.Contains(t, lines[0], "0x00001000")
		assert.Contains(t, lines[0], "CRITICAL")
		assert.Contains(t, lines[0], "HBIN")
		assert.Contains(t, lines[0], "STRUCTURE")

		assert.Contains(t, lines[1], "0x00002000")
		assert.Contains(t, lines[1], "ERROR")
		assert.Contains(t, lines[1], "VK")
		assert.Contains(t, lines[1], "DATA")

		assert.Contains(t, lines[2], "0x00003000")
		assert.Contains(t, lines[2], "WARNING")
		assert.Contains(t, lines[2], "NK")
		assert.Contains(t, lines[2], "INTEGRITY")

		t.Logf("Compact output:\n%s", compact)
	})

	t.Run("HexAnnotations", func(t *testing.T) {
		annotations := report.FormatHexAnnotations()
		assert.NotEmpty(t, annotations, "Hex annotations should not be empty")

		// Verify header
		assert.Contains(t, annotations, "# Hex dump annotations")
		assert.Contains(t, annotations, "# Format: offset,severity,structure,message")

		// Verify annotations
		lines := strings.Split(strings.TrimSpace(annotations), "\n")
		dataLines := 0
		for _, line := range lines {
			if !strings.HasPrefix(line, "#") && line != "" {
				dataLines++
				// Each line should be CSV format
				parts := strings.Split(line, ",")
				assert.GreaterOrEqual(t, len(parts), 4, "Annotation should have at least 4 fields")
				assert.True(t, strings.HasPrefix(parts[0], "0x"), "First field should be hex offset")
			}
		}
		assert.Equal(t, 3, dataLines, "Should have 3 annotation lines")

		t.Logf("Hex annotations:\n%s", annotations)
	})
}

// TestDiagnosticFormatters_Empty tests formatters with empty report.
func TestDiagnosticFormatters_Empty(t *testing.T) {
	report := hive.NewDiagnosticReport()
	report.FileSize = 4096
	report.Finalize()

	t.Run("JSON_Empty", func(t *testing.T) {
		jsonStr, err := report.FormatJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonStr)

		// Should have empty diagnostics (null or [])
		assert.True(t,
			strings.Contains(jsonStr, `"diagnostics": []`) || strings.Contains(jsonStr, `"diagnostics": null`),
			"Diagnostics should be empty array or null")
		assert.Contains(t, jsonStr, `"critical": 0`)
	})

	t.Run("Text_Empty", func(t *testing.T) {
		text := report.FormatText()
		assert.Contains(t, text, "No issues found")
	})

	t.Run("TextCompact_Empty", func(t *testing.T) {
		compact := report.FormatTextCompact()
		assert.Contains(t, compact, "No issues found")
	})

	t.Run("HexAnnotations_Empty", func(t *testing.T) {
		annotations := report.FormatHexAnnotations()
		assert.Contains(t, annotations, "# Hex dump annotations")
		// Should only have header, no data lines
		lines := strings.Split(strings.TrimSpace(annotations), "\n")
		dataLines := 0
		for _, line := range lines {
			if !strings.HasPrefix(line, "#") && line != "" {
				dataLines++
			}
		}
		assert.Equal(t, 0, dataLines, "Empty report should have no annotation data lines")
	})
}

// TestDiagnosticJSON_Roundtrip tests JSON marshaling/unmarshaling.
func TestDiagnosticJSON_Roundtrip(t *testing.T) {
	original := hive.NewDiagnosticReport()
	original.FilePath = "roundtrip.hive"
	original.FileSize = 12345

	original.Add(hive.Diagnostic{
		Severity:  hive.SevError,
		Category:  hive.DiagData,
		Offset:    0x1234,
		Structure: "NK",
		Issue:     "Test issue",
		Expected:  "expected",
		Actual:    "actual",
	})

	original.Finalize()

	// Marshal to JSON
	jsonStr, err := original.FormatJSON()
	require.NoError(t, err)

	// Unmarshal back
	var restored hive.DiagnosticReport
	err = json.Unmarshal([]byte(jsonStr), &restored)
	require.NoError(t, err)

	// Verify key fields match
	assert.Equal(t, original.FilePath, restored.FilePath)
	assert.Equal(t, original.FileSize, restored.FileSize)
	assert.Equal(t, original.Summary.Errors, restored.Summary.Errors)
	assert.Len(t, restored.Diagnostics, 1)
	assert.Equal(t, original.Diagnostics[0].Severity, restored.Diagnostics[0].Severity)
	assert.Equal(t, original.Diagnostics[0].Offset, restored.Diagnostics[0].Offset)
	assert.Equal(t, original.Diagnostics[0].Issue, restored.Diagnostics[0].Issue)
}

// TestDiagnosticFormatters_SpecialCharacters tests handling of special characters.
func TestDiagnosticFormatters_SpecialCharacters(t *testing.T) {
	report := hive.NewDiagnosticReport()

	// Add diagnostic with special characters
	report.Add(hive.Diagnostic{
		Severity:  hive.SevError,
		Category:  hive.DiagData,
		Offset:    0x1000,
		Structure: "NK",
		Issue:     `Issue with "quotes", commas, and\nnewlines`,
		Context: &hive.DiagContext{
			KeyPath: `HKLM\Software\Company\Product`,
		},
	})

	report.Finalize()

	t.Run("JSON_SpecialChars", func(t *testing.T) {
		jsonStr, err := report.FormatJSON()
		require.NoError(t, err)

		// Should be valid JSON despite special characters
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err, "JSON with special characters should be valid")
	})

	t.Run("Text_SpecialChars", func(t *testing.T) {
		text := report.FormatText()
		// Should contain the issue (may be escaped)
		assert.Contains(t, text, "quotes")
		assert.Contains(t, text, "commas")
	})

	t.Run("HexAnnotations_SpecialChars", func(t *testing.T) {
		annotations := report.FormatHexAnnotations()
		// Commas should be escaped with semicolons
		assert.NotContains(t, strings.Split(annotations, "\n")[2], `"quotes", commas`)
		// Should have semicolon instead
		assert.Contains(t, annotations, "quotes")
	})
}
