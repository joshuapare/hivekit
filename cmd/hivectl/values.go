package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/spf13/cobra"
)

var (
	valuesShowData bool
	valuesShowType bool
	valuesHex      bool
)

func init() {
	cmd := newValuesCmd()
	cmd.Flags().BoolVar(&valuesShowData, "show-data", true, "Show full value data")
	cmd.Flags().BoolVar(&valuesShowType, "show-type", true, "Show registry type")
	cmd.Flags().BoolVar(&valuesHex, "hex", false, "Show numeric values as hex")
	rootCmd.AddCommand(cmd)
}

func newValuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values <hive> <path>",
		Short: "List all values at a registry key",
		Long: `The values command lists all values at a specific registry key.

Example:
  hivectl values system.hive "ControlSet001"
  hivectl values system.hive "Software" --json
  hivectl values system.hive "ControlSet001\\Services"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValues(args)
		},
	}
	return cmd
}

func runValues(args []string) error {
	hivePath := args[0]
	keyPath := args[1]

	printVerbose("Opening hive: %s\n", hivePath)

	// Open hive with new backend
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer h.Close()

	// Get reader
	reader, err := h.Reader()
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}

	// Find the key
	keyID, err := h.Find(keyPath)
	if err != nil {
		return fmt.Errorf("failed to find key: %w", err)
	}

	// Get values
	values, err := reader.Values(keyID)
	if err != nil {
		return fmt.Errorf("failed to get values: %w", err)
	}

	// Sort values by name
	sort.Slice(values, func(i, j int) bool {
		mi, _ := reader.StatValue(values[i])
		mj, _ := reader.StatValue(values[j])
		return mi.Name < mj.Name
	})

	// Handle JSON output
	if jsonOut {
		return outputValuesJSON(reader, values)
	}

	// Text output in .reg format (pipeline-friendly)
	return outputValuesText(reader, values)
}

func outputValuesText(reader types.Reader, values []types.ValueID) error {
	var buf bytes.Buffer
	for _, vid := range values {
		meta, err := reader.StatValue(vid)
		if err != nil {
			continue
		}
		// Use regtext emitValue function for consistent .reg formatting
		if err := regtext.EmitValue(&buf, reader, vid, meta); err != nil {
			return err
		}
	}
	_, err := os.Stdout.Write(buf.Bytes())
	return err
}

func outputValuesJSON(reader types.Reader, values []types.ValueID) error {
	result := make(map[string]interface{})
	for _, vid := range values {
		meta, err := reader.StatValue(vid)
		if err != nil {
			continue
		}

		name := meta.Name
		if name == "" {
			name = "(Default)"
		}

		// Decode value based on type
		var data interface{}
		switch meta.Type {
		case types.REG_SZ, types.REG_EXPAND_SZ:
			str, err := reader.ValueString(vid, types.ReadOptions{})
			if err == nil {
				data = str
			}
		case types.REG_DWORD, types.REG_DWORD_BE:
			val, err := reader.ValueDWORD(vid)
			if err == nil {
				data = val
			}
		case types.REG_QWORD:
			val, err := reader.ValueQWORD(vid)
			if err == nil {
				data = val
			}
		case types.REG_MULTI_SZ:
			strs, err := reader.ValueStrings(vid, types.ReadOptions{})
			if err == nil {
				data = strs
			}
		default:
			bytes, err := reader.ValueBytes(vid, types.ReadOptions{CopyData: true})
			if err == nil {
				data = fmt.Sprintf("<%d bytes>", len(bytes))
			}
		}

		if valuesShowType {
			result[name] = map[string]interface{}{
				"type": meta.Type.String(),
				"data": data,
			}
		} else {
			result[name] = data
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
