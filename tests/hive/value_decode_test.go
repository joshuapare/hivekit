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
			meta, statErr := r.StatValue(id)
			if statErr != nil {
				return statErr
			}
			if meta.Type != hive.REG_DWORD {
				return metaError{meta}
			}
			data, readErr := r.ValueBytes(id, hive.ReadOptions{CopyData: true})
			if readErr != nil {
				return readErr
			}
			if !bytes.Equal(data, []byte{0x78, 0x56, 0x34, 0x12}) {
				return bytesError{data}
			}
			v, dwordErr := r.ValueDWORD(id)
			if dwordErr != nil {
				return dwordErr
			}
			if v != 0x12345678 {
				return dwordError(v)
			}
			return nil
		},
		"Path": func(id hive.ValueID) error {
			meta, statErr := r.StatValue(id)
			if statErr != nil {
				return statErr
			}
			if meta.Type != hive.REG_SZ {
				return metaError{meta}
			}
			str, stringErr := r.ValueString(id, hive.ReadOptions{})
			if stringErr != nil {
				return stringErr
			}
			if str != "C:\\Temp" {
				return stringError(str)
			}
			return nil
		},
		"Multi": func(id hive.ValueID) error {
			meta, statErr := r.StatValue(id)
			if statErr != nil {
				return statErr
			}
			if meta.Type != hive.REG_MULTI_SZ {
				return metaError{meta}
			}
			vals, stringsErr := r.ValueStrings(id, hive.ReadOptions{})
			if stringsErr != nil {
				return stringsErr
			}
			if len(vals) != 2 || vals[0] != "One" || vals[1] != "Two" {
				return stringsError{vals}
			}
			return nil
		},
	}

	for _, id := range vals {
		meta, statErr := r.StatValue(id)
		if statErr != nil {
			t.Fatalf("StatValue: %v", statErr)
		}
		testFn, ok := tests[meta.Name]
		if !ok {
			t.Fatalf("unexpected value %q", meta.Name)
		}
		if testErr := testFn(id); testErr != nil {
			t.Fatalf("value %s check failed: %v", meta.Name, testErr)
		}
	}
}

type metaError struct{ hive.ValueMeta }

func (e metaError) Error() string { return fmt.Sprintf("unexpected meta: %+v", e.ValueMeta) }

type bytesError struct{ got []byte }

func (e bytesError) Error() string { return fmt.Sprintf("unexpected bytes: %x", e.got) }

type dwordError uint32

func (e dwordError) Error() string { return fmt.Sprintf("unexpected DWORD: 0x%x", uint32(e)) }

type stringError string

func (e stringError) Error() string { return "unexpected string: " + string(e) }

type stringsError struct{ vals []string }

func (e stringsError) Error() string { return fmt.Sprintf("unexpected multisz: %v", e.vals) }
