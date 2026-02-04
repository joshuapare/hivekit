package subkeys

import (
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Benchmark_Hash tests the Windows hash function performance.
func Benchmark_Hash(b *testing.B) {
	name := "Software\\Microsoft\\Windows\\CurrentVersion"

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = Hash(name)
	}
}

// Benchmark_List_Insert tests list insertion performance.
func Benchmark_List_Insert(b *testing.B) {
	// Create initial list with some entries
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x100},
			{NameLower: "ccc", NKRef: 0x200},
			{NameLower: "eee", NKRef: 0x300},
		},
	}

	entry := Entry{NameLower: "ddd", NKRef: 0x400}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Insert(entry)
	}
}

// Benchmark_List_Find tests binary search performance.
func Benchmark_List_Find(b *testing.B) {
	// Create a list with 100 entries
	list := &List{Entries: make([]Entry, 100)}
	for i := range 100 {
		list.Entries[i] = Entry{
			NameLower: string(rune('a'+i/26)) + string(rune('a'+i%26)),
			NKRef:     uint32(0x1000 + i*0x100),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = list.Find("mm") // Middle of the list
	}
}

// Benchmark_List_Remove tests removal performance.
func Benchmark_List_Remove(b *testing.B) {
	// Create a list with some entries
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x100},
			{NameLower: "bbb", NKRef: 0x200},
			{NameLower: "ccc", NKRef: 0x300},
			{NameLower: "ddd", NKRef: 0x400},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Remove("ccc")
	}
}

// Benchmark_WriteDBHeader tests DB header writing (pure function).
func Benchmark_readLFLH(b *testing.B) {
	// Create a mock LF header
	buf := make([]byte, 8+100*8) // Header + 100 entries
	buf[0] = 'l'
	buf[1] = 'f'
	// Count: 100
	buf[2] = 100
	buf[3] = 0

	// Fill entries with dummy data
	for i := range 100 {
		offset := 8 + i*8
		// NK offset (4 bytes)
		buf[offset] = byte(i)
		buf[offset+1] = byte(i >> 8)
		buf[offset+2] = 0
		buf[offset+3] = 0
		// Hash (4 bytes)
		buf[offset+4] = 0xAB
		buf[offset+5] = 0xCD
		buf[offset+6] = 0xEF
		buf[offset+7] = 0x12
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = readLFLH(buf, 100)
	}
}

// Benchmark_readLI tests LI list reading.
func Benchmark_readLI(b *testing.B) {
	// Create a mock LI header
	buf := make([]byte, 8+100*4) // Header + 100 refs
	buf[0] = 'l'
	buf[1] = 'i'
	// Count: 100
	buf[2] = 100
	buf[3] = 0

	// Fill refs
	for i := range 100 {
		offset := 8 + i*4
		buf[offset] = byte(i)
		buf[offset+1] = byte(i >> 8)
		buf[offset+2] = 0
		buf[offset+3] = 0
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = readLI(buf, 100)
	}
}

// Benchmark_decodeCompressedName tests ASCII decoding.
func Benchmark_decodeCompressedName(b *testing.B) {
	nameBytes := []byte("SoftwareMicrosoftWindowsCurrentVersion")

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = decodeCompressedName(nameBytes)
	}
}

// Benchmark_decodeUTF16LEName tests UTF-16LE decoding.
func Benchmark_decodeUTF16LEName(b *testing.B) {
	// "Software" in UTF-16LE
	nameBytes := []byte{
		'S', 0, 'o', 0, 'f', 0, 't', 0, 'w', 0, 'a', 0, 'r', 0, 'e', 0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = decodeUTF16LEName(nameBytes)
	}
}

// Benchmark_decodeCompressedNameLowerWithHashes benchmarks the fused decode+lowercase+hash.
func Benchmark_decodeCompressedNameLowerWithHashes(b *testing.B) {
	nameBytes := []byte("SoftwareMicrosoftWindowsCurrentVersion")

	b.ReportAllocs()

	for range b.N {
		_, _, _, _ = decodeCompressedNameLowerWithHashes(nameBytes)
	}
}

// Benchmark_compressedNameEqualsLower_Match benchmarks targeted name matching (hit).
func Benchmark_compressedNameEqualsLower_Match(b *testing.B) {
	nameBytes := []byte("CurrentVersion")
	target := "currentversion"

	b.ReportAllocs()

	for range b.N {
		_ = compressedNameEqualsLower(nameBytes, target)
	}
}

// Benchmark_compressedNameEqualsLower_Mismatch benchmarks targeted name matching (miss).
func Benchmark_compressedNameEqualsLower_Mismatch(b *testing.B) {
	nameBytes := []byte("CurrentVersion")
	target := "othervalue"

	b.ReportAllocs()

	for range b.N {
		_ = compressedNameEqualsLower(nameBytes, target)
	}
}

