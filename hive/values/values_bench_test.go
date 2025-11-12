package values

import (
	"testing"
)

// Benchmark_List_Append tests appending to a value list.
func Benchmark_List_Append(b *testing.B) {
	list := &List{
		VKRefs: []uint32{0x100, 0x200, 0x300, 0x400, 0x500},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Append(0x600)
	}
}

// Benchmark_List_Remove tests removing from a value list.
func Benchmark_List_Remove(b *testing.B) {
	list := &List{
		VKRefs: []uint32{0x100, 0x200, 0x300, 0x400, 0x500},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Remove(0x300)
	}
}

// Benchmark_List_Find tests finding in a value list.
func Benchmark_List_Find(b *testing.B) {
	list := &List{
		VKRefs: []uint32{0x100, 0x200, 0x300, 0x400, 0x500},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Find(0x300)
	}
}

// Benchmark_List_Len tests length calculation.
func Benchmark_List_Len(b *testing.B) {
	list := &List{
		VKRefs: []uint32{0x100, 0x200, 0x300, 0x400, 0x500},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Len()
	}
}

// Benchmark_parseValueList tests parsing a value list array.
func Benchmark_parseValueList(b *testing.B) {
	// Create a buffer with 100 VK refs
	buf := make([]byte, 100*4)
	for i := range 100 {
		offset := i * 4
		ref := uint32(0x1000 + i*0x100)
		buf[offset] = byte(ref)
		buf[offset+1] = byte(ref >> 8)
		buf[offset+2] = byte(ref >> 16)
		buf[offset+3] = byte(ref >> 24)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = parseValueList(buf, 100)
	}
}

// Benchmark_List_Append_Large tests appending to a large list.
func Benchmark_List_Append_Large(b *testing.B) {
	// Create a list with 100 entries
	refs := make([]uint32, 100)
	for i := range 100 {
		refs[i] = uint32(0x1000 + i*0x100)
	}
	list := &List{VKRefs: refs}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Append(0xFFFF)
	}
}

// Benchmark_List_Remove_Large tests removing from a large list.
func Benchmark_List_Remove_Large(b *testing.B) {
	// Create a list with 100 entries
	refs := make([]uint32, 100)
	for i := range 100 {
		refs[i] = uint32(0x1000 + i*0x100)
	}
	list := &List{VKRefs: refs}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Remove(0x5000) // Middle of list
	}
}

// Benchmark_List_Find_Large tests finding in a large list.
func Benchmark_List_Find_Large(b *testing.B) {
	// Create a list with 100 entries
	refs := make([]uint32, 100)
	for i := range 100 {
		refs[i] = uint32(0x1000 + i*0x100)
	}
	list := &List{VKRefs: refs}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = list.Find(0x5000) // Middle of list
	}
}
