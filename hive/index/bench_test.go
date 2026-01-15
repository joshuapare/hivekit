package index

import (
	"math/rand"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/hive"
)

// ============================================================================
// Real Hive Data Extraction
// ============================================================================

// hiveData holds extracted NK/VK offsets from a real hive file.
type hiveData struct {
	nks   [][3]uint32 // [parentOff, offset, _] for NKs
	vks   [][3]uint32 // [parentOff, offset, _] for VKs
	names []string    // Decoded NK/VK names
}

// loadHiveData scans a real hive file and extracts all NK/VK relationships.
// Returns the data needed to build an index.
func loadHiveData(t testing.TB, hivePath string) *hiveData {
	t.Helper()

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("failed to open hive %s: %v", hivePath, err)
	}
	defer h.Close()

	data := &hiveData{
		nks:   make([][3]uint32, 0, 10000),
		vks:   make([][3]uint32, 0, 40000),
		names: make([]string, 0, 50000),
	}

	// Walk the hive inline and extract all NK/VK relationships
	visited := make(map[uint32]bool)
	nameIdx := 0

	// Inline walker: recursively walk NK cells starting from root
	var walkNK func(uint32) error
	walkNK = func(offset uint32) error {
		if visited[offset] {
			return nil
		}
		visited[offset] = true

		// Extract this NK
		if extractErr := extractNK(h, offset, data, &nameIdx); extractErr != nil {
			return extractErr
		}

		// Parse NK to get subkeys and values
		payload, resolveErr := h.ResolveCellPayload(offset)
		if resolveErr != nil {
			return nil //nolint:nilerr // Skip malformed cells in benchmark data extraction
		}
		nk, parseErr := hive.ParseNK(payload)
		if parseErr != nil {
			return nil //nolint:nilerr // Skip malformed cells in benchmark data extraction
		}

		// Walk subkeys recursively
		subkeyListOffset := nk.SubkeyListOffsetRel()
		if subkeyListOffset != 0 && subkeyListOffset != 0xFFFFFFFF && !visited[subkeyListOffset] {
			visited[subkeyListOffset] = true

			listPayload, listResolveErr := h.ResolveCellPayload(subkeyListOffset)
			if listResolveErr == nil && len(listPayload) >= 2 {
				// Extract subkey offsets based on list type
				sig := string(listPayload[0:2])
				subkeyOffsets := extractSubkeyOffsets(h, listPayload, sig)

				// Recursively walk each subkey
				for _, childOffset := range subkeyOffsets {
					if walkErr := walkNK(childOffset); walkErr != nil {
						return walkErr
					}
				}
			}
		}

		// Extract VKs for this NK
		valueCount := nk.ValueCount()
		if valueCount > 0 {
			valueListOffset := nk.ValueListOffsetRel()
			if valueListOffset != 0 && valueListOffset != 0xFFFFFFFF {
				listPayload, valueListResolveErr := h.ResolveCellPayload(valueListOffset)
				if valueListResolveErr == nil && len(listPayload) >= int(valueCount)*4 {
					// Value list is array of VK offsets
					for i := range valueCount {
						vkOffset := uint32(listPayload[i*4]) | uint32(listPayload[i*4+1])<<8 |
							uint32(listPayload[i*4+2])<<16 | uint32(listPayload[i*4+3])<<24
						if vkOffset != 0 && vkOffset != 0xFFFFFFFF {
							extractVK(h, vkOffset, data, &nameIdx)
						}
					}
				}
			}
		}

		return nil
	}

	// Start from root
	rootOffset := h.RootCellOffset()
	if walkErr := walkNK(rootOffset); walkErr != nil {
		t.Fatalf("failed to walk hive: %v", walkErr)
	}

	t.Logf("Loaded hive %s: %d NKs, %d VKs, %d unique names",
		filepath.Base(hivePath), len(data.nks), len(data.vks), len(data.names))

	return data
}

// extractSubkeyOffsets extracts child NK offsets from a subkey list (LF/LH/LI/RI).
func extractSubkeyOffsets(h *hive.Hive, listPayload []byte, sig string) []uint32 {
	if len(listPayload) < 4 {
		return nil
	}

	count := uint16(listPayload[2]) | uint16(listPayload[3])<<8
	offsets := make([]uint32, 0, count)

	switch sig {
	case "lf", "lh", "li":
		// Direct list: [sig(2)][count(2)][entries...]
		entrySize := 8 // offset(4) + hash/name(4)
		for i := range count {
			entryOffset := 4 + int(i)*entrySize
			if entryOffset+4 > len(listPayload) {
				break
			}
			childOffset := uint32(listPayload[entryOffset]) | uint32(listPayload[entryOffset+1])<<8 |
				uint32(listPayload[entryOffset+2])<<16 | uint32(listPayload[entryOffset+3])<<24
			if childOffset != 0 && childOffset != 0xFFFFFFFF {
				offsets = append(offsets, childOffset)
			}
		}

	case "ri":
		// Indirect list: [sig(2)][count(2)][sublist_offsets...]
		for i := range count {
			sublistOffset := 4 + int(i)*4
			if sublistOffset+4 > len(listPayload) {
				break
			}
			sublistRef := uint32(listPayload[sublistOffset]) | uint32(listPayload[sublistOffset+1])<<8 |
				uint32(listPayload[sublistOffset+2])<<16 | uint32(listPayload[sublistOffset+3])<<24

			if sublistRef != 0 && sublistRef != 0xFFFFFFFF {
				// Recursively extract from sub-list
				subPayload, err := h.ResolveCellPayload(sublistRef)
				if err == nil && len(subPayload) >= 2 {
					subSig := string(subPayload[0:2])
					subOffsets := extractSubkeyOffsets(h, subPayload, subSig)
					offsets = append(offsets, subOffsets...)
				}
			}
		}
	}

	return offsets
}

