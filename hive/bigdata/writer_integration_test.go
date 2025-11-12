package bigdata

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
)

// Test_Writer_SingleBlock tests storing data that fits in one block.
func Test_Writer_SingleBlock(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	// Create test hive with plenty of space
	createTestHive(t, hivePath, 100*1024) // 100KB

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	// Test data that fits in one block
	testData := bytes.Repeat([]byte("A"), 1000)
	dbHeaderRef, err := writer.Store(testData)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if dbHeaderRef == 0 {
		t.Fatal("Expected non-zero DB header reference")
	}

	t.Logf("Stored data, dbHeaderRef=0x%X", dbHeaderRef)

	// Check what's at the cell location
	data := h.Bytes()
	t.Logf("Hive size: %d bytes (0x%X)", len(data), len(data))
	cellOffset := int(0x1000 + dbHeaderRef)
	t.Logf("Cell offset: 0x%X", cellOffset)
	if cellOffset+16 > len(data) {
		t.Fatalf("Cell offset 0x%X is beyond hive size 0x%X", cellOffset, len(data))
	}
	t.Logf("Cell header at 0x%X: %x", cellOffset, data[cellOffset:cellOffset+4])
	t.Logf("Cell payload at 0x%X: %x", cellOffset+4, data[cellOffset+4:cellOffset+16])

	// Verify the DB header was written correctly
	// HCELL_INDEX is relative to 0x1000, skip 4-byte cell header to get to payload
	offset := 0x1000 + dbHeaderRef + 4
	t.Logf(
		"dbHeaderRef=0x%X, offset=0x%X, data at offset: %x",
		dbHeaderRef,
		offset,
		data[offset:offset+12],
	)
	header, err := ReadDBHeader(data[offset:])
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != 1 {
		t.Errorf("Expected 1 block, got %d", header.Count)
	}

	// Read blocklist (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	blocklistData := data[0x1000+header.Blocklist+4:]
	blockRefs, err := ReadBlocklist(blocklistData, header.Count)
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	if len(blockRefs) != 1 {
		t.Errorf("Expected 1 block ref, got %d", len(blockRefs))
	}

	// Read and verify data (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	blockData := data[0x1000+blockRefs[0]+4:]
	if !bytes.Equal(blockData[:len(testData)], testData) {
		t.Error("Stored data doesn't match original")
	}
}

// Test_Writer_MultipleBlocks tests storing data that requires chunking.
func Test_Writer_MultipleBlocks(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	// Create test hive with plenty of space
	createTestHive(t, hivePath, 500*1024) // 500KB

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	// Test data requiring 3 blocks (MaxBlockSize * 2.5)
	testData := bytes.Repeat([]byte("B"), MaxBlockSize*2+MaxBlockSize/2)
	expectedBlocks := 3

	dbHeaderRef, err := writer.Store(testData)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify the DB header (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	data := h.Bytes()
	header, err := ReadDBHeader(data[0x1000+dbHeaderRef+4:])
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != uint16(expectedBlocks) {
		t.Errorf("Expected %d blocks, got %d", expectedBlocks, header.Count)
	}

	// Read blocklist (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	blocklistData := data[0x1000+header.Blocklist+4:]
	blockRefs, err := ReadBlocklist(blocklistData, header.Count)
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	if len(blockRefs) != expectedBlocks {
		t.Errorf("Expected %d block refs, got %d", expectedBlocks, len(blockRefs))
	}

	// Reconstruct and verify data (HCELL_INDEX is relative to 0x1000, skip 4-byte cell headers)
	var reconstructed []byte
	for i, ref := range blockRefs {
		blockData := data[0x1000+ref+4:]

		// Calculate expected block size
		expectedSize := MaxBlockSize
		if i == expectedBlocks-1 {
			// Last block may be partial
			expectedSize = len(testData) - (i * MaxBlockSize)
		}

		reconstructed = append(reconstructed, blockData[:expectedSize]...)
	}

	if !bytes.Equal(reconstructed, testData) {
		t.Error("Reconstructed data doesn't match original")
	}
}

// Test_Writer_EmptyData tests error handling for empty data.
func Test_Writer_EmptyData(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createTestHive(t, hivePath, 10*1024)

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	_, err = writer.Store([]byte{})
	if !errors.Is(err, ErrEmptyData) {
		t.Errorf("Expected ErrEmptyData, got %v", err)
	}
}

// Test_Writer_ExactlyMaxBlock tests data exactly at MaxBlockSize.
func Test_Writer_ExactlyMaxBlock(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createTestHive(t, hivePath, 100*1024)

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	// Test data exactly MaxBlockSize
	testData := bytes.Repeat([]byte("C"), MaxBlockSize)
	dbHeaderRef, err := writer.Store(testData)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should be exactly 1 block (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	data := h.Bytes()
	header, err := ReadDBHeader(data[0x1000+dbHeaderRef+4:])
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != 1 {
		t.Errorf("Expected 1 block for MaxBlockSize data, got %d", header.Count)
	}
}

// Test_Writer_MaxBlockPlusOne tests boundary condition.
func Test_Writer_MaxBlockPlusOne(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createTestHive(t, hivePath, 100*1024)

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	// Test data MaxBlockSize + 1 (should create 2 blocks)
	testData := bytes.Repeat([]byte("D"), MaxBlockSize+1)
	dbHeaderRef, err := writer.Store(testData)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	data := h.Bytes()
	header, err := ReadDBHeader(data[0x1000+dbHeaderRef+4:])
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != 2 {
		t.Errorf("Expected 2 blocks for MaxBlockSize+1 data, got %d", header.Count)
	}

	// Verify blocklist has 2 refs (HCELL_INDEX is relative to 0x1000, skip 4-byte cell header)
	blocklistData := data[0x1000+header.Blocklist+4:]
	blockRefs, err := ReadBlocklist(blocklistData, header.Count)
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	if len(blockRefs) != 2 {
		t.Errorf("Expected 2 block refs, got %d", len(blockRefs))
	}

	// Verify data (HCELL_INDEX is relative to 0x1000, skip 4-byte cell headers)
	var reconstructed []byte
	reconstructed = append(
		reconstructed,
		data[0x1000+blockRefs[0]+4:0x1000+blockRefs[0]+4+MaxBlockSize]...)
	reconstructed = append(reconstructed, data[0x1000+blockRefs[1]+4:0x1000+blockRefs[1]+4+1]...)

	if !bytes.Equal(reconstructed, testData) {
		t.Error("Reconstructed data doesn't match original")
	}
}

// Test_Writer_TenBlocks tests larger data requiring many blocks.
func Test_Writer_TenBlocks(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	// Need much more space for 10 blocks
	createTestHive(t, hivePath, 1024*1024) // 1MB

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(h, allocator, noopDT{})

	// Test data requiring exactly 10 blocks
	testData := bytes.Repeat([]byte("E"), MaxBlockSize*10)
	dbHeaderRef, err := writer.Store(testData)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	data := h.Bytes()
	header, err := ReadDBHeader(data[0x1000+dbHeaderRef+4:])
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != 10 {
		t.Errorf("Expected 10 blocks, got %d", header.Count)
	}
}
