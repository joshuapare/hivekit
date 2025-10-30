package reg_test

import (
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestParseReg(t *testing.T) {
	reg := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Vendor]",
		"\"Path\"=\"C\\\\Temp\"",
		"\"DWORD\"=dword:0000002a",
		"\"Binary\"=hex:01,02,0a",
		"@=\"Default\"",
		"",
		"[-HKEY_LOCAL_MACHINE\\SOFTWARE\\Obsolete]",
		"",
	}, "\r\n")

	ops, err := regtext.ParseReg([]byte(reg), hive.RegParseOptions{})
	if err != nil {
		t.Fatalf("ParseReg: %v", err)
	}
	if len(ops) != 6 {
		t.Fatalf("expected 6 ops, got %d", len(ops))
	}

	if _, ok := ops[0].(hive.OpCreateKey); !ok {
		t.Fatalf("first op not create key: %T", ops[0])
	}
	if set, ok := ops[1].(hive.OpSetValue); !ok || set.Type != hive.REG_SZ {
		t.Fatalf("expected REG_SZ set value, got %T", ops[1])
	}
	if set, ok := ops[2].(hive.OpSetValue); !ok || set.Type != hive.REG_DWORD {
		t.Fatalf("expected REG_DWORD, got %T", ops[2])
	}
	// Default behavior strips root key (HKEY_LOCAL_MACHINE\)
	if del, ok := ops[len(ops)-1].(hive.OpDeleteKey); !ok || del.Path != "SOFTWARE\\Obsolete" {
		t.Fatalf("unexpected delete op: %#v", ops[len(ops)-1])
	}
}

func TestExportRegRoundTrip(t *testing.T) {
	buf, offsets := buildSampleHive()
	r, err := reader.OpenBytes(buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	root := hive.NodeID(offsets["root"])
	out, err := regtext.ExportReg(r, root, hive.RegExportOptions{})
	if err != nil {
		t.Fatalf("ExportReg: %v", err)
	}
	str := string(out)
	if !strings.Contains(str, "[HKEY_LOCAL_MACHINE\\SOFTWARE]") {
		t.Fatalf("missing section: %s", str)
	}
	ops, err := regtext.ParseReg(out, hive.RegParseOptions{})
	if err != nil {
		t.Fatalf("ParseReg roundtrip: %v", err)
	}
	hasPath := false
	for _, op := range ops {
		if set, ok := op.(hive.OpSetValue); ok && set.Name == "Path" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Fatalf("expected Path value in exported reg: %s", str)
	}
}
