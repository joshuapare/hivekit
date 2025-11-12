package subkeys

import (
	"testing"
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
