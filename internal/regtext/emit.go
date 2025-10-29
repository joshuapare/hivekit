package regtext

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/joshuapare/hivekit/pkg/types"
)

// ExportReg walks a subtree and emits textual .reg output.
func ExportReg(r types.Reader, root types.NodeID, opts types.RegExportOptions) ([]byte, error) {
	meta, err := r.StatKey(root)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(RegFileHeader + CRLF + CRLF)
	if err := exportKey(&buf, r, root, []string{meta.Name}); err != nil {
		return nil, err
	}
	switch strings.ToUpper(opts.OutputEncoding) {
	case "", EncodingUTF8:
		return buf.Bytes(), nil
	case EncodingUTF16LE:
		return encodeUTF16LE(buf.String(), opts.WithBOM), nil
	default:
		return nil, errUnsupportedEncoding
	}
}

func exportKey(buf *bytes.Buffer, r types.Reader, id types.NodeID, path []string) error {
	full := strings.Join(path, Backslash)
	buf.WriteString(KeyOpenBracket)
	buf.WriteString(full)
	buf.WriteString(KeyCloseBracket + CRLF)

	values, err := r.Values(id)
	if err != nil {
		return err
	}
	sort.Slice(values, func(i, j int) bool {
		mi, _ := r.StatValue(values[i])
		mj, _ := r.StatValue(values[j])
		return mi.Name < mj.Name
	})

	for _, vid := range values {
		meta, err := r.StatValue(vid)
		if err != nil {
			return err
		}
		if err := emitValue(buf, r, vid, meta); err != nil {
			return err
		}
	}
	buf.WriteString(CRLF)

	subkeys, err := r.Subkeys(id)
	if err != nil {
		return err
	}
	type child struct {
		name string
		id   types.NodeID
	}
	children := make([]child, 0, len(subkeys))
	for _, sid := range subkeys {
		meta, err := r.StatKey(sid)
		if err != nil {
			return err
		}
		children = append(children, child{name: meta.Name, id: sid})
	}
	sort.Slice(children, func(i, j int) bool {
		return strings.ToLower(children[i].name) < strings.ToLower(children[j].name)
	})
	for _, c := range children {
		if err := exportKey(buf, r, c.id, append(path, c.name)); err != nil {
			return err
		}
	}
	return nil
}

func emitValue(buf *bytes.Buffer, r types.Reader, id types.ValueID, meta types.ValueMeta) error {
	if meta.Name == "" {
		buf.WriteString(DefaultValuePrefix)
	} else {
		buf.WriteString(Quote)
		buf.WriteString(escapeString(meta.Name))
		buf.WriteString(Quote + ValueAssignment)
	}

	switch meta.Type {
	case types.REG_SZ, types.REG_EXPAND_SZ:
		str, err := r.ValueString(id, types.ReadOptions{})
		if err != nil {
			return err
		}
		buf.WriteString(Quote)
		buf.WriteString(escapeString(str))
		buf.WriteString(Quote)
	case types.REG_MULTI_SZ:
		vals, err := r.ValueStrings(id, types.ReadOptions{})
		if err != nil {
			return err
		}
		buf.WriteString(HexMultiSZPrefix)
		buf.WriteString(formatHex(encodeMultiString(vals)))
	case types.REG_DWORD:
		dw, err := r.ValueDWORD(id)
		if err != nil {
			return err
		}
		buf.WriteString(DWORDPrefix)
		fmt.Fprintf(buf, DWORDHexFormat, dw)
	case types.REG_DWORD_BE:
		data, err := r.ValueBytes(id, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		buf.WriteString(HexPrefix)
		buf.WriteString(formatHex(data))
	default:
		data, err := r.ValueBytes(id, types.ReadOptions{CopyData: true})
		if err != nil {
			return err
		}
		buf.WriteString(HexPrefix)
		buf.WriteString(formatHex(data))
	}
	buf.WriteString(CRLF)
	return nil
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, Backslash, EscapedBackslash)
	s = strings.ReplaceAll(s, Quote, EscapedQuote)
	return s
}

func formatHex(data []byte) string {
	if len(data) == 0 {
		return "00"
	}
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf(HexByteFormat, b)
	}
	return strings.Join(parts, HexByteSeparator)
}

func encodeMultiString(values []string) []byte {
	var buf bytes.Buffer
	for _, v := range values {
		buf.Write(encodeUTF16LEZeroTerminated(v))
	}
	buf.Write(DoubleNullTerminator)
	return buf.Bytes()
}
