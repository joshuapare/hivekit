package printer

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/pkg/types"
)

// printKeyReg prints a key in Windows .reg file format.
func (p *Printer) printKeyReg(node types.NodeID, path string) error {
	// Write .reg header
	fmt.Fprintf(p.writer, "Windows Registry Editor Version 5.00\n\n")

	// Write key path
	fmt.Fprintf(p.writer, "[%s]\n", normalizeRegPath(path))

	// Print values if requested
	if p.opts.ShowValues {
		values, err := p.reader.Values(node)
		if err != nil {
			return err
		}
		for _, valID := range values {
			if err := p.printValueReg(valID); err != nil {
				return err
			}
		}
	}

	return nil
}

// printTreeReg recursively prints a subtree in .reg file format.
func (p *Printer) printTreeReg(node types.NodeID, path string, depth int) error {
	// Check depth limit
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return nil
	}

	// Write .reg header only for root
	if depth == 0 {
		fmt.Fprintf(p.writer, "Windows Registry Editor Version 5.00\n\n")
	}

	// Write key path
	fmt.Fprintf(p.writer, "[%s]\n", normalizeRegPath(path))

	// Print values if requested
	if p.opts.ShowValues {
		values, err := p.reader.Values(node)
		if err == nil {
			for _, valID := range values {
				if err := p.printValueReg(valID); err != nil {
					// Continue on error
					continue
				}
			}
		}
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

		// Add blank line between keys
		fmt.Fprintln(p.writer)

		if err := p.printTreeReg(child, childPath, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// printValueReg prints a value in .reg file format.
func (p *Printer) printValueReg(valID types.ValueID) error {
	meta, err := p.reader.StatValue(valID)
	if err != nil {
		return err
	}

	// Get value name
	name := meta.Name
	if name == "" {
		name = "@" // Default value uses @ in .reg format
	} else {
		name = fmt.Sprintf("\"%s\"", escapeRegString(name))
	}

	// Encode value based on type
	switch meta.Type {
	case types.REG_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "%s=\"%s\"\n", name, escapeRegString(str))

	case types.REG_EXPAND_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		// REG_EXPAND_SZ is encoded as hex(2): in .reg format
		data := encodeUTF16LE(str)
		fmt.Fprintf(p.writer, "%s=hex(2):%s\n", name, formatHexBytes(data))

	case types.REG_DWORD:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "%s=dword:%08x\n", name, val)

	case types.REG_QWORD:
		val, err := p.reader.ValueQWORD(valID)
		if err != nil {
			return err
		}
		fmt.Fprintf(p.writer, "%s=hex(b):%s\n", name, formatQWORD(val))

	case types.REG_MULTI_SZ:
		strs, err := p.reader.ValueStrings(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		// Encode as UTF-16LE with double null terminator
		var buf []byte
		for _, s := range strs {
			utf16Bytes := encodeUTF16LE(s)
			buf = append(buf, utf16Bytes...)
		}
		// Add final null terminator (double null for MULTI_SZ)
		buf = append(buf, 0, 0)
		fmt.Fprintf(p.writer, "%s=hex(7):%s\n", name, formatHexBytes(buf))

	case types.REG_BINARY:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 || maxBytes > len(data) {
			maxBytes = len(data)
		}
		fmt.Fprintf(p.writer, "%s=hex:%s\n", name, formatHexBytes(data[:maxBytes]))

	case types.REG_NONE:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 || maxBytes > len(data) {
			maxBytes = len(data)
		}
		if len(data) == 0 {
			fmt.Fprintf(p.writer, "%s=hex(0):\n", name)
		} else {
			fmt.Fprintf(p.writer, "%s=hex(0):%s\n", name, formatHexBytes(data[:maxBytes]))
		}

	case types.REG_DWORD_BE:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return err
		}
		// Encode as big-endian bytes
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, val)
		fmt.Fprintf(p.writer, "%s=hex(5):%s\n", name, formatHexBytes(buf))

	default:
		// Unknown type - output as hex with type number
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 || maxBytes > len(data) {
			maxBytes = len(data)
		}
		fmt.Fprintf(p.writer, "%s=hex(%x):%s\n", name, uint32(meta.Type), formatHexBytes(data[:maxBytes]))
	}

	return nil
}

// normalizeRegPath ensures the path has a proper registry root prefix.
func normalizeRegPath(path string) string {
	// If path is empty, use a default root
	if path == "" {
		return "HKEY_LOCAL_MACHINE"
	}

	// Check if path already has a root prefix
	upper := strings.ToUpper(path)
	if strings.HasPrefix(upper, "HKEY_") || strings.HasPrefix(upper, "HK") {
		return path
	}

	// Add HKEY_LOCAL_MACHINE prefix
	return "HKEY_LOCAL_MACHINE\\" + path
}

// escapeRegString escapes special characters in .reg file strings.
func escapeRegString(s string) string {
	// Escape backslashes and quotes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// encodeUTF16LE encodes a string as UTF-16LE with null terminator.
func encodeUTF16LE(s string) []byte {
	runes := []rune(s)
	utf16Codes := utf16.Encode(runes)

	// Allocate buffer with space for null terminator
	buf := make([]byte, (len(utf16Codes)+1)*2)

	// Encode as little-endian
	for i, code := range utf16Codes {
		binary.LittleEndian.PutUint16(buf[i*2:], code)
	}

	// Add null terminator
	buf[len(utf16Codes)*2] = 0
	buf[len(utf16Codes)*2+1] = 0

	return buf
}

// formatHexBytes formats bytes as comma-separated hex values for .reg format.
func formatHexBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, ",")
}

// formatQWORD formats a QWORD as little-endian hex bytes.
func formatQWORD(val uint64) string {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	return formatHexBytes(buf)
}
