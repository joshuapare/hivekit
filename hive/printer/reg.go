package printer

import (
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/pkg/types"
)

// printKeyReg prints a key in Windows .reg file format.
func (p *Printer) printKeyReg(node types.NodeID, path string) error {
	// Write .reg header
	io.WriteString(p.writer, "Windows Registry Editor Version 5.00\n\n")

	// Write key path
	normalizedPath := normalizeRegPath(path)
	var sb strings.Builder
	sb.Grow(len(normalizedPath) + 3) // "[" + path + "]\n"
	sb.WriteByte('[')
	sb.WriteString(normalizedPath)
	sb.WriteString("]\n")
	io.WriteString(p.writer, sb.String())

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
	// Write .reg header only for root
	if depth == 0 {
		io.WriteString(p.writer, "Windows Registry Editor Version 5.00\n\n")
	}

	// Print blank line before each key except the root
	if depth > 0 {
		io.WriteString(p.writer, "\n")
	}

	// Write key path
	normalizedPath := normalizeRegPath(path)
	var sb strings.Builder
	sb.Grow(len(normalizedPath) + 3) // "[" + path + "]\n"
	sb.WriteByte('[')
	sb.WriteString(normalizedPath)
	sb.WriteString("]\n")
	io.WriteString(p.writer, sb.String())

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

	// Check depth limit for recursion (but still print current node above)
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return nil
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

		if err := p.printTreeReg(child, childPath, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// buildPrefix builds a .reg format prefix string efficiently without allocations.
// Format: name=hex(type): or name=hex:
func buildPrefix(name string, hexType string) string {
	// Pre-calculate exact size: name + "=hex" + optional "(type)" + ":"
	size := len(name) + 5 // "=hex:"
	if hexType != "" {
		size += len(hexType) + 2 // "(type)"
	}

	var sb strings.Builder
	sb.Grow(size)
	sb.WriteString(name)
	sb.WriteString("=hex")
	if hexType != "" {
		sb.WriteByte('(')
		sb.WriteString(hexType)
		sb.WriteByte(')')
	}
	sb.WriteByte(':')
	return sb.String()
}

// writeValueLine writes a value line efficiently to the writer.
// Format: prefix + hexData + newline
// Builds complete line in a single string to minimize write calls.
func (p *Printer) writeValueLine(prefix string, hexData string) {
	// Build complete line in one allocation
	var sb strings.Builder
	sb.Grow(len(prefix) + len(hexData) + 1)
	sb.WriteString(prefix)
	sb.WriteString(hexData)
	sb.WriteByte('\n')
	io.WriteString(p.writer, sb.String())
}

// formatHexForReg formats hex bytes for .reg output, with optional wrapping.
func (p *Printer) formatHexForReg(data []byte, prefix string) string {
	if p.opts.WrapLines {
		return formatHexBytesWithWrapping(data, prefix)
	}
	return formatHexBytes(data)
}

// printValueReg prints a value in .reg file format.
func (p *Printer) printValueReg(valID types.ValueID) error {
	meta, err := p.reader.StatValue(valID)
	if err != nil {
		return err
	}

	// Get value name
	var name string
	if meta.Name == "" {
		name = "@" // Default value uses @ in .reg format
	} else {
		escapedName := escapeRegString(meta.Name)
		// Build quoted name: "escapedName"
		var sb strings.Builder
		sb.Grow(len(escapedName) + 2)
		sb.WriteByte('"')
		sb.WriteString(escapedName)
		sb.WriteByte('"')
		name = sb.String()
	}

	// Encode value based on type
	switch meta.Type {
	case types.REG_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		// REG_SZ is encoded as hex(1): in .reg format (UTF-16LE)
		data := encodeUTF16LE(str)
		prefix := buildPrefix(name, "1")
		p.writeValueLine(prefix, p.formatHexForReg(data, prefix))

	case types.REG_EXPAND_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return err
		}
		// REG_EXPAND_SZ is encoded as hex(2): in .reg format
		data := encodeUTF16LE(str)
		prefix := buildPrefix(name, "2")
		p.writeValueLine(prefix, p.formatHexForReg(data, prefix))

	case types.REG_DWORD:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return err
		}
		// Build complete dword line: name=dword:XXXXXXXX
		var sb strings.Builder
		sb.Grow(len(name) + 15) // "=dword:" + 8 hex chars + "\n"
		sb.WriteString(name)
		sb.WriteString("=dword:")
		// Format hex manually to avoid fmt.Sprintf
		const hexChars = "0123456789abcdef"
		for shift := 28; shift >= 0; shift -= 4 {
			sb.WriteByte(hexChars[(val>>shift)&0xF])
		}
		sb.WriteByte('\n')
		io.WriteString(p.writer, sb.String())

	case types.REG_QWORD:
		val, err := p.reader.ValueQWORD(valID)
		if err != nil {
			return err
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, val)
		prefix := buildPrefix(name, "b")
		p.writeValueLine(prefix, p.formatHexForReg(buf, prefix))

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
		prefix := buildPrefix(name, "7")
		p.writeValueLine(prefix, p.formatHexForReg(buf, prefix))

	case types.REG_BINARY:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 || maxBytes > len(data) {
			maxBytes = len(data)
		}
		prefix := buildPrefix(name, "")
		p.writeValueLine(prefix, p.formatHexForReg(data[:maxBytes], prefix))

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
			// Build complete empty line
			var sb strings.Builder
			sb.Grow(len(name) + 9) // "=hex(0):\n"
			sb.WriteString(name)
			sb.WriteString("=hex(0):\n")
			io.WriteString(p.writer, sb.String())
		} else {
			prefix := buildPrefix(name, "0")
			p.writeValueLine(prefix, p.formatHexForReg(data[:maxBytes], prefix))
		}

	case types.REG_DWORD_BE:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return err
		}
		// Encode as big-endian bytes
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, val)
		prefix := buildPrefix(name, "5")
		p.writeValueLine(prefix, p.formatHexForReg(buf, prefix))

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
		// Build hex type string manually to avoid sprintf
		var hexType strings.Builder
		hexType.Grow(8) // Enough for "ffffffff"
		typeVal := uint32(meta.Type)
		const hexChars = "0123456789abcdef"
		// Write hex digits (minimum formatting, no padding)
		if typeVal == 0 {
			hexType.WriteByte('0')
		} else {
			// Find first non-zero digit
			shift := 28
			for shift >= 0 && (typeVal>>(shift))&0xF == 0 {
				shift -= 4
			}
			// Write remaining digits
			for shift >= 0 {
				hexType.WriteByte(hexChars[(typeVal>>shift)&0xF])
				shift -= 4
			}
		}
		prefix := buildPrefix(name, hexType.String())
		p.writeValueLine(prefix, p.formatHexForReg(data[:maxBytes], prefix))
	}

	return nil
}

