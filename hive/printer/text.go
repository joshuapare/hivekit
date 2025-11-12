package printer

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/types"
)

// printKeyText prints a key in human-readable text format.
func (p *Printer) printKeyText(node types.NodeID, depth int) error {
	meta, err := p.reader.StatKey(node)
	if err != nil {
		return err
	}

	indent := strings.Repeat(" ", depth*p.opts.IndentSize)

	// Print key name
	fmt.Fprintf(p.writer, "%s[%s]\n", indent, meta.Name)

	// Print timestamp if requested
	if p.opts.ShowTimestamps {
		fmt.Fprintf(p.writer, "%s  Last Write: %s\n", indent, meta.LastWrite.Format("2006-01-02 15:04:05"))
	}

	// Print metadata
	fmt.Fprintf(p.writer, "%s  Subkeys: %d, Values: %d\n", indent, meta.SubkeyN, meta.ValueN)

	// Print values if requested
	if p.opts.ShowValues {
		values, err := p.reader.Values(node)
		if err != nil {
			return err
		}
		for _, valID := range values {
			if err := p.printValueText(valID, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// printValueText prints a value in human-readable text format.
func (p *Printer) printValueText(valID types.ValueID, depth int) error {
	meta, err := p.reader.StatValue(valID)
	if err != nil {
		return err
	}

	indent := strings.Repeat(" ", depth*p.opts.IndentSize)

	// Format: "  Name" = type : value
	name := meta.Name
	if name == "" {
		name = "(Default)"
	}

	fmt.Fprintf(p.writer, "%s\"%s\"", indent, name)

	if p.opts.ShowValueTypes {
		fmt.Fprintf(p.writer, " [%s]", meta.Type)
	}

	fmt.Fprintf(p.writer, " = ")

	// Decode value based on type
	switch meta.Type {
	case types.REG_SZ, types.REG_EXPAND_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "\"%s\"\n", str)

	case types.REG_DWORD, types.REG_DWORD_BE:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "0x%08X (%d)\n", val, val)

	case types.REG_QWORD:
		val, err := p.reader.ValueQWORD(valID)
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "0x%016X (%d)\n", val, val)

	case types.REG_MULTI_SZ:
		strs, err := p.reader.ValueStrings(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		if len(strs) == 0 {
			fmt.Fprintf(p.writer, "[]\n")
		} else {
			fmt.Fprintf(p.writer, "[\n")
			for _, s := range strs {
				fmt.Fprintf(p.writer, "%s  \"%s\"\n", indent, s)
			}
			fmt.Fprintf(p.writer, "%s]\n", indent)
		}

	case types.REG_BINARY, types.REG_NONE:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 {
			maxBytes = len(data)
		}
		displayLen := min(len(data), maxBytes)
		truncated := ""
		if len(data) > maxBytes {
			truncated = fmt.Sprintf(" (truncated, %d total bytes)", len(data))
		}
		if displayLen == 0 {
			fmt.Fprintf(p.writer, "<empty>%s\n", truncated)
		} else {
			fmt.Fprintf(p.writer, "%X%s\n", data[:displayLen], truncated)
		}

	default:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "<%d bytes>\n", len(data))
	}

	return nil
}

// printTreeText recursively prints a subtree in text format.
func (p *Printer) printTreeText(node types.NodeID, path string, depth int) error {
	// Check depth limit
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return nil
	}

	// Print current key
	if err := p.printKeyText(node, depth); err != nil {
		return err
	}

	// Print children recursively
	children, err := p.reader.Subkeys(node)
	if err != nil {
		return err
	}

	for _, child := range children {
		meta, err := p.reader.StatKey(child)
		if err != nil {
			continue // Skip corrupted keys
		}

		childPath := path
		if path != "" {
			childPath += "\\"
		}
		childPath += meta.Name

		// Add blank line between keys for readability
		fmt.Fprintln(p.writer)

		if err := p.printTreeText(child, childPath, depth+1); err != nil {
			return err
		}
	}

	return nil
}
