package hive_test

import (
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Example shows basic merge functionality.
func Example() {
	// Create a .reg file content
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Example]
"Version"="1.0"
"Enabled"=dword:00000001
`

	// Merge into a hive (in real usage, use a real hive file)
	err := hive.MergeRegString("system.hive", regContent, nil)
	if err != nil {
		fmt.Printf("Merge failed: %v\n", err)
	}
}

// ExampleMergeRegFile demonstrates merging a .reg file into a.
func ExampleMergeRegFile() {
	// Merge a .reg file into a hive with default settings
	err := hive.MergeRegFile("system.hive", "changes.reg", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
}

// ExampleMergeRegFile_withProgress demonstrates progress reporting.
func ExampleMergeRegFile_withProgress() {
	opts := &hive.MergeOptions{
		OnProgress: func(current, total int) {
			pct := (current * 100) / total
			fmt.Printf("\rProgress: %d%% (%d/%d)", pct, current, total)
		},
		Defragment:   true,
		CreateBackup: true,
	}

	err := hive.MergeRegFile("system.hive", "delta.reg", opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("\nDone!")
}

// ExampleMergeRegFile_dryRun demonstrates validation without applying changes.
func ExampleMergeRegFile_dryRun() {
	opts := &hive.MergeOptions{
		DryRun: true,
		Limits: func() *hive.Limits {
			l := hive.StrictLimits()
			return &l
		}(),
	}

	err := hive.MergeRegFile("system.hive", "risky-changes.reg", opts)
	if err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		fmt.Println("Changes would violate registry limits - not applying!")
		return
	}

	fmt.Println("Validation passed - safe to apply")
}

// ExampleMergeRegFiles demonstrates batch merging.
func ExampleMergeRegFiles() {
	regFiles := []string{
		"base.reg",
		"patch1.reg",
		"patch2.reg",
	}

	opts := &hive.MergeOptions{
		OnProgress: func(current, total int) {
			fmt.Printf("Merging file %d/%d\n", current, total)
		},
	}

	err := hive.MergeRegFiles("system.hive", regFiles, opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
}

// ExampleExportReg demonstrates exporting a hive to .reg format.
func ExampleExportReg() {
	err := hive.ExportReg("software.hive", "backup.reg", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("Hive exported successfully")
}

// ExampleExportReg_subtree demonstrates exporting a subtree.
func ExampleExportReg_subtree() {
	opts := &hive.ExportOptions{
		SubtreePath: "Microsoft\\Windows",
		Encoding:    "UTF-16LE",
		WithBOM:     true,
	}

	err := hive.ExportReg("software.hive", "windows.reg", opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
}

// ExampleDefragment demonstrates hive compaction.
func ExampleDefragment() {
	err := hive.Defragment("software.hive")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("Hive defragmented successfully")
}

// ExampleValidateHive demonstrates hive validation.
func ExampleValidateHive() {
	err := hive.ValidateHive("system.hive", hive.DefaultLimits())
	if err != nil {
		fmt.Printf("Hive validation failed: %v\n", err)
		return
	}
	fmt.Println("Hive is valid")
}

// ExampleDefaultLimits demonstrates using different limit presets.
func ExampleDefaultLimits() {
	// Use default Windows limits (recommended)
	defaultLimits := hive.DefaultLimits()
	opts1 := &hive.MergeOptions{
		Limits: &defaultLimits, //nolint:govet // Example code demonstrating API usage
	}
	_ = opts1

	// Use relaxed limits for system keys
	relaxedLimits := hive.RelaxedLimits()
	opts2 := &hive.MergeOptions{
		Limits: &relaxedLimits, //nolint:govet // Example code demonstrating API usage
	}
	_ = opts2

	// Use strict limits for constrained environments
	strictLimits := hive.StrictLimits()
	opts3 := &hive.MergeOptions{
		Limits: &strictLimits, //nolint:govet // Example code demonstrating API usage
	}
	_ = opts3
}