// normalizeRegPath converts paths to .reg file format (leading backslash, no HKEY prefix).
func normalizeRegPath(path string) string {
	// If path is empty, return root
	if path == "" {
		return "\\"
	}

	// Strip HKEY prefixes if present
	// Find backslash position to determine prefix length
	backslashPos := strings.IndexByte(path, '\\')
	if backslashPos == -1 {
		// No backslash, check if it's just a root key name
		switch strings.ToUpper(path) {
		case "HKEY_LOCAL_MACHINE", "HKLM", "HKEY_CURRENT_USER", "HKCU",
			"HKEY_CLASSES_ROOT", "HKCR", "HKEY_USERS", "HKU",
			"HKEY_CURRENT_CONFIG", "HKCC":
			return "\\"
		}
		// Not a root key, add leading backslash
		return "\\" + path
	}

	// Check prefix before first backslash
	prefix := strings.ToUpper(path[:backslashPos])
	switch prefix {
	case "HKEY_LOCAL_MACHINE", "HKLM", "HKEY_CURRENT_USER", "HKCU",
		"HKEY_CLASSES_ROOT", "HKCR", "HKEY_USERS", "HKU",
		"HKEY_CURRENT_CONFIG", "HKCC":
		// Strip the prefix and keep the rest
		path = path[backslashPos+1:]
	}

	// Add leading backslash if not present
	if !strings.HasPrefix(path, "\\") {
		path = "\\" + path
	}

	return path
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

	// Pre-allocate string builder with exact size needed: "XX," per byte, minus last comma
	var sb strings.Builder
	sb.Grow(len(data)*3 - 1)

	const hexChars = "0123456789abcdef"
	for i, b := range data {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte(hexChars[b>>4])
		sb.WriteByte(hexChars[b&0x0F])
	}
	return sb.String()
}

// formatHexBytesWithWrapping formats bytes as comma-separated hex values with line wrapping.
// Lines are wrapped at 80 characters using backslash continuation.
// The prefix parameter is used to calculate the first line length (e.g., "\"Name\"=hex(1):").
func formatHexBytesWithWrapping(data []byte, prefix string) string {
	if len(data) == 0 {
		return ""
	}

	const maxLineLength = 80
	const continuationIndent = "  " // 2 spaces

	var sb strings.Builder
	// Estimate size: prefix + data*3 + some overhead for continuations
	sb.Grow(len(prefix) + len(data)*3 + 100)

	currentLineLength := len(prefix)
	const hexChars = "0123456789abcdef"

	for i, b := range data {
		// Calculate what we're about to add: "," + "XX" = 3 chars (or just "XX" = 2 chars for first byte)
		var charsToAdd int
		if i > 0 {
			charsToAdd = 3 // ",XX"
		} else {
			charsToAdd = 2 // "XX"
		}

		// Check if adding this would exceed line length
		// Account for ",\" (2 chars) that we'll add if we need to continue on next line
		// Only wrap if we're not on the first byte
		if i > 0 && currentLineLength+charsToAdd+2 > maxLineLength {
			// Add continuation backslash and newline
			sb.WriteString(",\\")
			sb.WriteByte('\n')
			sb.WriteString(continuationIndent)

			// Reset line length to indent + current hex value (no comma on continuation line start)
			currentLineLength = len(continuationIndent) + 2

			// Write the hex byte without leading comma
			sb.WriteByte(hexChars[b>>4])
			sb.WriteByte(hexChars[b&0x0F])
		} else {
			// Normal case: add comma if not first byte
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte(hexChars[b>>4])
			sb.WriteByte(hexChars[b&0x0F])
			currentLineLength += charsToAdd
		}
	}

	return sb.String()
}
