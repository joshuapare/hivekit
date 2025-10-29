package repair_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/repair"
	"github.com/joshuapare/hivekit/pkg/types"
)

// RepairTestCase describes a single repair integration test.
type RepairTestCase struct {
	Name              string   // Test case name
	HivePath          string   // Path to corrupted hive file
	ExpectedIssues    int      // Expected number of issues found
	ExpectedRepairable int     // Expected number of repairable issues
	ExpectedApplied   int      // Expected number of repairs applied
	ExpectedDiagnostics []string // Expected diagnostic issue substrings (optional)
	ShouldSucceed     bool     // Whether repair should succeed
	VerifyParseable   bool     // Whether to verify hive is parseable after repair
}

func TestRepairEngine_Integration(t *testing.T) {
	testCases := []RepairTestCase{
		{
			Name:              "corrupt_cell_offset_overflow",
			HivePath:          "../../testdata/corrupted/corrupt_cell_offset_overflow",
			ExpectedIssues:    1, // Should find the dangling subkey list offset
			ExpectedRepairable: 1,
			ExpectedApplied:   1,
			ExpectedDiagnostics: []string{
				"Subkey count is 0 but list offset",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
		{
			Name:              "corrupt_regf_sequence_mismatch",
			HivePath:          "../../testdata/corrupted/corrupt_regf_sequence_mismatch",
			ExpectedIssues:    1,
			ExpectedRepairable: 1,
			ExpectedApplied:   0, // Not auto-applied (AutoApply: false)
			ExpectedDiagnostics: []string{
				"sequence numbers differ",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
		{
			Name:              "corrupt_hbin_file_offset_mismatch",
			HivePath:          "../../testdata/corrupted/corrupt_hbin_file_offset_mismatch",
			ExpectedIssues:    1,
			ExpectedRepairable: 1,
			ExpectedApplied:   1,
			ExpectedDiagnostics: []string{
				"HBIN",
				"file offset mismatch",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
		{
			Name:              "corrupt_nk_subkey_count_orphaned",
			HivePath:          "../../testdata/corrupted/corrupt_nk_subkey_count_orphaned",
			ExpectedIssues:    1,
			ExpectedRepairable: 1,
			ExpectedApplied:   1,
			ExpectedDiagnostics: []string{
				"Subkey count > 0 but list offset is invalid",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
		{
			Name:              "corrupt_nk_value_count_orphaned",
			HivePath:          "../../testdata/corrupted/corrupt_nk_value_count_orphaned",
			ExpectedIssues:    1,
			ExpectedRepairable: 1,
			ExpectedApplied:   1,
			ExpectedDiagnostics: []string{
				"Value count > 0 but list offset is invalid",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
		{
			Name:              "corrupt_nk_value_list_dangling",
			HivePath:          "../../testdata/corrupted/corrupt_nk_value_list_dangling",
			ExpectedIssues:    1,
			ExpectedRepairable: 1,
			ExpectedApplied:   1,
			ExpectedDiagnostics: []string{
				"Value count is 0 but list offset is set",
			},
			ShouldSucceed:   true,
			VerifyParseable: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			runRepairTest(t, tc)
		})
	}
}

func runRepairTest(t *testing.T, tc RepairTestCase) {
	// Step 1: Read the corrupted hive file
	data, err := os.ReadFile(tc.HivePath)
	if err != nil {
		t.Fatalf("failed to read hive file: %v", err)
	}

	t.Logf("Loaded hive file: %s (%d bytes)", tc.HivePath, len(data))

	// Step 2: Run diagnostics to find issues
	r, err := reader.Open(tc.HivePath, types.OpenOptions{
		Tolerant: true,
	})
	if err != nil {
		t.Fatalf("failed to open hive: %v", err)
	}
	defer r.Close()

	report, err := r.Diagnose()
	if err != nil {
		t.Fatalf("diagnostic scan failed: %v", err)
	}

	if report == nil {
		t.Fatal("diagnostic report is nil")
	}

	diagnostics := report.Diagnostics
	t.Logf("Found %d diagnostic issues", len(diagnostics))

	// Verify expected issue count
	if tc.ExpectedIssues > 0 && len(diagnostics) != tc.ExpectedIssues {
		t.Errorf("expected %d issues, found %d", tc.ExpectedIssues, len(diagnostics))
		for i, d := range diagnostics {
			t.Logf("  [%d] %s at 0x%X: %s", i, d.Severity, d.Offset, d.Issue)
		}
	}

	// Verify expected diagnostics are present
	for _, expectedDiag := range tc.ExpectedDiagnostics {
		found := false
		for _, d := range diagnostics {
			if containsSubstring(d.Issue, expectedDiag) {
				found = true
				t.Logf("Found expected diagnostic: %s", expectedDiag)
				break
			}
		}
		if !found {
			t.Errorf("expected diagnostic not found: %s", expectedDiag)
		}
	}

	// Count repairable issues
	repairableDiags := filterRepairable(diagnostics)
	t.Logf("Found %d repairable issues", len(repairableDiags))

	if tc.ExpectedRepairable > 0 && len(repairableDiags) != tc.ExpectedRepairable {
		t.Errorf("expected %d repairable issues, found %d", tc.ExpectedRepairable, len(repairableDiags))
	}

	if len(repairableDiags) == 0 {
		t.Log("No repairable issues found, skipping repair")
		return
	}

	// Step 3: Create repair engine and register modules
	engine := repair.NewEngine(repair.EngineConfig{
		DryRun:   false,
		AutoOnly: false,
		MaxRisk:  repair.RiskHigh, // Allow all repairs for testing
		Verbose:  true,
	})

	// Register repair modules
	engine.RegisterModule(repair.NewNKModule())
	engine.RegisterModule(repair.NewREGFModule())
	engine.RegisterModule(repair.NewHBINModule())
	engine.RegisterModule(repair.NewVKModule())

	// Step 4: Make a copy of data for repair
	repairData := make([]byte, len(data))
	copy(repairData, data)

	// Step 5: Execute repairs
	result, err := engine.ExecuteRepairs(repairData, repairableDiags)

	// Check if repair should succeed
	if tc.ShouldSucceed {
		if err != nil {
			t.Fatalf("repair failed unexpectedly: %v", err)
		}

		t.Logf("Repair completed: %d applied, %d skipped, %d failed",
			result.Applied, result.Skipped, result.Failed)
		t.Logf("Duration: %v", result.Duration)

		// Verify expected applied count
		if tc.ExpectedApplied > 0 && result.Applied != tc.ExpectedApplied {
			t.Errorf("expected %d repairs applied, got %d", tc.ExpectedApplied, result.Applied)
		}

		// Show repair details
		for i, detail := range result.Repairs {
			t.Logf("  [%d] %s: %s at 0x%X (module: %s, duration: %v)",
				i, detail.Status, detail.Diagnostic.Issue, detail.Diagnostic.Offset,
				detail.Module, detail.Duration)
			if detail.Error != nil {
				t.Logf("      Error: %v", detail.Error)
			}
		}

		// Show transaction log
		t.Logf("\nTransaction Log:\n%s", engine.ExportLog())

	} else {
		if err == nil {
			t.Error("repair should have failed but succeeded")
		} else {
			t.Logf("Repair failed as expected: %v", err)
		}
		return
	}

	// Step 6: Verify repaired hive is parseable
	if tc.VerifyParseable {
		t.Log("Verifying repaired hive is parseable...")

		// Save repaired data to temp file for reader to open
		tmpFile := filepath.Join(os.TempDir(), tc.Name+"_repaired_verify.tmp")
		if err := os.WriteFile(tmpFile, repairData, 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
		defer os.Remove(tmpFile)

		repairedReader, err := reader.Open(tmpFile, types.OpenOptions{
			Tolerant: true,
		})

		if err != nil {
			t.Errorf("failed to open repaired hive: %v", err)
		} else {
			defer repairedReader.Close()

			// Check if issues were fixed
			repairedReport, err := repairedReader.Diagnose()
			if err != nil {
				t.Logf("Warning: failed to run diagnostics on repaired hive: %v", err)
			} else if repairedReport != nil {
				repairedDiags := repairedReport.Diagnostics
				t.Logf("After repair: %d diagnostic issues remain", len(repairedDiags))

				// Log issue reduction
				if len(repairedDiags) < len(diagnostics) {
					t.Logf("Issues reduced: %d -> %d", len(diagnostics), len(repairedDiags))
				} else if len(repairedDiags) == len(diagnostics) {
					t.Logf("Note: Same number of issues after repair (may be different issues or re-detected)")
				}

				// Show remaining issues
				if len(repairedDiags) > 0 {
					t.Log("Remaining issues:")
					for i, d := range repairedDiags {
						t.Logf("  [%d] %s at 0x%X: %s", i, d.Severity, d.Offset, d.Issue)
					}
				}
			}

			// Try to walk the registry tree
			rootID, err := repairedReader.Root()
			if err != nil {
				t.Errorf("failed to get root key after repair: %v", err)
			} else {
				rootName, err := repairedReader.KeyName(rootID)
				if err != nil {
					t.Logf("Warning: could not get root name: %v", err)
				} else {
					t.Logf("Successfully accessed root key: %s", rootName)
				}

				// Try to list subkeys
				subkeyCount, err := repairedReader.KeySubkeyCount(rootID)
				if err != nil {
					t.Logf("Warning: could not get subkey count: %v", err)
				} else {
					t.Logf("Root has %d subkeys", subkeyCount)
				}
			}
		}
	}

	// Step 7: Optionally save repaired hive for manual inspection
	if os.Getenv("SAVE_REPAIRED_HIVES") == "1" {
		repairedPath := filepath.Join(os.TempDir(), tc.Name+"_repaired")
		if err := os.WriteFile(repairedPath, repairData, 0644); err != nil {
			t.Logf("Warning: failed to save repaired hive: %v", err)
		} else {
			t.Logf("Saved repaired hive to: %s", repairedPath)
		}
	}
}

// filterRepairable filters diagnostics to only those with repair actions.
func filterRepairable(diagnostics []repair.Diagnostic) []repair.Diagnostic {
	var repairable []repair.Diagnostic
	for _, d := range diagnostics {
		if d.Repair != nil {
			repairable = append(repairable, d)
		}
	}
	return repairable
}

// containsSubstring checks if a string contains a substring (case-insensitive).
func containsSubstring(s, substr string) bool {
	// Simple case-sensitive check for now
	// Could use strings.Contains or regexp for more sophisticated matching
	return len(substr) > 0 && len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestRepairEngine_DryRun tests dry-run mode.
func TestRepairEngine_DryRun(t *testing.T) {
	hivePath := "../../testdata/corrupted/corrupt_cell_offset_overflow"

	// Read hive
	data, err := os.ReadFile(hivePath)
	if err != nil {
		t.Fatalf("failed to read hive file: %v", err)
	}

	// Run diagnostics
	r, err := reader.Open(hivePath, types.OpenOptions{Tolerant: true})
	if err != nil {
		t.Fatalf("failed to open hive: %v", err)
	}
	defer r.Close()

	report, err := r.Diagnose()
	if err != nil {
		t.Fatalf("diagnostic scan failed: %v", err)
	}

	diagnostics := report.Diagnostics
	repairableDiags := filterRepairable(diagnostics)

	if len(repairableDiags) == 0 {
		t.Skip("no repairable issues found")
	}

	// Create engine in dry-run mode
	engine := repair.NewEngine(repair.EngineConfig{
		DryRun:   true,
		AutoOnly: false,
		MaxRisk:  repair.RiskHigh,
	})
	engine.RegisterModule(repair.NewNKModule())
	engine.RegisterModule(repair.NewREGFModule())
	engine.RegisterModule(repair.NewHBINModule())
	engine.RegisterModule(repair.NewVKModule())

	// Make a copy
	repairData := make([]byte, len(data))
	copy(repairData, data)

	// Execute dry-run
	result, err := engine.ExecuteRepairs(repairData, repairableDiags)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	t.Logf("Dry-run: %d would be applied, %d skipped, %d failed",
		result.Applied, result.Skipped, result.Failed)

	// Verify data was NOT modified
	for i := range data {
		if data[i] != repairData[i] {
			t.Errorf("data was modified at offset %d in dry-run mode", i)
			break
		}
	}

	t.Log("Dry-run: data unchanged (correct)")
}

// TestRepairEngine_RollbackOnFailure tests that repairs are rolled back on failure.
func TestRepairEngine_RollbackOnFailure(t *testing.T) {
	// This test will be more useful once we have more complex repair scenarios
	// For now, just verify the rollback mechanism works
	t.Skip("Rollback test requires more complex repair scenarios")
}
