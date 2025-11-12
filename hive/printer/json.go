package printer

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/joshuapare/hivekit/pkg/types"
)

const (
	DefaultKeyNameSymbol = "(Default)"
)

// jsonKey represents a registry key in JSON format.
type jsonKey struct {
	Name      string         `json:"name"`
	LastWrite string         `json:"last_write,omitempty"`
	Subkeys   int            `json:"subkeys"`
	Values    int            `json:"values"`
	ValueData map[string]any `json:"value_data,omitempty"`
	Children  []jsonKey      `json:"children,omitempty"`
}

// jsonValue represents a registry value in JSON format.
type jsonValue struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Data any    `json:"data"`
}

// printKeyJSON prints a key in JSON format.
func (p *Printer) printKeyJSON(node types.NodeID, _ string, _ int) error {
	meta, err := p.reader.StatKey(node)
	if err != nil {
		return err
	}

	// Without metadata, just output the name as a string
	if !p.opts.PrintMetadata {
		data, err := json.Marshal(meta.Name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(p.writer, "%s\n", data)
		return err
	}

	key := jsonKey{
		Name:    meta.Name,
		Subkeys: meta.SubkeyN,
		Values:  meta.ValueN,
	}

	if p.opts.ShowTimestamps {
		key.LastWrite = meta.LastWrite.Format("2006-01-02T15:04:05Z07:00")
	}

	// Add values if requested
	if p.opts.ShowValues {
		valueData := make(map[string]any)
		values, err := p.reader.Values(node)
		if err == nil {
			for _, valID := range values {
				valMeta, err := p.reader.StatValue(valID)
				if err != nil {
					continue
				}

				// Decode value
				decoded, err := p.decodeValueJSON(valID, valMeta)
				if err != nil {
					continue
				}

				name := valMeta.Name
				if name == "" {
					name = DefaultKeyNameSymbol
				}

				if p.opts.ShowValueTypes {
					valueData[name] = jsonValue{
						Name: name,
						Type: valMeta.Type.String(),
						Data: decoded,
					}
				} else {
					valueData[name] = decoded
				}
			}
		}
		if len(valueData) > 0 {
			key.ValueData = valueData
		}
	}

	// Marshal and write
	data, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(p.writer, "%s\n", data)
	return err
}

// printValueJSON prints a single value in JSON format.
func (p *Printer) printValueJSON(valID types.ValueID, _ int) error {
	meta, err := p.reader.StatValue(valID)
	if err != nil {
		return err
	}

	// Decode value
	decoded, err := p.decodeValueJSON(valID, meta)
	if err != nil {
		return err
	}

	name := meta.Name
	if name == "" {
		name = DefaultKeyNameSymbol
	}

	var val any
	if p.opts.ShowValueTypes {
		val = jsonValue{
			Name: name,
			Type: meta.Type.String(),
			Data: decoded,
		}
	} else {
		val = map[string]any{
			name: decoded,
		}
	}

	// Marshal and write
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(p.writer, "%s\n", data)
	return err
}

// printTreeJSON recursively prints a subtree in JSON format.
func (p *Printer) printTreeJSON(node types.NodeID, path string, depth int) error {
	// Check depth limit
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return nil
	}

	// Without metadata, collect and print names only
	if !p.opts.PrintMetadata {
		return p.printTreeJSONNamesOnly(node, depth)
	}

	meta, err := p.reader.StatKey(node)
	if err != nil {
		return err
	}

	key := jsonKey{
		Name:    meta.Name,
		Subkeys: meta.SubkeyN,
		Values:  meta.ValueN,
	}

	if p.opts.ShowTimestamps {
		key.LastWrite = meta.LastWrite.Format("2006-01-02T15:04:05Z07:00")
	}

	// Add values if requested
	if p.opts.ShowValues {
		valueData := make(map[string]any)
		values, err := p.reader.Values(node)
		if err == nil {
			for _, valID := range values {
				valMeta, err := p.reader.StatValue(valID)
				if err != nil {
					continue
				}

				// Decode value
				decoded, err := p.decodeValueJSON(valID, valMeta)
				if err != nil {
					continue
				}

				name := valMeta.Name
				if name == "" {
					name = DefaultKeyNameSymbol
				}

				if p.opts.ShowValueTypes {
					valueData[name] = jsonValue{
						Name: name,
						Type: valMeta.Type.String(),
						Data: decoded,
					}
				} else {
					valueData[name] = decoded
				}
			}
		}
		if len(valueData) > 0 {
			key.ValueData = valueData
		}
	}

	// Recursively process children
	children, err := p.reader.Subkeys(node)
	if err == nil && len(children) > 0 {
		key.Children = make([]jsonKey, 0, len(children))
		for _, child := range children {
			childMeta, err := p.reader.StatKey(child)
			if err != nil {
				continue
			}

			childPath := path
			if path != "" {
				childPath += "\\"
			}
			childPath += childMeta.Name

			childKey, err := p.buildJSONTree(child, childPath, depth+1)
			if err != nil {
				continue
			}
			key.Children = append(key.Children, childKey)
		}
	}

	// Marshal and write
	data, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(p.writer, "%s\n", data)
	return err
}

