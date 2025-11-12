package regtext

import (
	"testing"
	"unsafe"

	"github.com/joshuapare/hivekit/pkg/types"
)

func TestStructSizes(t *testing.T) {
	t.Logf("EditOp struct sizes:")
	t.Logf("  OpSetValue:     %d bytes", unsafe.Sizeof(types.OpSetValue{}))
	t.Logf("  OpDeleteValue:  %d bytes", unsafe.Sizeof(types.OpDeleteValue{}))
	t.Logf("  OpCreateKey:    %d bytes", unsafe.Sizeof(types.OpCreateKey{}))
	t.Logf("  OpDeleteKey:    %d bytes", unsafe.Sizeof(types.OpDeleteKey{}))

	t.Logf("\nField sizes:")
	t.Logf("  string:         %d bytes (header only)", unsafe.Sizeof(""))
	t.Logf("  []byte:         %d bytes (header only)", unsafe.Sizeof([]byte{}))
	t.Logf("  RegType:        %d bytes", unsafe.Sizeof(types.REG_SZ))
	t.Logf("  bool:           %d bytes", unsafe.Sizeof(true))

	// Example instances with typical data
	exampleSetValue := types.OpSetValue{
		Path: "HKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows\\CurrentVersion",
		Name: "ProgramFilesDir",
		Type: types.REG_SZ,
		Data: make([]byte, 50), // Typical UTF-16LE string
	}

	exampleCreateKey := types.OpCreateKey{
		Path: "HKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows\\CurrentVersion",
	}

	t.Logf("\nExample instances (struct + string data):")
	t.Logf("  OpSetValue struct: %d bytes", unsafe.Sizeof(exampleSetValue))
	t.Logf("    + Path string:   %d bytes", len(exampleSetValue.Path))
	t.Logf("    + Name string:   %d bytes", len(exampleSetValue.Name))
	t.Logf("    + Data slice:    %d bytes", len(exampleSetValue.Data))
	t.Logf("    Total:          ~%d bytes",
		int(unsafe.Sizeof(exampleSetValue))+len(exampleSetValue.Path)+
			len(exampleSetValue.Name)+len(exampleSetValue.Data))

	t.Logf("  OpCreateKey struct: %d bytes", unsafe.Sizeof(exampleCreateKey))
	t.Logf("    + Path string:    %d bytes", len(exampleCreateKey.Path))
	t.Logf("    Total:           ~%d bytes",
		int(unsafe.Sizeof(exampleCreateKey))+len(exampleCreateKey.Path))
}
