// Package main demonstrates building a hive from .reg file format data.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joshuapare/hivekit/hive/builder"
)

// RegEntry represents a registry entry in .reg file format.
type RegEntry struct {
	FullPath  string // Full path with backslashes, e.g., "HKLM\Software\MyApp"
	ValueName string // Value name
	ValueType string // Type string, e.g., "REG_SZ", "REG_DWORD"
	Data      []byte // Raw data bytes
}

func regfileFormatExample() {
	// Simulate data from a .reg file or similar source
	// In a real parser, these would come from parsing the file
	entries := []RegEntry{
		{
			FullPath:  "HKEY_LOCAL_MACHINE\\Software\\MyCompany\\MyApp",
			ValueName: "Version",
			ValueType: "REG_SZ",
			Data:      builder.EncodeStringHelper("1.0.0"), // We'll add this helper
		},
		{
			FullPath:  "HKLM\\Software\\MyCompany\\MyApp",
			ValueName: "InstallDate",
			ValueType: "REG_SZ",
			Data:      builder.EncodeStringHelper("2025-01-15"),
		},
		{
			FullPath:  "Software\\MyCompany\\MyApp\\Settings",
			ValueName: "Timeout",
			ValueType: "DWORD",
			Data:      builder.EncodeDWORDHelper(30),
		},
		{
			FullPath:  "HKCU\\Software\\MyCompany\\MyApp\\Settings",
			ValueName: "MaxConnections",
			ValueType: "REG_DWORD",
			Data:      builder.EncodeDWORDHelper(100),
		},
		{
			FullPath:  "Software\\MyCompany\\MyApp",
			ValueName: "Features",
			ValueType: "REG_MULTI_SZ",
			Data: builder.EncodeMultiStringHelper(
				[]string{"Feature1", "Feature2", "Feature3"},
			),
		},
		{
			FullPath:  "Software\\MyCompany\\MyApp",
			ValueName: "ConfigData",
			ValueType: "REG_BINARY",
			Data:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		},
	}

	// Create builder
	b, err := builder.New("regfile_output.hive", nil)
	if err != nil {
		slog.Error("Failed to create builder", "error", err)
		os.Exit(1)
	}

	fmt.Println("Building hive from reg file format data...")

	// Process each entry using the convenient SetValueFromString method
	for i, entry := range entries {
		err := b.SetValueFromString(entry.FullPath, entry.ValueName, entry.ValueType, entry.Data)
		if err != nil {
			slog.Error("Failed to set entry", "index", i, "error", err)
			b.Close()
			os.Exit(1)
		}

		fmt.Printf("  Added: %s\\%s (%s)\n", entry.FullPath, entry.ValueName, entry.ValueType)
	}

	// Commit the hive
	fmt.Println("\nCommitting hive...")
	if err := b.Commit(); err != nil {
		slog.Error("Failed to commit", "error", err)
		b.Close()
		os.Exit(1)
	}

	b.Close()
	fmt.Println("Hive created successfully: regfile_output.hive")
	fmt.Printf("Total entries: %d\n", len(entries))
}

func main() {
	regfileFormatExample()
}
