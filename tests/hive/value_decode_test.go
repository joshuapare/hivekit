package hive_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestValueDecoders(t *testing.T) {
	buf, offsets := synthHive()
	r, err := reader.OpenBytes(buf, hive.OpenOptions{ZeroCopy: true})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	root := hive.NodeID(offsets["root"])
	vals, err := r.Values(root)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %d", len(vals))
	}

	tests := map[string]func(hive.ValueID) error{
		"Test": func(id hive.ValueID) error {
			meta, err := r.StatValue(id)
			if err != nil {
				return err
			}
			if meta.Type != hive.REG_DWORD {
				return errMeta{meta}
			}
			data, err := r.ValueBytes(id, hive.ReadOptions{CopyData: true})
			if err != nil {
				return err
			}
			if !bytes.Equal(data, []byte{0x78, 0x56, 0x34, 0x12}) {
				return errBytes{data}
			}
			v, err := r.ValueDWORD(id)
			if err != nil {
				return err
			}
			if v != 0x12345678 {
				return errDWORD(v)
			}
			return nil
		},
		"Path": func(id hive.ValueID) error {
			meta, err := r.StatValue(id)
			if err != nil {
				return err
			}
			if meta.Type != hive.REG_SZ {
				return errMeta{meta}
			}
			str, err := r.ValueString(id, hive.ReadOptions{})
			if err != nil {
				return err
			}
			if str != "C:\\Temp" {
				return errString(str)
			}
			return nil
		},
		"Multi": func(id hive.ValueID) error {
			meta, err := r.StatValue(id)
			if err != nil {
				return err
			}
			if meta.Type != hive.REG_MULTI_SZ {
				return errMeta{meta}
			}
			vals, err := r.ValueStrings(id, hive.ReadOptions{})
			if err != nil {
				return err
			}
			if len(vals) != 2 || vals[0] != "One" || vals[1] != "Two" {
				return errStrings{vals}
			}
			return nil
		},
	}

	for _, id := range vals {
		meta, err := r.StatValue(id)
		if err != nil {
			t.Fatalf("StatValue: %v", err)
		}
		testFn, ok := tests[meta.Name]
		if !ok {
			t.Fatalf("unexpected value %q", meta.Name)
		}
		if err := testFn(id); err != nil {
			t.Fatalf("value %s check failed: %v", meta.Name, err)
		}
	}
}

type errMeta struct{ hive.ValueMeta }

func (e errMeta) Error() string { return fmt.Sprintf("unexpected meta: %+v", e.ValueMeta) }

type errBytes struct{ got []byte }

func (e errBytes) Error() string { return fmt.Sprintf("unexpected bytes: %x", e.got) }

type errDWORD uint32

func (e errDWORD) Error() string { return fmt.Sprintf("unexpected DWORD: 0x%x", uint32(e)) }

type errString string

func (e errString) Error() string { return fmt.Sprintf("unexpected string: %s", string(e)) }

type errStrings struct{ vals []string }

func (e errStrings) Error() string { return fmt.Sprintf("unexpected multisz: %v", e.vals) }
