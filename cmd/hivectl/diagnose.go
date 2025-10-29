package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	diagFormat      string
	diagTolerant    bool
	diagOutputFile  string
	diagShowSummary bool
)

var diagnoseCmd = &cobra.Command{
	Use:   "diagnose <hive-file>",
	Short: "Run comprehensive diagnostic scan on a hive file",
	Long: `Performs a complete diagnostic scan of a registry hive file, checking for:
  - Structural integrity (REGF, HBIN headers)
  - Data corruption (truncated values, invalid offsets)
  - Tree integrity (cycles, orphaned cells)
  - Consistency issues

The scan reports all issues with precise byte offsets and suggests repairs where possible.`,
	Example: `  # Scan a hive and show text report
  hivectl diagnose system.hive

  # Output JSON for programmatic analysis
  hivectl diagnose --format json system.hive

  # Compact format for grep
  hivectl diagnose --format compact corrupt.hive

  # Save report to file
  hivectl diagnose --output report.txt system.hive

  # Tolerant mode (continue scanning despite errors)
  hivectl diagnose --tolerant corrupt.hive`,
	Args: cobra.ExactArgs(1),
	RunE: runDiagnose,
}

func init() {
	diagnoseCmd.Flags().StringVarP(&diagFormat, "format", "f", "text",
		"Output format: text, json, compact, hex (text=human-readable, json=structured, compact=one-line-per-issue, hex=annotations)")
	diagnoseCmd.Flags().BoolVarP(&diagTolerant, "tolerant", "t", false,
		"Continue scanning despite errors (tolerant mode)")
	diagnoseCmd.Flags().StringVarP(&diagOutputFile, "output", "o", "",
		"Write report to file instead of stdout")
	diagnoseCmd.Flags().BoolVarP(&diagShowSummary, "summary", "s", false,
		"Show only summary (no detailed diagnostics)")

	rootCmd.AddCommand(diagnoseCmd)
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	hivePath := args[0]

	// Verify file exists
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	printInfo("Scanning hive file: %s\n", hivePath)
	if diagTolerant {
		printInfo("Mode: Tolerant (continuing despite errors)\n")
	}
	printInfo("\n")

	// Open hive
	r, err := reader.Open(hivePath, hive.OpenOptions{
		Tolerant: diagTolerant,
	})
	if err != nil {
		return fmt.Errorf(
			"failed to open hive: %w\n\nNote: This hive has critical structural issues that prevent opening.\nRun with --tolerant to attempt partial recovery.",
			err,
		)
	}
	defer r.Close()

	// Run full diagnostic scan
	report, err := r.Diagnose()
	if err != nil {
		return fmt.Errorf("diagnostic scan failed: %w", err)
	}

	// Set file path for report
	report.FilePath = hivePath

	// Output based on format
	var output string
	switch diagFormat {
	case "json":
		jsonStr, err := report.FormatJSON()
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
		output = jsonStr

	case "compact":
		output = report.FormatTextCompact()

	case "hex":
		output = report.FormatHexAnnotations()

	case "text":
		if diagShowSummary {
			// Just summary
			output = formatSummaryOnly(report)
		} else {
			output = report.FormatText()
		}

	default:
		return fmt.Errorf("unknown format: %s (use: text, json, compact, hex)", diagFormat)
	}

	// Write output
	if diagOutputFile != "" {
		err := os.WriteFile(diagOutputFile, []byte(output), 0644)
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		printInfo("Report written to: %s\n", diagOutputFile)
	} else {
		fmt.Print(output)
	}

	// Exit code based on severity
	if report.HasCriticalIssues() {
		printInfo("\n⚠️  CRITICAL issues found\n")
		os.Exit(2)
	} else if report.HasErrors() {
		printInfo("\n⚠️  Errors found\n")
		os.Exit(1)
	} else if report.Summary.Warnings > 0 {
		printInfo("\n✓ Warnings found (non-critical)\n")
	} else {
		printInfo("\n✓ No issues found\n")
	}

	return nil
}

func formatSummaryOnly(report *hive.DiagnosticReport) string {
	output := fmt.Sprintf("Diagnostic Summary for %s\n", report.FilePath)
	output += fmt.Sprintf("File size: %d bytes\n", report.FileSize)
	output += fmt.Sprintf("Scan time: %v\n\n", report.ScanTime)
	output += fmt.Sprintf("Critical:  %d\n", report.Summary.Critical)
	output += fmt.Sprintf("Errors:    %d\n", report.Summary.Errors)
	output += fmt.Sprintf("Warnings:  %d\n", report.Summary.Warnings)
	output += fmt.Sprintf("Info:      %d\n\n", report.Summary.Info)

	if report.Summary.Repairable > 0 {
		output += fmt.Sprintf("Repairable:      %d\n", report.Summary.Repairable)
		output += fmt.Sprintf("Auto-repairable: %d\n", report.Summary.AutoRepairable)
	}

	return output
}
