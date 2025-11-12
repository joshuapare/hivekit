package reader

import (
	"errors"
	"strings"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

var rootAliasMap = map[string][]string{
	"HKEY_LOCAL_MACHINE":  {"HKLM"},
	"HKEY_CLASSES_ROOT":   {"HKCR"},
	"HKEY_CURRENT_USER":   {"HKCU"},
	"HKEY_USERS":          {"HKU"},
	"HKEY_CURRENT_CONFIG": {"HKCC"},
}

var rootAliasList = []string{
	"HKEY_LOCAL_MACHINE", "HKLM",
	"HKEY_CLASSES_ROOT", "HKCR",
	"HKEY_CURRENT_USER", "HKCU",
	"HKEY_USERS", "HKU",
	"HKEY_CURRENT_CONFIG", "HKCC",
}

func (r *reader) Find(path string) (types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	path = strings.TrimSpace(path)
	path = stripRootPrefix(path)
	segments := normalizePath(path)
	current := types.NodeID(r.head.RootCellOffset)
	if len(segments) == 0 {
		return current, nil
	}

	rootNK, err := r.nk(current)
	if err != nil {
		return 0, err
	}
	rootName, err := DecodeKeyName(rootNK)
	if err != nil {
		return 0, wrapFormatErr(err)
	}
	if len(segments) > 0 && (strings.EqualFold(segments[0], rootName) || aliasMatches(rootName, segments[0])) {
		segments = segments[1:]
	}

	for _, seg := range segments {
		subs, subErr := r.Subkeys(current)
		if subErr != nil {
			return 0, subErr
		}
		needle := strings.ToLower(seg)
		matched := false
		for _, child := range subs {
			meta, statErr := r.StatKey(child)
			if statErr != nil {
				return 0, statErr
			}
			if strings.ToLower(meta.Name) == needle {
				current = child
				matched = true
				break
			}
		}
		if !matched {
			return 0, types.ErrNotFound
		}
	}
	return current, nil
}

func (r *reader) Walk(id types.NodeID, fn func(types.NodeID) error) error {
	if err := r.ensureOpen(); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("reader: nil walk callback")
	}
	return r.walk(id, fn)
}

func (r *reader) walk(id types.NodeID, fn func(types.NodeID) error) error {
	if err := fn(id); err != nil {
		return err
	}
	nk, err := r.nk(id)
	if err != nil {
		return err
	}
	if nk.SubkeyCount == 0 || nk.SubkeyListOffset == format.InvalidOffset {
		return nil
	}
	list, err := r.subkeyList(nk.SubkeyListOffset, nk.SubkeyCount)
	if err != nil {
		return err
	}
	for _, off := range list {
		if walkErr := r.walk(types.NodeID(off), fn); walkErr != nil {
			return walkErr
		}
	}
	return nil
}

func normalizePath(path string) []string {
	if path == "" || path == `\` || path == "/" {
		return nil
	}
	path = strings.ReplaceAll(path, "/", `\`)
	path = strings.TrimPrefix(path, `\`)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, `\`)
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func stripRootPrefix(path string) string {
	upper := strings.ToUpper(path)
	for _, alias := range rootAliasList {
		if upper == alias {
			return ""
		}
		prefix := alias + `\`
		if strings.HasPrefix(upper, prefix) {
			return path[len(alias)+1:]
		}
	}
	return path
}

func aliasMatches(rootName, seg string) bool {
	for canon, aliases := range rootAliasMap {
		if !strings.EqualFold(rootName, canon) {
			continue
		}
		for _, alias := range aliases {
			if strings.EqualFold(seg, alias) {
				return true
			}
		}
		break
	}
	return false
}
