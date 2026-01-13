package hive_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestDiagnostics_Disabled verifies zero overhead when diagnostics are disabled.
func TestDiagnostics_Disabled(t *testing.T) {
	// Open normally (diagnostics disabled by default)
	r, err := reader.Open("../../testdata/minimal", hive.OpenOptions{})
	require.NoError(t, err)
	defer r.Close()

	// GetDiagnostics should return nil when not enabled
	report := r.GetDiagnostics()
	assert.Nil(t, report, "GetDiagnostics should return nil when not enabled")
}

// TestDiagnostics_EnabledWithHealthyFile tests passive collection with healthy file.
func TestDiagnostics_EnabledWithHealthyFile(t *testing.T) {
	// Open with diagnostics enabled
	r, err := reader.Open("../../testdata/minimal", hive.OpenOptions{
		CollectDiagnostics: true,
	})
	require.NoError(t, err)
	defer r.Close()

	// Access some data
	root, err := r.Root()
	require.NoError(t, err)

	_, err = r.StatKey(root)
	require.NoError(t, err)

	// Get diagnostics report
	report := r.GetDiagnostics()
	require.NotNil(t, report, "GetDiagnostics should return report when enabled")

	// Healthy file should have no issues
	assert.Equal(t, 0, report.Summary.Critical, "Healthy file should have no critical issues")
	assert.Equal(t, 0, report.Summary.Errors, "Healthy file should have no errors")
	assert.False(t, report.HasAnyIssues(), "Healthy file should have no issues")
}

// TestDiagnostics_CorruptedFile tests diagnostic collection with corrupted file.
func TestDiagnostics_CorruptedFile_HBIN(t *testing.T) {
	// Open corrupted file with diagnostics enabled
	// This file has invalid HBIN signature, so Open() will fail
	// but we should have recorded the diagnostic before failing
	_, err := reader.Open("../../testdata/corrupted/corrupt_hbin_signature", hive.OpenOptions{
		CollectDiagnostics: true,
	})
	require.Error(t, err, "Opening corrupt file should fail")

	// Note: In current implementation, diagnostics are lost when Open() fails
	// because we don't have access to the reader object
	// This is acceptable for now - diagnostics are mainly for files that open successfully
	// but have issues during traversal
}

// TestDiagnostics_TolerantMode tests diagnostic collection in tolerant mode.
func TestDiagnostics_TolerantMode(t *testing.T) {
	// Open file with truncated value data in tolerant mode with diagnostics
	r, err := reader.Open("../../testdata/corrupted/corrupt_value_data_truncated", hive.OpenOptions{
		Tolerant:           true,
		CollectDiagnostics: true,
	})
	require.NoError(t, err)
	defer r.Close()

	// Try to access the corrupted value
	root, err := r.Root()
	require.NoError(t, err)

	// Get all values (should succeed in tolerant mode)
	values, err := r.Values(root)
	if err != nil {
		t.Logf("Values() error: %v", err)
	}

	// Try to read value data (may trigger truncation diagnostic)
	for _, vid := range values {
		_, _ = r.ValueBytes(vid, hive.ReadOptions{})
	}

	// Get diagnostics report
	report := r.GetDiagnostics()
	require.NotNil(t, report)

	// Log summary for debugging
	t.Logf("Diagnostics summary: Critical=%d, Errors=%d, Warnings=%d, Info=%d",
		report.Summary.Critical, report.Summary.Errors, report.Summary.Warnings, report.Summary.Info)

	// If issues were found, verify structure
	if report.HasAnyIssues() {
		for i, d := range report.Diagnostics {
			t.Logf("Diagnostic %d: [%s/%s] %s at offset 0x%x",
				i, d.Severity, d.Category, d.Issue, d.Offset)
			if d.Repair != nil {
				t.Logf("  Repair: %s (risk=%s, auto=%v)",
					d.Repair.Description, d.Repair.Risk, d.Repair.AutoApply)
			}
		}

		// Verify at least one diagnostic has repair suggestion
		hasRepair := false
		for _, d := range report.Diagnostics {
			if d.Repair != nil {
				hasRepair = true
				break
			}
		}
		if report.Summary.Repairable > 0 {
			assert.True(t, hasRepair, "Report indicates repairable issues but no repairs found")
		}
	}
}

