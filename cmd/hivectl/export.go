package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	exportKey      string
	exportEncoding string
	exportBOM      bool
	exportStdout   bool
)

func init() {
	cmd := newExportCmd()
	cmd.Flags().StringVar(&exportKey, "key", "", "Export only specific subtree path")
	cmd.Flags().StringVar(&exportEncoding, "encoding", "utf8", "Output encoding (utf8, utf16le)")
	cmd.Flags().BoolVar(&exportBOM, "with-bom", false, "Include byte-order mark")
	cmd.Flags().BoolVar(&exportStdout, "stdout", false, "Write to stdout instead of file")
	rootCmd.AddCommand(cmd)
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <hive> [output.reg]",
		Short: "Export hive to .reg format",
		Long: `The export command converts a Windows registry hive to .reg text format.
You can export the entire hive or just a specific subtree.

Example:
  hivectl export system.hive system.reg
  hivectl export system.hive system.reg --key "ControlSet001\\Services"
  hivectl export system.hive --stdout > output.reg
  hivectl export system.hive system.reg --encoding utf16le --with-bom`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(args)
		},
	}
	return cmd
}

func runExport(args []string) error {
	hivePath := args[0]
	var outputPath string
	if len(args) > 1 {
		outputPath = args[1]
	}

	// Can't specify both output file and stdout
	if outputPath != "" && exportStdout {
		return fmt.Errorf("cannot specify both output file and --stdout")
	}

	// Need either output file or stdout
	if outputPath == "" && !exportStdout {
		return fmt.Errorf("must specify output file or use --stdout")
	}

	printVerbose("Exporting hive: %s\n", hivePath)
	if exportKey != "" {
		printVerbose("Subtree: %s\n", exportKey)
	}

	// Prepare options
	opts := &hive.ExportOptions{
		SubtreePath: exportKey,
		Encoding:    exportEncoding,
		WithBOM:     exportBOM,
	}

	// Export to string or file
	if exportStdout {
		// Export to stdout
		content, err := hive.ExportRegString(hivePath, opts)
		if err != nil {
			return fmt.Errorf("export failed: %w", err)
		}
		fmt.Print(content)
		return nil
	}

	// Export to file
	printInfo("\nExporting %s to %s...\n", hivePath, outputPath)

	if err := hive.ExportReg(hivePath, outputPath, opts); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	// Get output file size
	if stat, err := os.Stat(outputPath); err == nil {
		size := stat.Size()
		printInfo("  Output size: ")
		if size < 1024 {
			printInfo("%d bytes\n", size)
		} else if size < 1024*1024 {
			printInfo("%.1f KB\n", float64(size)/1024)
		} else {
			printInfo("%.1f MB\n", float64(size)/(1024*1024))
		}
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":     hivePath,
			"output":   outputPath,
			"subtree":  exportKey,
			"encoding": exportEncoding,
			"success":  true,
		}
		return printJSON(result)
	}

	printInfo("\n✓ Export complete\n")

	return nil
}