func extractNK(h *hive.Hive, offset uint32, data *hiveData, nameIdx *int) error {
	payload, err := h.ResolveCellPayload(offset)
	if err != nil {
		return nil //nolint:nilerr // Skip malformed cells in benchmark data extraction
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return nil //nolint:nilerr // Skip malformed cells in benchmark data extraction
	}

	// Extract parent and name
	parentOff := nk.ParentOffsetRel()
	name := decodeNKName(nk)

	// Store: [parentOff, offset, nameIdx]
	data.nks = append(data.nks, [3]uint32{parentOff, offset, uint32(*nameIdx)})
	data.names = append(data.names, name)
	*nameIdx++

	return nil
}

func extractVK(h *hive.Hive, offset uint32, data *hiveData, nameIdx *int) {
	// For VKs, we need to find the parent NK by walking backwards
	// For benchmarking, we'll use offset 0 as a placeholder parent
	// In a real implementation, you'd track parent during tree traversal

	payload, err := h.ResolveCellPayload(offset)
	if err != nil {
		return // Skip malformed cells in benchmark data extraction
	}

	vk, err := hive.ParseVK(payload)
	if err != nil {
		return // Skip malformed cells in benchmark data extraction
	}

	name := decodeVKName(vk)

	// Store: [parentOff=0, offset, nameIdx]
	// Note: In real use, you'd pass parent context through the walker
	data.vks = append(data.vks, [3]uint32{0, offset, uint32(*nameIdx)})
	data.names = append(data.names, name)
	*nameIdx++
}

func decodeNKName(nk hive.NK) string {
	nameBytes := nk.Name()
	if len(nameBytes) == 0 {
		return ""
	}
	if nk.IsCompressedName() {
		return string(nameBytes) // ASCII
	}
	return decodeUTF16LE(nameBytes)
}

func decodeVKName(vk hive.VK) string {
	nameBytes := vk.Name()
	if len(nameBytes) == 0 {
		return "" // (Default) value
	}
	if vk.NameCompressed() {
		return string(nameBytes)
	}
	return decodeUTF16LE(nameBytes)
}

func decodeUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		return string(b) // Malformed, fallback
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return string(utf16.Decode(u16))
}

// ============================================================================
// Benchmarks
// ============================================================================

var testHives = []string{
	"../../testdata/suite/windows-xp-system",
	"../../testdata/suite/windows-8-enterprise-system",
}

// Benchmark_Build_StringIndex measures index build time using real hive data.
func Benchmark_Build_StringIndex(b *testing.B) {
	for _, hivePath := range testHives {
		data := loadHiveData(b, hivePath)
		name := filepath.Base(hivePath)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				idx := NewStringIndex(len(data.nks), len(data.vks))
				for _, nk := range data.nks {
					parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
					idx.AddNK(parentOff, data.names[nameIdx], offset)
				}
				for _, vk := range data.vks {
					parentOff, offset, nameIdx := vk[0], vk[1], vk[2]
					idx.AddVK(parentOff, data.names[nameIdx], offset)
				}
			}
		})
	}
}

// Benchmark_Build_UniqueIndex measures index build time with interning.
func Benchmark_Build_UniqueIndex(b *testing.B) {
	for _, hivePath := range testHives {
		data := loadHiveData(b, hivePath)
		name := filepath.Base(hivePath)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				idx := NewUniqueIndex(len(data.nks), len(data.vks))
				for _, nk := range data.nks {
					parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
					idx.AddNK(parentOff, data.names[nameIdx], offset)
				}
				for _, vk := range data.vks {
					parentOff, offset, nameIdx := vk[0], vk[1], vk[2]
					idx.AddVK(parentOff, data.names[nameIdx], offset)
				}
			}
		})
	}
}

// Benchmark_Build_NumericIndex measures index build time with numeric keys.
// This is the allocation-optimized implementation using uint64 map keys.
func Benchmark_Build_NumericIndex(b *testing.B) {
	for _, hivePath := range testHives {
		data := loadHiveData(b, hivePath)
		name := filepath.Base(hivePath)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				idx := NewNumericIndex(len(data.nks), len(data.vks))
				for _, nk := range data.nks {
					parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
					idx.AddNK(parentOff, data.names[nameIdx], offset)
				}
				for _, vk := range data.vks {
					parentOff, offset, nameIdx := vk[0], vk[1], vk[2]
					idx.AddVK(parentOff, data.names[nameIdx], offset)
				}
			}
		})
	}
}

