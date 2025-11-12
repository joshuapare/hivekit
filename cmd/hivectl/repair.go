package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	repairDryRun       bool
	repairAutoOnly     bool
	repairMaxRisk      string
	repairBackupSuffix string
	repairNoBackup     bool
	repairTolerant     bool
	repairInteractive  bool
)

var repairCmd = &cobra.Command{
	Use:   "repair <hive-file>",
	Short: "Apply repairs to a corrupted hive file",
	Long: `Applies repair actions to fix issues in a registry hive file.

The repair command:
1. Runs a diagnostic scan to identify issues
2. Filters repairs based on risk level and auto-apply flags
3. Creates a backup (unless --no-backup)
4. Applies repairs to the original file
5. Reports results

IMPORTANT: Always create backups before repairing. The --dry-run flag lets you
preview what would be changed without modifying the file.`,
	Example: `  # Preview repairs without applying them
  hivectl repair --dry-run corrupt.hive

  # Apply only auto-repairable fixes
  hivectl repair --auto-only corrupt.hive

  # Apply repairs up to medium risk level
  hivectl repair --max-risk medium corrupt.hive

  # Repair with custom backup suffix
  hivectl repair --backup-suffix .backup.$(date +%s) corrupt.hive

  # Dangerous: repair without backup (not recommended!)
  hivectl repair --no-backup corrupt.hive`,
	Args: cobra.ExactArgs(1),
	RunE: runRepair,
}

func init() {
	repairCmd.Flags().BoolVarP(&repairDryRun, "dry-run", "n", false,
		"Preview repairs without applying them")
	repairCmd.Flags().BoolVarP(&repairAutoOnly, "auto-only", "a", false,
		"Only apply auto-repairable fixes (safest)")
	repairCmd.Flags().StringVarP(&repairMaxRisk, "max-risk", "r", "medium",
		"Maximum risk level: none, low, medium, high")
	repairCmd.Flags().StringVarP(&repairBackupSuffix, "backup-suffix", "b", ".backup",
		"Suffix for backup file")
	repairCmd.Flags().BoolVar(&repairNoBackup, "no-backup", false,
		"Skip creating backup (DANGEROUS - not recommended!)")
	repairCmd.Flags().BoolVarP(&repairTolerant, "tolerant", "t", false,
		"Use tolerant mode when opening hive")
	repairCmd.Flags().BoolVarP(&repairInteractive, "interactive", "i", false,
		"Prompt before applying each repair (not yet implemented)")

	rootCmd.AddCommand(repairCmd)
}

func runRepair(cmd *cobra.Command, args []string) error {
	hivePath := args[0]

	// Verify file exists
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Warn about no-backup
	if repairNoBackup && !repairDryRun {
		printInfo("WARNING: Running without backup! Original file will be modified.\n")
		printInfo("Press Ctrl+C to cancel or Enter to continue...\n")
		fmt.Scanln()
	}

	// Parse max risk level
	maxRisk, err := parseRiskLevel(repairMaxRisk)
	if err != nil {
		return err
	}

	printInfo("Scanning hive file: %s\n", hivePath)
	if repairDryRun {
		printInfo("Mode: DRY-RUN (no changes will be made)\n")
	}
	if repairAutoOnly {
		printInfo("Filter: Auto-repairable only\n")
	}
	printInfo("Max risk: %s\n", maxRisk)
	printInfo("\n")

	// Open hive and run diagnostics
	r, err := reader.Open(hivePath, hive.OpenOptions{
		Tolerant: repairTolerant,
	})
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	// Run diagnostic scan
	report, err := r.Diagnose()
	if err != nil {
		return fmt.Errorf("diagnostic scan failed: %w", err)
	}
	report.FilePath = hivePath

	// Check if there are any repairable issues
	if report.Summary.Repairable == 0 {
		printInfo("✓ No repairable issues found.\n")
		return nil
	}

	printInfo("Found %d repairable issue(s)\n", report.Summary.Repairable)
	if report.Summary.AutoRepairable > 0 {
		printInfo("  - %d auto-repairable (low risk)\n", report.Summary.AutoRepairable)
	}
	printInfo("\n")

	// Apply repairs using the internal reader package
	repairOpts := hive.RepairOptions{
		DryRun:       repairDryRun,
		AutoOnly:     repairAutoOnly,
		MaxRisk:      maxRisk,
		BackupSuffix: repairBackupSuffix,
		NoBackup:     repairNoBackup,
		Verbose:      verbose,
	}

	result, err := reader.ApplyRepairs(hivePath, report.Diagnostics, repairOpts)
	if err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}

	// Report results
	if repairDryRun {
		printInfo("=== DRY-RUN RESULTS ===\n")
		printInfo("Would apply: %d repair(s)\n", result.Applied)
	} else {
		printInfo("=== REPAIR RESULTS ===\n")
		printInfo("Applied:  %d repair(s)\n", result.Applied)
		if result.BackupPath != "" {
			printInfo("Backup:   %s\n", result.BackupPath)
		}
	}
	printInfo("Skipped:  %d repair(s)\n", result.Skipped)
	if result.Failed > 0 {
		printInfo("Failed:   %d repair(s)\n", result.Failed)
	}
	printInfo("Duration: %v\n\n", result.Duration)

	// Show detailed repair log if verbose
	if verbose && len(result.Diagnostics) > 0 {
		printInfo("Detailed repair log:\n")
		for i, diag := range result.Diagnostics {
			status := "✓"
			if !diag.Applied {
				status = "○"
			}
			errMsg := ""
			if diag.Error != "" {
				errMsg = fmt.Sprintf(" (%s)", diag.Error)
			}
			printInfo("  %s %d. [0x%X] %s%s\n", status, i+1, diag.Offset, diag.Description, errMsg)
		}
		printInfo("\n")
	}

	// Final status
	if repairDryRun {
		printInfo("✓ Dry-run complete. Run without --dry-run to apply repairs.\n")
	} else if result.Applied > 0 {
		printInfo("✓ Repairs applied successfully!\n")
		if result.BackupPath != "" {
			printInfo("\nTo restore from backup:\n")
			printInfo("  cp %s %s\n", result.BackupPath, hivePath)
		}
	} else {
		printInfo("✓ No repairs applied (all skipped or filtered).\n")
	}

	return nil
}

func parseRiskLevel(s string) (hive.RiskLevel, error) {
	switch s {
	case "none":
		return hive.RiskNone, nil
	case "low":
		return hive.RiskLow, nil
	case "medium":
		return hive.RiskMedium, nil
	case "high":
		return hive.RiskHigh, nil
	default:
		return hive.RiskNone, fmt.Errorf("invalid risk level: %s (use: none, low, medium, high)", s)
	}
}