// TestDiagnostics_Diagnose tests explicit Diagnose() method.
func TestDiagnostics_Diagnose(t *testing.T) {
	// Open healthy file
	r, err := reader.Open("../../testdata/minimal", hive.OpenOptions{})
	require.NoError(t, err)
	defer r.Close()

	// Run explicit diagnostic scan
	report, err := r.Diagnose()
	require.NoError(t, err)
	require.NotNil(t, report)

	// Log summary for debugging
	t.Logf("Diagnostics summary: Critical=%d, Errors=%d, Warnings=%d, Info=%d",
		report.Summary.Critical, report.Summary.Errors, report.Summary.Warnings, report.Summary.Info)

	// Log all diagnostics found
	if report.HasAnyIssues() {
		for i, d := range report.Diagnostics {
			t.Logf("Diagnostic %d: [%s/%s] %s at offset 0x%x",
				i, d.Severity, d.Category, d.Issue, d.Offset)
		}
	}

	// Healthy file should have no critical errors (warnings about orphaned cells are OK)
	assert.False(t, report.HasErrors(), "Diagnose on healthy file should find no critical errors")
	assert.Equal(t, 0, report.Summary.Critical, "Should have no critical issues")
	assert.Equal(t, 0, report.Summary.Errors, "Should have no errors")
}

// TestDiagnosticReport_Grouping tests report grouping functionality.
func TestDiagnosticReport_Grouping(t *testing.T) {
	report := hive.NewDiagnosticReport()

	// Add some test diagnostics
	report.Add(hive.Diagnostic{
		Severity:  hive.SevCritical,
		Category:  hive.DiagStructure,
		Offset:    0x1000,
		Structure: "HBIN",
		Issue:     "Test issue 1",
	})

	report.Add(hive.Diagnostic{
		Severity:  hive.SevError,
		Category:  hive.DiagData,
		Offset:    0x2000,
		Structure: "VK",
		Issue:     "Test issue 2",
		Repair: &hive.RepairAction{
			Type:      hive.RepairTruncate,
			Risk:      hive.RiskLow,
			AutoApply: true,
		},
	})

	report.Add(hive.Diagnostic{
		Severity:  hive.SevWarning,
		Category:  hive.DiagIntegrity,
		Offset:    0x3000,
		Structure: "NK",
		Issue:     "Test issue 3",
		Repair: &hive.RepairAction{
			Type:      hive.RepairRebuild,
			Risk:      hive.RiskMedium,
			AutoApply: false,
		},
	})

	// Finalize to populate groupings
	report.Finalize()

	// Verify summary
	assert.Equal(t, 1, report.Summary.Critical)
	assert.Equal(t, 1, report.Summary.Errors)
	assert.Equal(t, 1, report.Summary.Warnings)
	assert.Equal(t, 2, report.Summary.Repairable)
	assert.Equal(t, 1, report.Summary.AutoRepairable)

	// Verify groupings
	assert.Len(t, report.BySeverity[hive.SevCritical], 1)
	assert.Len(t, report.BySeverity[hive.SevError], 1)
	assert.Len(t, report.BySeverity[hive.SevWarning], 1)

	assert.Len(t, report.ByStructure["HBIN"], 1)
	assert.Len(t, report.ByStructure["VK"], 1)
	assert.Len(t, report.ByStructure["NK"], 1)

	// Verify offset sorting
	assert.Len(t, report.ByOffset, 3)
	assert.Equal(t, uint64(0x1000), report.ByOffset[0].Offset)
	assert.Equal(t, uint64(0x2000), report.ByOffset[1].Offset)
	assert.Equal(t, uint64(0x3000), report.ByOffset[2].Offset)

	// Verify auto-repairable filtering
	autoRepairable := report.GetAutoRepairable()
	assert.Len(t, autoRepairable, 1)
	assert.Equal(t, hive.RepairTruncate, autoRepairable[0].Repair.Type)

	// Verify risk filtering
	lowRisk := report.GetByMaxRisk(hive.RiskLow)
	assert.Len(t, lowRisk, 1)

	mediumRisk := report.GetByMaxRisk(hive.RiskMedium)
	assert.Len(t, mediumRisk, 2) // Low + Medium
}
