package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	searchKeysOnly      bool
	searchValuesOnly    bool
	searchRegex         bool
	searchCaseSensitive bool
	searchMaxResults    int
	searchKey           string
)

func init() {
	cmd := newSearchCmd()
	cmd.Flags().BoolVar(&searchKeysOnly, "keys-only", false, "Search only keys")
	cmd.Flags().BoolVar(&searchValuesOnly, "values-only", false, "Search only values")
	cmd.Flags().BoolVar(&searchRegex, "regex", false, "Use regex pattern")
	cmd.Flags().BoolVar(&searchCaseSensitive, "case-sensitive", false, "Case-sensitive search")
	cmd.Flags().IntVar(&searchMaxResults, "max-results", 0, "Limit results (0 = unlimited)")
	cmd.Flags().StringVar(&searchKey, "key", "", "Search within subtree")
	rootCmd.AddCommand(cmd)
}

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <hive> <pattern>",
		Short: "Search for keys and values matching a pattern",
		Long: `The search command searches for keys and values matching a pattern.
By default, searches both key names and value names/data (case-insensitive).

Example:
  hivectl search system.hive "network"
  hivectl search system.hive "^Network" --regex --case-sensitive
  hivectl search system.hive "ACPI" --keys-only
  hivectl search system.hive "startup" --key "ControlSet001\\Services"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(args)
		},
	}
	return cmd
}

type SearchResult struct {
	MatchedKeys   []string
	MatchedValues []ValueMatch
}

type ValueMatch struct {
	KeyPath   string
	ValueName string
	ValueType string
	ValueData string
}

func runSearch(args []string) error {
	hivePath := args[0]
	pattern := args[1]

	printVerbose("Opening hive: %s\n", hivePath)
	printVerbose("Searching for pattern: %s\n", pattern)

	// Compile regex if needed
	var re *regexp.Regexp
	var err error
	if searchRegex {
		flags := ""
		if !searchCaseSensitive {
			flags = "(?i)"
		}
		re, err = regexp.Compile(flags + pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Match function
	matchFunc := func(text string) bool {
		if searchRegex {
			return re.MatchString(text)
		}
		if searchCaseSensitive {
			return strings.Contains(text, pattern)
		}
		return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
	}

	result := SearchResult{
		MatchedKeys:   make([]string, 0),
		MatchedValues: make([]ValueMatch, 0),
	}

	// List all keys
	keys, err := hive.ListKeys(hivePath, searchKey, true, 0)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	// Search keys
	if !searchValuesOnly {
		for _, key := range keys {
			// Extract key name (last component)
			parts := strings.Split(key.Path, "\\")
			keyName := parts[len(parts)-1]

			if matchFunc(keyName) {
				result.MatchedKeys = append(result.MatchedKeys, key.Path)

				// Check max results
				if searchMaxResults > 0 && len(result.MatchedKeys) >= searchMaxResults {
					break
				}
			}
		}
	}

	// Search values
	if !searchKeysOnly {
		for _, key := range keys {
			// Check max results
			if searchMaxResults > 0 && len(result.MatchedValues) >= searchMaxResults {
				break
			}

			values, err := hive.ListValues(hivePath, key.Path)
			if err != nil {
				printVerbose("Warning: failed to list values for %s: %v\n", key.Path, err)
				continue
			}

			for _, val := range values {
				// Check value name
				nameMatches := matchFunc(val.Name)

				// Check value data
				dataMatches := false
				var dataStr string
				if val.StringVal != "" {
					dataStr = val.StringVal
					dataMatches = matchFunc(val.StringVal)
				} else if len(val.StringVals) > 0 {
					dataStr = strings.Join(val.StringVals, ", ")
					for _, s := range val.StringVals {
						if matchFunc(s) {
							dataMatches = true
							break
						}
					}
				} else if val.Type == "REG_DWORD" {
					dataStr = fmt.Sprintf("0x%08x", val.DWordVal)
				} else if val.Type == "REG_QWORD" {
					dataStr = fmt.Sprintf("0x%016x", val.QWordVal)
				}

				if nameMatches || dataMatches {
					result.MatchedValues = append(result.MatchedValues, ValueMatch{
						KeyPath:   key.Path,
						ValueName: val.Name,
						ValueType: val.Type,
						ValueData: dataStr,
					})

					// Check max results
					if searchMaxResults > 0 && len(result.MatchedValues) >= searchMaxResults {
						break
					}
				}
			}
		}
	}

	// Output as JSON if requested
	if jsonOut {
		return printJSON(result)
	}

	// Text output
	printInfo("\nSearching for \"%s\" in %s...\n\n", pattern, hivePath)

	// Print matched keys
	if len(result.MatchedKeys) > 0 {
		printInfo("Keys (case-insensitive):\n")
		for _, keyPath := range result.MatchedKeys {
			printInfo("  %s\n", keyPath)
		}
		if searchMaxResults > 0 && len(result.MatchedKeys) >= searchMaxResults {
			printInfo("  ... (limited to %d results)\n", searchMaxResults)
		}
		printInfo("\n")
	}

	// Print matched values
	if len(result.MatchedValues) > 0 {
		printInfo("Values:\n")
		currentKey := ""
		for _, vm := range result.MatchedValues {
			if vm.KeyPath != currentKey {
				printInfo("  %s\n", vm.KeyPath)
				currentKey = vm.KeyPath
			}
			valName := vm.ValueName
			if valName == "" {
				valName = "(Default)"
			}
			if vm.ValueData != "" {
				printInfo("    └── %s = %s\n", valName, vm.ValueData)
			} else {
				printInfo("    └── %s\n", valName)
			}
		}
		if searchMaxResults > 0 && len(result.MatchedValues) >= searchMaxResults {
			printInfo("  ... (limited to %d results)\n", searchMaxResults)
		}
		printInfo("\n")
	}

	// Summary
	printInfo("Total: %d keys, %d values\n", len(result.MatchedKeys), len(result.MatchedValues))

	return nil
}