// Benchmark_Lookup_Hot measures repeated lookups of the same keys (cache warm).
func Benchmark_Lookup_Hot_StringIndex(b *testing.B) {
	benchLookupHot(b, testHives[0], func(nkCap, vkCap int) ReadWriteIndex {
		return NewStringIndex(nkCap, vkCap)
	})
}

func Benchmark_Lookup_Hot_UniqueIndex(b *testing.B) {
	benchLookupHot(b, testHives[0], func(nkCap, vkCap int) ReadWriteIndex {
		return NewUniqueIndex(nkCap, vkCap)
	})
}

func benchLookupHot(b *testing.B, hivePath string, newIdx func(int, int) ReadWriteIndex) {
	data := loadHiveData(b, hivePath)

	// Build index once
	idx := newIdx(len(data.nks), len(data.vks))
	for _, nk := range data.nks {
		parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
		idx.AddNK(parentOff, data.names[nameIdx], offset)
	}

	// Pick 100 random NKs to lookup repeatedly
	rng := rand.New(rand.NewSource(42))
	hotKeys := make([][2]uint32, 100) // [parentOff, nameIdx]
	for i := range hotKeys {
		nk := data.nks[rng.Intn(len(data.nks))]
		hotKeys[i] = [2]uint32{nk[0], nk[2]}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for _, key := range hotKeys {
			parentOff, nameIdx := key[0], key[1]
			_, ok := idx.GetNK(parentOff, data.names[nameIdx])
			if !ok {
				b.Fatal("lookup failed")
			}
		}
	}
}

// Benchmark_Lookup_Cold measures random lookups across all keys (cache cold).
func Benchmark_Lookup_Cold_StringIndex(b *testing.B) {
	benchLookupCold(b, testHives[0], func(nkCap, vkCap int) ReadWriteIndex {
		return NewStringIndex(nkCap, vkCap)
	})
}

func Benchmark_Lookup_Cold_UniqueIndex(b *testing.B) {
	benchLookupCold(b, testHives[0], func(nkCap, vkCap int) ReadWriteIndex {
		return NewUniqueIndex(nkCap, vkCap)
	})
}

func benchLookupCold(b *testing.B, hivePath string, newIdx func(int, int) ReadWriteIndex) {
	data := loadHiveData(b, hivePath)

	// Build index once
	idx := newIdx(len(data.nks), len(data.vks))
	for _, nk := range data.nks {
		parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
		idx.AddNK(parentOff, data.names[nameIdx], offset)
	}

	// Pre-shuffle NK indices for random access
	rng := rand.New(rand.NewSource(42))
	indices := rng.Perm(len(data.nks))

	b.ReportAllocs()
	b.ResetTimer()

	idx2 := 0
	for range b.N {
		nk := data.nks[indices[idx2]]
		parentOff, nameIdx := nk[0], nk[2]
		_, ok := idx.GetNK(parentOff, data.names[nameIdx])
		if !ok {
			b.Fatal("lookup failed")
		}
		idx2 = (idx2 + 1) % len(indices)
	}
}

// Benchmark_Merge simulates a real merge scenario:
//  1. Build index from hive1
//  2. Build index from hive2
//  3. Lookup all keys from hive1 in hive2 (simulates conflict detection)
func Benchmark_Merge_StringIndex(b *testing.B) {
	benchMerge(b, func(nkCap, vkCap int) ReadWriteIndex {
		return NewStringIndex(nkCap, vkCap)
	})
}

func Benchmark_Merge_UniqueIndex(b *testing.B) {
	benchMerge(b, func(nkCap, vkCap int) ReadWriteIndex {
		return NewUniqueIndex(nkCap, vkCap)
	})
}

func benchMerge(b *testing.B, newIdx func(int, int) ReadWriteIndex) {
	if len(testHives) < 2 {
		b.Skip("need at least 2 test hives for merge benchmark")
	}

	data1 := loadHiveData(b, testHives[0])
	data2 := loadHiveData(b, testHives[1])

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		// Build index1
		idx1 := newIdx(len(data1.nks), len(data1.vks))
		for _, nk := range data1.nks {
			parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
			idx1.AddNK(parentOff, data1.names[nameIdx], offset)
		}

		// Build index2
		idx2 := newIdx(len(data2.nks), len(data2.vks))
		for _, nk := range data2.nks {
			parentOff, offset, nameIdx := nk[0], nk[1], nk[2]
			idx2.AddNK(parentOff, data2.names[nameIdx], offset)
		}

		// Cross-lookup: check if keys from hive1 exist in hive2
		found := 0
		for _, nk := range data1.nks {
			parentOff, nameIdx := nk[0], nk[2]
			if _, ok := idx2.GetNK(parentOff, data1.names[nameIdx]); ok {
				found++
			}
		}

		// Prevent optimization
		_ = found
	}
}
