//go:build hivex

package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestValueType_BindingsVsReader compares how bindings and reader report value types
func TestValueType_BindingsVsReader(t *testing.T) {
	hivePath := "../../testdata/suite/windows-xp-system"
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

	// Navigate to the problematic key
	// \controlset001\control\deviceclasses\{2eef81be-33fa-4800-9670-1cd474972c3f}\properties\{14c83a99-0b3f-44b7-be4c-a178d3990564}\0003
	hxRoot := hx.Root()
	hxNode := hxRoot

	path := []string{
		"ControlSet001",
		"Control",
		"DeviceClasses",
		"{2eef81be-33fa-4800-9670-1cd474972c3f}",
		"Properties",
		"{14c83a99-0b3f-44b7-be4c-a178d3990564}",
		"0003",
	}

	for _, name := range path {
		hxNode = hx.NodeGetChild(hxNode, name)
		if hxNode == 0 {
			t.Fatalf("Failed to navigate to %s", name)
		}
	}

	// Get the value with empty name
	hxVal := hx.NodeGetValue(hxNode, "")
	if hxVal == 0 {
		t.Fatal("Value not found in hivex")
	}

	hxType, hxLen, err := hx.ValueType(hxVal)
	if err != nil {
		t.Fatalf("Failed to get type from hivex: %v", err)
	}

	// Navigate with our reader
	ourRoot, _ := r.Root()
	ourNode := ourRoot
	for _, name := range path {
		ourNode, err = r.Lookup(ourNode, name)
		if err != nil {
			t.Fatalf("Failed to navigate to %s: %v", name, err)
		}
	}

	// Get values from our reader
	ourValues, err := r.Values(ourNode)
	if err != nil {
		t.Fatalf("Failed to get values: %v", err)
	}

	// Find the value with empty name
	var ourValID types.ValueID
	for _, val := range ourValues {
		meta, _ := r.StatValue(val)
		if meta.Name == "" {
			ourValID = val
			break
		}
	}

	if ourValID == 0 {
		t.Fatal("Value not found in reader")
	}

	ourMeta, err := r.StatValue(ourValID)
	if err != nil {
		t.Fatalf("Failed to get value meta: %v", err)
	}

	t.Logf("\n=== TYPE COMPARISON ===")
	t.Logf("Hivex bindings:")
	t.Logf("  Type (as int32):  %d", int32(hxType))
	t.Logf("  Type (as uint32): %d", uint32(hxType))
	t.Logf("  Type.String():    %s", hxType.String())
	t.Logf("  Length:           %d", hxLen)

	t.Logf("Reader:")
	t.Logf("  Type (as int32):  %d", int32(ourMeta.Type))
	t.Logf("  Type (as uint32): %d", uint32(ourMeta.Type))
	t.Logf("  Type.String():    %s", ourMeta.Type.String())
	t.Logf("  Size:             %d", ourMeta.Size)

	// Check if they match when both treated as uint32
	if uint32(hxType) != uint32(ourMeta.Type) {
		t.Errorf("Type mismatch! Hivex: %d (0x%08x), Reader: %d (0x%08x)",
			uint32(hxType), uint32(hxType),
			uint32(ourMeta.Type), uint32(ourMeta.Type))
	}
}
