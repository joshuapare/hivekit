// Package main demonstrates basic usage of the hive builder.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joshuapare/hivekit/hive/builder"
)

func simpleExample() {
	// Create a new hive builder
	b, err := builder.New("example.hive", nil)
	if err != nil {
		slog.Error("Failed to create builder", "error", err)
		os.Exit(1)
	}

	// Add registry keys and values using the path-based API
	fmt.Println("Building hive...")

	// Set string values
	if err := b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0"); err != nil {
		slog.Error("Failed to set string", "error", err)
		b.Close()
		os.Exit(1)
	}

	if err := b.SetString([]string{"Software", "MyApp"}, "Name", "Example Application"); err != nil {
		slog.Error("Failed to set string", "error", err)
		b.Close()
		os.Exit(1)
	}

	// Set numeric values
	if err := b.SetDWORD([]string{"Software", "MyApp", "Settings"}, "Timeout", 30); err != nil {
		slog.Error("Failed to set DWORD", "error", err)
		b.Close()
		os.Exit(1)
	}

	if err := b.SetDWORD([]string{"Software", "MyApp", "Settings"}, "MaxRetries", 5); err != nil {
		slog.Error("Failed to set DWORD", "error", err)
		b.Close()
		os.Exit(1)
	}

	// Set binary data
	configData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if err := b.SetBinary([]string{"Software", "MyApp"}, "Config", configData); err != nil {
		slog.Error("Failed to set binary", "error", err)
		b.Close()
		os.Exit(1)
	}

	// Set multi-string value
	paths := []string{
		"C:\\Program Files\\MyApp",
		"C:\\Program Files\\MyApp\\Plugins",
	}
	if err := b.SetMultiString([]string{"Software", "MyApp"}, "SearchPaths", paths); err != nil {
		slog.Error("Failed to set multi-string", "error", err)
		b.Close()
		os.Exit(1)
	}

	// Commit the changes
	fmt.Println("Committing hive...")
	if err := b.Commit(); err != nil {
		slog.Error("Failed to commit", "error", err)
		b.Close()
		os.Exit(1)
	}

	b.Close()
	fmt.Println("Hive created successfully: example.hive")
}

func main() {
	simpleExample()
}
