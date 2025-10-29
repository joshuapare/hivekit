package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	statsKey string
)

func init() {
	cmd := newStatsCmd()
	cmd.Flags().StringVar(&statsKey, "key", "", "Stats for specific subtree")
	rootCmd.AddCommand(cmd)
}

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats <hive>",
		Short: "Show detailed statistics",
		Long: `The stats command shows detailed statistics about a hive including
key/value counts, type distribution, size analysis, and more.

Example:
  hivectl stats system.hive
  hivectl stats system.hive --key "ControlSet001\\Services"
  hivectl stats system.hive --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(args)
		},
	}
	return cmd
}

type HiveStats struct {
	FilePath     string
	FileSize     int64
	LastModified time.Time

	TotalKeys   int
	TotalValues int
	MaxDepth    int

	KeysByLevel map[int]int
	ValueTypes  map[string]int
	ValueSizes  map[string]int // <100, 100-1K, 1K-10K, >10K

	LargestKey struct {
		Path        string
		SubkeyCount int
	}

	LargestValue struct {
		Path string
		Name string
		Size int
	}
}

func runStats(args []string) error {
	hivePath := args[0]
	keyPath := statsKey

	printVerbose("Opening hive: %s\n", hivePath)

	// Get file info
	fileInfo, err := os.Stat(hivePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	stats := HiveStats{
		FilePath:     hivePath,
		FileSize:     fileInfo.Size(),
		LastModified: fileInfo.ModTime(),
		KeysByLevel:  make(map[int]int),
		ValueTypes:   make(map[string]int),
		ValueSizes:   make(map[string]int),
	}

	// List all keys recursively
	keys, err := hive.ListKeys(hivePath, keyPath, true, 0)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	stats.TotalKeys = len(keys)

	// Analyze each key
	for _, key := range keys {
		// Calculate depth
		depth := strings.Count(key.Path, "\\") + 1
		if keyPath != "" {
			// Adjust for subtree
			depth -= strings.Count(keyPath, "\\")
		}
		stats.KeysByLevel[depth]++
		if depth > stats.MaxDepth {
			stats.MaxDepth = depth
		}

		// Track largest key by subkey count
		if key.SubkeyN > stats.LargestKey.SubkeyCount {
			stats.LargestKey.Path = key.Path
			stats.LargestKey.SubkeyCount = key.SubkeyN
		}

		// Get values for this key
		values, err := hive.ListValues(hivePath, key.Path)
		if err != nil {
			printVerbose("Warning: failed to list values for %s: %v\n", key.Path, err)
			continue
		}

		stats.TotalValues += len(values)

		// Analyze values
		for _, val := range values {
			// Type distribution
			stats.ValueTypes[val.Type]++

			// Size distribution
			size := val.Size
			if size < 100 {
				stats.ValueSizes["<100"]++
			} else if size < 1024 {
				stats.ValueSizes["100-1K"]++
			} else if size < 10240 {
				stats.ValueSizes["1K-10K"]++
			} else {
				stats.ValueSizes[">10K"]++
			}

			// Track largest value
			if size > stats.LargestValue.Size {
				stats.LargestValue.Path = key.Path
				stats.LargestValue.Name = val.Name
				stats.LargestValue.Size = size
			}
		}
	}

	// Output as JSON if requested
	if jsonOut {
		return printJSON(stats)
	}

	// Text output
	printInfo("\nHive Statistics: %s\n", hivePath)
	printInfo("%s\n\n", strings.Repeat("═", 40))

	// File information
	printInfo("File Information:\n")
	printInfo("  Path: %s\n", hivePath)
	printInfo("  Size: %s (%s bytes)\n", formatBytes(stats.FileSize), formatNumber(stats.FileSize))
	printInfo("  Last Modified: %s\n\n", stats.LastModified.Format("2006-01-02 15:04:05"))

	// Structure
	printInfo("Structure:\n")
	printInfo("  Total Keys: %s\n", formatNumber(int64(stats.TotalKeys)))
	printInfo("  Total Values: %s\n", formatNumber(int64(stats.TotalValues)))
	printInfo("  Max Depth: %d levels\n", stats.MaxDepth)
	printInfo("  Largest Key: %s (%d subkeys)\n\n", stats.LargestKey.Path, stats.LargestKey.SubkeyCount)

	// Keys by level
	if len(stats.KeysByLevel) > 0 {
		printInfo("Keys by Level:\n")
		// Sort levels
		levels := make([]int, 0, len(stats.KeysByLevel))
		for level := range stats.KeysByLevel {
			levels = append(levels, level)
		}
		sort.Ints(levels)

		for _, level := range levels {
			if level <= 10 { // Only show first 10 levels
				printInfo("  Level %d: %s keys\n", level, formatNumber(int64(stats.KeysByLevel[level])))
			}
		}
		if len(levels) > 10 {
			printInfo("  ... (%d more levels)\n", len(levels)-10)
		}
		printInfo("\n")
	}

	// Values by type
	if len(stats.ValueTypes) > 0 {
		printInfo("Values by Type:\n")
		// Sort types by count
		type typeCount struct {
			Type  string
			Count int
		}
		var types []typeCount
		for t, c := range stats.ValueTypes {
			types = append(types, typeCount{t, c})
		}
		sort.Slice(types, func(i, j int) bool {
			return types[i].Count > types[j].Count
		})

		for _, tc := range types {
			percentage := float64(tc.Count) * 100.0 / float64(stats.TotalValues)
			printInfo("  %s: %s (%.1f%%)\n", tc.Type, formatNumber(int64(tc.Count)), percentage)
		}
		printInfo("\n")
	}

	// Size distribution
	if len(stats.ValueSizes) > 0 {
		printInfo("Size Distribution:\n")
		order := []string{"<100", "100-1K", "1K-10K", ">10K"}
		for _, bucket := range order {
			if count, ok := stats.ValueSizes[bucket]; ok {
				percentage := float64(count) * 100.0 / float64(stats.TotalValues)
				printInfo("  Values %s bytes: %s (%.1f%%)\n", bucket, formatNumber(int64(count)), percentage)
			}
		}
		if stats.LargestValue.Size > 0 {
			valName := stats.LargestValue.Name
			if valName == "" {
				valName = "(Default)"
			}
			printInfo("  Largest value: %s (%s\\%s)\n", formatBytes(int64(stats.LargestValue.Size)), stats.LargestValue.Path, valName)
		}
	}

	return nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatNumber(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	// Add commas
	var result strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}
