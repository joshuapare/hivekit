package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose bool
	quiet   bool
	jsonOut bool
	noColor bool
)

var rootCmd = &cobra.Command{
	Use:   "hivectl",
	Short: "Inspect and manipulate Windows registry hive files",
	Long: `hivectl is a tool for inspecting, modifying, and analyzing
Windows Registry hive files. It supports reading, writing, merging, and exporting
registry data with comprehensive validation and safety features.`,
	Version: "0.1.0",
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().
		BoolVarP(&quiet, "quiet", "q", false, "Suppress all output except errors")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Helper functions for output

// printInfo prints an info message if not in quiet mode
func printInfo(format string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stdout, format, args...)
	}
}

// printError prints an error message
func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format, args...)
}

// printVerbose prints a verbose message if verbose mode is enabled
func printVerbose(format string, args ...interface{}) {
	if verbose && !quiet {
		fmt.Fprintf(os.Stdout, format, args...)
	}
}

// printJSON outputs data as JSON
func printJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// checkArgs validates that the correct number of arguments were provided
func checkArgs(args []string, expected int, usage string) error {
	if len(args) != expected {
		return fmt.Errorf("expected %d argument(s), got %d\nUsage: %s", expected, len(args), usage)
	}
	return nil
}

// checkMinArgs validates that at least the minimum number of arguments were provided
func checkMinArgs(args []string, min int, usage string) error {
	if len(args) < min {
		return fmt.Errorf(
			"expected at least %d argument(s), got %d\nUsage: %s",
			min,
			len(args),
			usage,
		)
	}
	return nil
}