// Benchmark_compressedNameEqualsLower_LengthMismatch benchmarks early length rejection.
func Benchmark_compressedNameEqualsLower_LengthMismatch(b *testing.B) {
	nameBytes := []byte("CurrentVersion")
	target := "cv"

	b.ReportAllocs()

	for range b.N {
		_ = compressedNameEqualsLower(nameBytes, target)
	}
}

// Benchmark_utf16NameEqualsLower_Match benchmarks UTF-16 targeted matching (hit).
func Benchmark_utf16NameEqualsLower_Match(b *testing.B) {
	// "Software" in UTF-16LE
	nameBytes := []byte{
		'S', 0, 'o', 0, 'f', 0, 't', 0, 'w', 0, 'a', 0, 'r', 0, 'e', 0,
	}
	target := "software"

	b.ReportAllocs()

	for range b.N {
		_ = utf16NameEqualsLower(nameBytes, target)
	}
}

// Benchmark_utf16NameEqualsLower_Mismatch benchmarks UTF-16 targeted matching (miss).
func Benchmark_utf16NameEqualsLower_Mismatch(b *testing.B) {
	// "Software" in UTF-16LE
	nameBytes := []byte{
		'S', 0, 'o', 0, 'f', 0, 't', 0, 'w', 0, 'a', 0, 'r', 0, 'e', 0,
	}
	target := "hardware"

	b.ReportAllocs()

	for range b.N {
		_ = utf16NameEqualsLower(nameBytes, target)
	}
}

// Benchmark_ReadNKEntry_Repeated decodes the same common registry names 1000x
// to show per-call allocation cost. Cross-hive optimization should reduce
// allocs/op for repeated names via interning.
func Benchmark_ReadNKEntry_Repeated(b *testing.B) {
	commonNames := [][]byte{
		[]byte("Software"),
		[]byte("Microsoft"),
		[]byte("Windows"),
		[]byte("CurrentVersion"),
		[]byte("ControlSet001"),
		[]byte("Services"),
		[]byte("Control"),
		[]byte("Parameters"),
		[]byte("Explorer"),
		[]byte("Run"),
	}

	b.ReportAllocs()

	for range b.N {
		for range 100 {
			for _, name := range commonNames {
				_, _, _, _ = decodeCompressedNameLowerWithHashes(name)
			}
		}
	}
}

// findServicesSubkeyList navigates root → ControlSet001 → Services and returns
// the Services NK's subkey list offset. Used by benchmarks to target a key
// with 150+ children.
func findServicesSubkeyList(b *testing.B, h *hive.Hive) uint32 {
	b.Helper()

	// Navigate from root to ControlSet001 → Services
	path := []string{"controlset001", "services"}
	currentRef := h.RootCellOffset()

	for _, segment := range path {
		payload, err := h.ResolveCellPayload(currentRef)
		if err != nil {
			b.Fatalf("resolve NK at 0x%X: %v", currentRef, err)
		}
		nk, err := hive.ParseNK(payload)
		if err != nil {
			b.Fatalf("parse NK at 0x%X: %v", currentRef, err)
		}

		listRef := nk.SubkeyListOffsetRel()
		if listRef == format.InvalidOffset {
			b.Fatalf("NK at 0x%X has no subkey list", currentRef)
		}

		// Read the subkey list and find the matching child
		list, err := Read(h, listRef)
		if err != nil {
			b.Fatalf("Read subkey list at 0x%X: %v", listRef, err)
		}

		found := false
		for _, entry := range list.Entries {
			if strings.EqualFold(entry.NameLower, segment) {
				currentRef = entry.NKRef
				found = true
				break
			}
		}
		if !found {
			b.Fatalf("child %q not found under NK at 0x%X", segment, currentRef)
		}
	}

	// Now currentRef points to Services NK — get its subkey list offset
	payload, err := h.ResolveCellPayload(currentRef)
	if err != nil {
		b.Fatalf("resolve Services NK: %v", err)
	}
	nk, err := hive.ParseNK(payload)
	if err != nil {
		b.Fatalf("parse Services NK: %v", err)
	}

	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		b.Fatal("Services NK has no subkey list")
	}
	return listRef
}

// Benchmark_SubkeysDecode_LargeList reads the subkey list of
// ControlSet001\Services (150+ entries) repeatedly, measuring
// the decode cost of a large sibling list.
func Benchmark_SubkeysDecode_LargeList(b *testing.B) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	h, err := hive.Open(testHivePath)
	if err != nil {
		b.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	servicesListRef := findServicesSubkeyList(b, h)

	// Verify we have a large list
	list, err := Read(h, servicesListRef)
	if err != nil {
		b.Fatalf("Read services list: %v", err)
	}
	b.Logf("Services subkey count: %d", len(list.Entries))

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		list, _ := Read(h, servicesListRef)
		_ = list
	}
}

