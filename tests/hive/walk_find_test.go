package hive_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestFindAndWalk(t *testing.T) {
	buf, offsets := synthHive()
	r, err := reader.OpenBytes(buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	root := hive.NodeID(offsets["root"])
	child := hive.NodeID(offsets["child"])

	rootMeta, err := r.StatKey(root)
	if err != nil {
		t.Fatalf("StatKey root failed: %v", err)
	}
	if rootMeta.Name != "HKEY_LOCAL_MACHINE" {
		t.Fatalf("unexpected root name: %s", rootMeta.Name)
	}

	subkeys, err := r.Subkeys(root)
	if err != nil {
		t.Fatalf("Subkeys failed: %v", err)
	}
	if len(subkeys) != 1 {
		t.Fatalf("expected one subkey, got %d", len(subkeys))
	}
	if subkeys[0] != child {
		t.Fatalf("unexpected subkey id: %v", subkeys[0])
	}
	childMeta, err := r.StatKey(subkeys[0])
	if err != nil {
		t.Fatalf("StatKey child failed: %v", err)
	}
	if childMeta.Name != "SOFTWARE" {
		t.Fatalf("unexpected child name: %s", childMeta.Name)
	}

	if id, findErr := r.Find("SOFTWARE"); findErr != nil || id != child {
		t.Fatalf("Find child without root failed: %v, id=%v", findErr, id)
	}
	if id, findErr := r.Find("HKEY_LOCAL_MACHINE"); findErr != nil || id != root {
		t.Fatalf("Find root failed: %v, id=%v", findErr, id)
	}
	segments := normalizePathForTest("HKEY_LOCAL_MACHINE\\SOFTWARE")
	if len(segments) != 1 || segments[0] != "SOFTWARE" {
		t.Fatalf("unexpected normalized segments: %v", segments)
	}
	if found := searchChildForTest(r, root, "SOFTWARE"); found != child {
		t.Fatalf("search helper failed: got %v want %v", found, child)
	}
	if id, findErr := r.Find("HKEY_LOCAL_MACHINE\\SOFTWARE"); findErr != nil || id != child {
		t.Fatalf("Find child failed: %v, id=%v", findErr, id)
	}
	if id, findErr := r.Find("HKLM\\SOFTWARE"); findErr != nil || id != child {
		t.Fatalf("Find with alias failed: %v, id=%v", findErr, id)
	}
	if _, findErr := r.Find("ROOT\\missing"); !errors.Is(findErr, hive.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", findErr)
	}

	var order []hive.NodeID
	err = r.Walk(root, func(id hive.NodeID) error {
		order = append(order, id)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if len(order) != 2 || order[0] != root || order[1] != child {
		t.Fatalf("unexpected walk order: %v", order)
	}
}

func normalizePathForTest(path string) []string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, `/`, `\`)
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
	if len(out) > 0 {
		upper := strings.ToUpper(out[0])
		switch upper {
		case "HKLM",
			"HKEY_LOCAL_MACHINE",
			"HKCR",
			"HKEY_CLASSES_ROOT",
			"HKCU",
			"HKEY_CURRENT_USER",
			"HKU",
			"HKEY_USERS",
			"HKCC",
			"HKEY_CURRENT_CONFIG":
			out = out[1:]
		}
	}
	return out
}

func searchChildForTest(r hive.Reader, parent hive.NodeID, name string) hive.NodeID {
	subs, _ := r.Subkeys(parent)
	needle := strings.ToLower(name)
	for _, id := range subs {
		meta, _ := r.StatKey(id)
		if strings.ToLower(meta.Name) == needle {
			return id
		}
	}
	return 0
}
