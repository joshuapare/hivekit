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

// ExampleMergeRegFile_withOptions demonstrates merge with options.
func ExampleMergeRegFile_withOptions() {
	opts := &hive.MergeOptions{
		Defragment:   true,
		CreateBackup: true,
	}

	err := hive.MergeRegFile("system.hive", "delta.reg", opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("Merge complete with defragmentation and backup")
}

// ExampleMergeRegFiles demonstrates batch merging.
func ExampleMergeRegFiles() {
	regFiles := []string{
		"base.reg",
		"patch1.reg",
		"patch2.reg",
	}

	err := hive.MergeRegFiles("system.hive", regFiles, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("All files merged successfully")
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
