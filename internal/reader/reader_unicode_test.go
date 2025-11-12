//go:build hivex

package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestUnicode_BindingsVsReader compares unicode name handling in bindings vs reader
func TestUnicode_BindingsVsReader(t *testing.T) {
	hivePath := "../../testdata/special"
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("Hive not found: %s", hivePath)
	}

	// Open with hivex bindings
	hx, err := bindings.Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Failed to open with hivex: %v", err)
	}
	defer hx.Close()

	// Open with our reader
	data, err := os.ReadFile(hivePath)
	if err != nil {
		t.Fatalf("Failed to read hive: %v", err)
	}
	r, err := OpenBytes(data, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open with reader: %v", err)
	}
	defer r.Close()

	// Get root from both
	hxRoot := hx.Root()
	ourRoot, _ := r.Root()

	// Get children from hivex bindings
	hxChildren := hx.NodeChildren(hxRoot)
	t.Logf("Hivex found %d children", len(hxChildren))

	// Get children from our reader
	ourChildren, _ := r.Subkeys(ourRoot)
	t.Logf("Reader found %d children", len(ourChildren))

	// Compare names
	for _, hxChild := range hxChildren {
		hxName := hx.NodeName(hxChild)
		t.Logf("Hivex child: %q (bytes: %v)", hxName, []byte(hxName))
	}

	for _, ourChild := range ourChildren {
		meta, _ := r.StatKey(ourChild)
		t.Logf("Reader child: %q (bytes: %v)", meta.Name, []byte(meta.Name))
	}

	// Find the unicode key specifically
	var hxUnicodeKey bindings.NodeHandle
	for _, hxChild := range hxChildren {
		name := hx.NodeName(hxChild)
		if len(name) > 5 && name[:5] == "abcd_" {
			hxUnicodeKey = hxChild
			t.Logf("\nFound unicode key in hivex: %q", name)
			break
		}
	}

	var ourUnicodeKey types.NodeID
	for _, ourChild := range ourChildren {
		meta, _ := r.StatKey(ourChild)
		if len(meta.Name) > 5 && meta.Name[:5] == "abcd_" {
			ourUnicodeKey = ourChild
			t.Logf("Found unicode key in reader: %q", meta.Name)
			break
		}
	}

	if hxUnicodeKey == 0 || ourUnicodeKey == 0 {
		t.Fatal("Unicode key not found in one or both implementations")
	}

	// Compare the names
	hxName := hx.NodeName(hxUnicodeKey)
	ourMeta, _ := r.StatKey(ourUnicodeKey)

	t.Logf("\n=== COMPARISON ===")
	t.Logf("Hivex bindings:  %q", hxName)
	t.Logf("Reader:          %q", ourMeta.Name)
	t.Logf("Hivex bytes:     % x", []byte(hxName))
	t.Logf("Reader bytes:    % x", []byte(ourMeta.Name))

	if hxName != ourMeta.Name {
		t.Errorf("Name mismatch! Hivex has correct unicode, reader does not")
	}
}