// buildJSONTree builds a JSON tree structure recursively.
func (p *Printer) buildJSONTree(node types.NodeID, path string, depth int) (jsonKey, error) {
	// Check depth limit
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return jsonKey{}, nil
	}

	meta, err := p.reader.StatKey(node)
	if err != nil {
		return jsonKey{}, err
	}

	key := jsonKey{
		Name:    meta.Name,
		Subkeys: meta.SubkeyN,
		Values:  meta.ValueN,
	}

	if p.opts.ShowTimestamps {
		key.LastWrite = meta.LastWrite.Format("2006-01-02T15:04:05Z07:00")
	}

	// Add values if requested
	if p.opts.ShowValues {
		valueData := make(map[string]any)
		values, err := p.reader.Values(node)
		if err == nil {
			for _, valID := range values {
				valMeta, err := p.reader.StatValue(valID)
				if err != nil {
					continue
				}

				// Decode value
				decoded, err := p.decodeValueJSON(valID, valMeta)
				if err != nil {
					continue
				}

				name := valMeta.Name
				if name == "" {
					name = DefaultKeyNameSymbol
				}

				if p.opts.ShowValueTypes {
					valueData[name] = jsonValue{
						Name: name,
						Type: valMeta.Type.String(),
						Data: decoded,
					}
				} else {
					valueData[name] = decoded
				}
			}
		}
		if len(valueData) > 0 {
			key.ValueData = valueData
		}
	}

	// Recursively process children
	children, err := p.reader.Subkeys(node)
	if err == nil && len(children) > 0 {
		key.Children = make([]jsonKey, 0, len(children))
		for _, child := range children {
			childMeta, statErr := p.reader.StatKey(child)
			if statErr != nil {
				continue
			}

			childPath := path
			if path != "" {
				childPath += "\\"
			}
			childPath += childMeta.Name

			childKey, childErr := p.buildJSONTree(child, childPath, depth+1)
			if childErr != nil {
				continue
			}
			key.Children = append(key.Children, childKey)
		}
	}

	return key, nil
}

// printTreeJSONNamesOnly prints only child key names as a JSON array (no metadata).
func (p *Printer) printTreeJSONNamesOnly(node types.NodeID, depth int) error {
	// Check depth limit
	if p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth {
		return nil
	}

	// Get children
	children, err := p.reader.Subkeys(node)
	if err != nil {
		return err
	}

	// Collect names
	names := make([]string, 0, len(children))
	for _, child := range children {
		meta, err := p.reader.StatKey(child)
		if err != nil {
			continue
		}
		names = append(names, meta.Name)
	}

	// Output as JSON array
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(p.writer, "%s\n", data)
	return err
}

// decodeValueJSON decodes a value for JSON output.
func (p *Printer) decodeValueJSON(valID types.ValueID, meta types.ValueMeta) (any, error) {
	switch meta.Type {
	case types.REG_SZ, types.REG_EXPAND_SZ:
		str, err := p.reader.ValueString(valID, types.ReadOptions{})
		if err != nil {
			return nil, err
		}
		return str, nil

	case types.REG_DWORD, types.REG_DWORD_BE:
		val, err := p.reader.ValueDWORD(valID)
		if err != nil {
			return nil, err
		}
		return val, nil

	case types.REG_QWORD:
		val, err := p.reader.ValueQWORD(valID)
		if err != nil {
			return nil, err
		}
		return val, nil

	case types.REG_MULTI_SZ:
		strs, err := p.reader.ValueStrings(valID, types.ReadOptions{})
		if err != nil {
			return nil, err
		}
		return strs, nil

	case types.REG_BINARY, types.REG_NONE:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return nil, err
		}
		maxBytes := p.opts.MaxValueBytes
		if maxBytes == 0 {
			maxBytes = len(data)
		}
		displayLen := min(len(data), maxBytes)
		if displayLen == 0 {
			return "", nil
		}
		hexStr := hex.EncodeToString(data[:displayLen])
		if len(data) > maxBytes {
			hexStr += fmt.Sprintf(" (truncated, %d total bytes)", len(data))
		}
		return hexStr, nil

	default:
		data, err := p.reader.ValueBytes(valID, types.ReadOptions{CopyData: true})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("<%d bytes>", len(data)), nil
	}
}
