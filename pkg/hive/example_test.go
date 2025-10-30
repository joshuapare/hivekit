package hive_test

import (
	"fmt"
	"log"

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
	_, err := hive.MergeRegString("system.hive", regContent, nil)
	if err != nil {
		log.Printf("Merge failed: %v", err)
	}
}

// ExampleMergeRegFile demonstrates merging a .reg file into a 
func ExampleMergeRegFile() {
	// Merge a .reg file into a hive with default settings
	_, err := hive.MergeRegFile("system.hive", "changes.reg", nil)
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleMergeRegFile_withProgress demonstrates progress reporting.
func ExampleMergeRegFile_withProgress() {
	opts := &hive.MergeOptions{
		OnProgress: func(current, total int) {
			pct := (current * 100) / total
			fmt.Printf("\rProgress: %d%% (%d/%d)", pct, current, total)
		},
		Defragment: true,
		CreateBackup: true,
	}

	_, err := hive.MergeRegFile("system.hive", "delta.reg", opts)
	if err != nil {
		log.Fatal(err)
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

	_, err := hive.MergeRegFile("system.hive", "risky-changes.reg", opts)
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

	_, err := hive.MergeRegFiles("system.hive", regFiles, opts)
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleExportReg demonstrates exporting a hive to .reg format.
func ExampleExportReg() {
	err := hive.ExportReg("software.hive", "backup.reg", nil)
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
	}
}

// ExampleDefragment demonstrates hive compaction.
func ExampleDefragment() {
	err := hive.Defragment("software.hive")
	if err != nil {
		log.Fatal(err)
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
	opts1 := &hive.MergeOptions{
		Limits: func() *hive.Limits {
			l := hive.DefaultLimits()
			return &l
		}(),
	}
	_ = opts1

	// Use relaxed limits for system keys
	opts2 := &hive.MergeOptions{
		Limits: func() *hive.Limits {
			l := hive.RelaxedLimits()
			return &l
		}(),
	}
	_ = opts2

	// Use strict limits for constrained environments
	opts3 := &hive.MergeOptions{
		Limits: func() *hive.Limits {
			l := hive.StrictLimits()
			return &l
		}(),
	}
	_ = opts3
}
