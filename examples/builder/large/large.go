// Package main demonstrates building a large hive with progressive writes.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joshuapare/hivekit/hive/builder"
)

func largeExample() {
	// Configure options for large hive building
	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 1000 // Flush every 1000 operations
	opts.PreallocPages = 10000     // Pre-allocate ~40MB

	// Create builder
	b, err := builder.New("large.hive", opts)
	if err != nil {
		slog.Error("Failed to create builder", "error", err)
		os.Exit(1)
	}

	fmt.Println("Building large hive with progressive writes...")

	// Add 10,000 keys with values
	const numKeys = 10000
	for i := range numKeys {
		keyPath := []string{"Data", fmt.Sprintf("Key%05d", i)}

		// Add values to each key
		if err := b.SetString(keyPath, "Name", fmt.Sprintf("Entry %d", i)); err != nil {
			slog.Error("Failed to set string", "error", err)
			b.Close()
			os.Exit(1)
		}

		if err := b.SetDWORD(keyPath, "Index", uint32(i)); err != nil {
			slog.Error("Failed to set DWORD", "error", err)
			b.Close()
			os.Exit(1)
		}

		if err := b.SetQWORD(keyPath, "Timestamp", uint64(1700000000+i)); err != nil {
			slog.Error("Failed to set QWORD", "error", err)
			b.Close()
			os.Exit(1)
		}

		// Progress indicator
		if (i+1)%1000 == 0 {
			fmt.Printf("Progress: %d/%d keys\n", i+1, numKeys)
		}
	}

	// Commit
	fmt.Println("Committing...")
	if err := b.Commit(); err != nil {
		slog.Error("Failed to commit", "error", err)
		b.Close()
		os.Exit(1)
	}

	b.Close()
	fmt.Println("Large hive created successfully: large.hive")
	fmt.Printf("Created %d keys with 3 values each (%d total operations)\n", numKeys, numKeys*3)
}

func main() {
	largeExample()
}
