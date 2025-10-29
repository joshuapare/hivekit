package hive_test

import (
	"errors"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestOpenBytesRejectsInvalidHeader(t *testing.T) {
	buf := make([]byte, regfHeaderSize)
	copy(buf, []byte("bad!"))
	_, err := reader.OpenBytes(buf, hive.OpenOptions{})
	if !errors.Is(err, hive.ErrNotHive) {
		t.Fatalf("expected ErrNotHive, got %v", err)
	}
}

func TestOpenBytesRootNode(t *testing.T) {
	buf, offsets := synthHive()
	r, err := reader.OpenBytes(buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}
	defer r.Close()

	root, err := r.Root()
	if err != nil {
		t.Fatalf("Root failed: %v", err)
	}
	if root != hive.NodeID(offsets["root"]) {
		t.Fatalf("unexpected root id: %v", root)
	}

	meta, err := r.StatKey(root)
	if err != nil {
		t.Fatalf("StatKey failed: %v", err)
	}
	if meta.Name != "HKEY_LOCAL_MACHINE" || meta.SubkeyN != 1 || meta.ValueN != 3 {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}
