package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/spf13/cobra"
)

var (
	exportKey       string
	exportEncoding  string
	exportBOM       bool
	exportStdout    bool
	exportWrapLines bool
)

func init() {
	cmd := newExportCmd()
	cmd.Flags().StringVar(&exportKey, "key", "", "Export only specific subtree path")
	cmd.Flags().StringVar(&exportEncoding, "encoding", "utf8", "Output encoding (utf8, utf16le)")
	cmd.Flags().BoolVar(&exportBOM, "with-bom", false, "Include byte-order mark")
	cmd.Flags().BoolVar(&exportStdout, "stdout", false, "Write to stdout instead of file")
	cmd.Flags().BoolVar(&exportWrapLines, "wrap-lines", false, "Wrap long hex values at 80 characters with backslash continuation")
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

	// Check for UTF-16LE encoding (not yet supported)
	if exportEncoding != "" && exportEncoding != "utf8" && exportEncoding != "UTF-8" {
		return fmt.Errorf("encoding %q not yet supported (only UTF-8 is currently supported)", exportEncoding)
	}

	// Open hive with new backend
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer h.Close()

	// Configure printer options
	opts := printer.DefaultOptions()
	opts.Format = printer.FormatReg
	opts.ShowValues = true
	opts.MaxDepth = 0       // unlimited depth
	opts.WrapLines = exportWrapLines // Enable line wrapping if flag is set

	// Determine output writer
	var writer *os.File
	if exportStdout {
		writer = os.Stdout
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		writer = f
	}

	// Export using printer
	path := exportKey
	if path == "" {
		// Export entire hive starting from root
		path = ""
	}

	if err := h.PrintTree(writer, path, opts); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	// Output as JSON if requested and not writing to stdout
	if !exportStdout && jsonOut {
		result := map[string]interface{}{
			"hive":     hivePath,
			"output":   outputPath,
			"subtree":  exportKey,
			"encoding": "utf8",
			"success":  true,
		}
		return printJSON(result)
	}

	return nil
}
