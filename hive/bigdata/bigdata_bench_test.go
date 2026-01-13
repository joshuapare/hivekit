package bigdata

import (
	"testing"
)

// Benchmark_WriteDBHeader tests DB header writing (pure function).
func Benchmark_WriteDBHeader(b *testing.B) {
	buf := make([]byte, DBHeaderSize)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = WriteDBHeader(buf, 100, 0x2000)
	}
}

// Benchmark_ReadDBHeader tests DB header reading (pure function).
func Benchmark_ReadDBHeader(b *testing.B) {
	buf := make([]byte, DBHeaderSize)
	buf[0] = 'd'
	buf[1] = 'b'
	buf[2] = 100 // count
	buf[3] = 0
	buf[4] = 0x00 // blocklist ref
	buf[5] = 0x20
	buf[6] = 0x00
	buf[7] = 0x00

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = ReadDBHeader(buf)
	}
}

// Benchmark_WriteBlocklist tests blocklist writing.
func Benchmark_WriteBlocklist(b *testing.B) {
	refs := make([]uint32, 100)
	for i := range 100 {
		refs[i] = uint32(0x1000 + i*0x100)
	}
	buf := make([]byte, 100*4)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = WriteBlocklist(buf, refs)
	}
}

// Benchmark_ReadBlocklist tests blocklist reading.
func Benchmark_ReadBlocklist(b *testing.B) {
	// Create buffer with 100 refs
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
		_, _ = ReadBlocklist(buf, 100)
	}
}

// Benchmark_WriteBlocklist_Small tests small blocklist.
func Benchmark_WriteBlocklist_Small(b *testing.B) {
	refs := []uint32{0x1000, 0x2000, 0x3000}
	buf := make([]byte, 3*4)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = WriteBlocklist(buf, refs)
	}
}

// Benchmark_ReadBlocklist_Small tests small blocklist.
func Benchmark_ReadBlocklist_Small(b *testing.B) {
	buf := make([]byte, 3*4)
	// Ref 0: 0x1000
	buf[0] = 0x00
	buf[1] = 0x10
	buf[2] = 0x00
	buf[3] = 0x00
	// Ref 1: 0x2000
	buf[4] = 0x00
	buf[5] = 0x20
	buf[6] = 0x00
	buf[7] = 0x00
	// Ref 2: 0x3000
	buf[8] = 0x00
	buf[9] = 0x30
	buf[10] = 0x00
	buf[11] = 0x00

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = ReadBlocklist(buf, 3)
	}
}

// Benchmark_WriteBlocklist_Large tests large blocklist (1000 blocks).
func Benchmark_WriteBlocklist_Large(b *testing.B) {
	refs := make([]uint32, 1000)
	for i := range 1000 {
		refs[i] = uint32(0x1000 + i*0x100)
	}
	buf := make([]byte, 1000*4)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = WriteBlocklist(buf, refs)
	}
}

// Benchmark_ReadBlocklist_Large tests large blocklist (1000 blocks).
func Benchmark_ReadBlocklist_Large(b *testing.B) {
	buf := make([]byte, 1000*4)
	for i := range 1000 {
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
		_, _ = ReadBlocklist(buf, 1000)
	}
}
