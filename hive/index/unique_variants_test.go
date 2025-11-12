package index

import (
	"strconv"
	"testing"
	"unique"
	"unsafe"
)

// ============================================================================
// Variant 1: Current double-interning approach
// ============================================================================

type UniqueIndexV1 struct {
	nodes  map[unique.Handle[PathKeyV1]]uint32
	values map[unique.Handle[PathKeyV1]]uint32
}

type PathKeyV1 struct {
	ParentOff uint32
	Name      unique.Handle[string]
}

func NewUniqueIndexV1(nkCap, vkCap int) *UniqueIndexV1 {
	return &UniqueIndexV1{
		nodes:  make(map[unique.Handle[PathKeyV1]]uint32, nkCap),
		values: make(map[unique.Handle[PathKeyV1]]uint32, vkCap),
	}
}

func (u *UniqueIndexV1) AddNK(parentOff uint32, name string, offset uint32) {
	key := unique.Make(PathKeyV1{parentOff, unique.Make(name)})
	u.nodes[key] = offset
}

func (u *UniqueIndexV1) AddVK(parentOff uint32, valueName string, offset uint32) {
	key := unique.Make(PathKeyV1{parentOff, unique.Make(valueName)})
	u.values[key] = offset
}

func (u *UniqueIndexV1) GetNK(parentOff uint32, name string) (uint32, bool) {
	key := unique.Make(PathKeyV1{parentOff, unique.Make(name)})
	offset, ok := u.nodes[key]
	return offset, ok
}

func (u *UniqueIndexV1) GetVK(parentOff uint32, valueName string) (uint32, bool) {
	key := unique.Make(PathKeyV1{parentOff, unique.Make(valueName)})
	offset, ok := u.values[key]
	return offset, ok
}

func (u *UniqueIndexV1) Stats() Stats {
	return Stats{
		NKCount:     len(u.nodes),
		VKCount:     len(u.values),
		BytesApprox: (len(u.nodes) + len(u.values)) * 48,
		Impl:        "UniqueV1(double-intern)",
	}
}

// ============================================================================
// Variant 2: Single interning - PathKey directly as map key
// ============================================================================

type UniqueIndexV2 struct {
	nodes  map[PathKeyV2]uint32
	values map[PathKeyV2]uint32
}

type PathKeyV2 struct {
	ParentOff uint32
	Name      unique.Handle[string] // Intern name only, no second intern!
}

func NewUniqueIndexV2(nkCap, vkCap int) *UniqueIndexV2 {
	return &UniqueIndexV2{
		nodes:  make(map[PathKeyV2]uint32, nkCap),
		values: make(map[PathKeyV2]uint32, vkCap),
	}
}

func (u *UniqueIndexV2) AddNK(parentOff uint32, name string, offset uint32) {
	key := PathKeyV2{parentOff, unique.Make(name)}
	u.nodes[key] = offset
}

func (u *UniqueIndexV2) AddVK(parentOff uint32, valueName string, offset uint32) {
	key := PathKeyV2{parentOff, unique.Make(valueName)}
	u.values[key] = offset
}

func (u *UniqueIndexV2) GetNK(parentOff uint32, name string) (uint32, bool) {
	key := PathKeyV2{parentOff, unique.Make(name)}
	offset, ok := u.nodes[key]
	return offset, ok
}

func (u *UniqueIndexV2) GetVK(parentOff uint32, valueName string) (uint32, bool) {
	key := PathKeyV2{parentOff, unique.Make(valueName)}
	offset, ok := u.values[key]
	return offset, ok
}

func (u *UniqueIndexV2) Stats() Stats {
	return Stats{
		NKCount:     len(u.nodes),
		VKCount:     len(u.values),
		BytesApprox: (len(u.nodes) + len(u.values)) * 48,
		Impl:        "UniqueV2(single-intern)",
	}
}

// ============================================================================
// Variant 3: Intern the composite string key
// ============================================================================

type UniqueIndexV3 struct {
	nodes  map[unique.Handle[string]]uint32
	values map[unique.Handle[string]]uint32
}

func NewUniqueIndexV3(nkCap, vkCap int) *UniqueIndexV3 {
	return &UniqueIndexV3{
		nodes:  make(map[unique.Handle[string]]uint32, nkCap),
		values: make(map[unique.Handle[string]]uint32, vkCap),
	}
}

func (u *UniqueIndexV3) AddNK(parentOff uint32, name string, offset uint32) {
	key := makeInternedKey(parentOff, name)
	u.nodes[key] = offset
}

func (u *UniqueIndexV3) AddVK(parentOff uint32, valueName string, offset uint32) {
	key := makeInternedKey(parentOff, valueName)
	u.values[key] = offset
}

func (u *UniqueIndexV3) GetNK(parentOff uint32, name string) (uint32, bool) {
	key := makeInternedKey(parentOff, name)
	offset, ok := u.nodes[key]
	return offset, ok
}

func (u *UniqueIndexV3) GetVK(parentOff uint32, valueName string) (uint32, bool) {
	key := makeInternedKey(parentOff, valueName)
	offset, ok := u.values[key]
	return offset, ok
}

func (u *UniqueIndexV3) Stats() Stats {
	return Stats{
		NKCount:     len(u.nodes),
		VKCount:     len(u.values),
		BytesApprox: (len(u.nodes) + len(u.values)) * 40,
		Impl:        "UniqueV3(intern-composite)",
	}
}

// makeInternedKey interns the entire "parentOff:name" string.
// This is powerful for cross-hive reuse: "32:System" is interned once globally.
func makeInternedKey(parentOff uint32, name string) unique.Handle[string] {
	buf := make([]byte, 0, 11+len(name))
	buf = strconv.AppendUint(buf, uint64(parentOff), 10)
	buf = append(buf, ':')
	buf = append(buf, name...)
	keyStr := unsafe.String(&buf[0], len(buf))
	return unique.Make(keyStr)
}

// ============================================================================
// Benchmarks
// ============================================================================

func benchmarkVariant[T any](
	b *testing.B,
	newFunc func(int, int) T,
	addNK func(T, uint32, string, uint32),
	addVK func(T, uint32, string, uint32),
	getNK func(T, uint32, string) (uint32, bool),
) {
	data := loadHiveData(b, testHives[0])

	b.Run("Build", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			idx := newFunc(len(data.nks), len(data.vks))
			for _, nk := range data.nks {
				addNK(idx, nk[0], data.names[nk[2]], nk[1])
			}
			for _, vk := range data.vks {
				addVK(idx, vk[0], data.names[vk[2]], vk[1])
			}
		}
	})

	b.Run("Lookup_Cold", func(b *testing.B) {
		idx := newFunc(len(data.nks), len(data.vks))
		for _, nk := range data.nks {
			addNK(idx, nk[0], data.names[nk[2]], nk[1])
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := range b.N {
			nk := data.nks[i%len(data.nks)]
			_, ok := getNK(idx, nk[0], data.names[nk[2]])
			if !ok {
				b.Fatal("lookup failed")
			}
		}
	})
}

func Benchmark_Variants(b *testing.B) {
	b.Run("StringIndex", func(b *testing.B) {
		benchmarkVariant(b,
			NewStringIndex,
			func(idx *StringIndex, p uint32, n string, o uint32) { idx.AddNK(p, n, o) },
			func(idx *StringIndex, p uint32, n string, o uint32) { idx.AddVK(p, n, o) },
			func(idx *StringIndex, p uint32, n string) (uint32, bool) { return idx.GetNK(p, n) },
		)
	})

	b.Run("UniqueV1_DoubleIntern", func(b *testing.B) {
		benchmarkVariant(b,
			NewUniqueIndexV1,
			func(idx *UniqueIndexV1, p uint32, n string, o uint32) { idx.AddNK(p, n, o) },
			func(idx *UniqueIndexV1, p uint32, n string, o uint32) { idx.AddVK(p, n, o) },
			func(idx *UniqueIndexV1, p uint32, n string) (uint32, bool) { return idx.GetNK(p, n) },
		)
	})

	b.Run("UniqueV2_SingleIntern", func(b *testing.B) {
		benchmarkVariant(b,
			NewUniqueIndexV2,
			func(idx *UniqueIndexV2, p uint32, n string, o uint32) { idx.AddNK(p, n, o) },
			func(idx *UniqueIndexV2, p uint32, n string, o uint32) { idx.AddVK(p, n, o) },
			func(idx *UniqueIndexV2, p uint32, n string) (uint32, bool) { return idx.GetNK(p, n) },
		)
	})

	b.Run("UniqueV3_InternComposite", func(b *testing.B) {
		benchmarkVariant(b,
			NewUniqueIndexV3,
			func(idx *UniqueIndexV3, p uint32, n string, o uint32) { idx.AddNK(p, n, o) },
			func(idx *UniqueIndexV3, p uint32, n string, o uint32) { idx.AddVK(p, n, o) },
			func(idx *UniqueIndexV3, p uint32, n string) (uint32, bool) { return idx.GetNK(p, n) },
		)
	})
}

// Multi-hive scenario: simulates processing multiple hives sequentially
// This shows the benefit of interning across hives.
func Benchmark_MultiHive_Variants(b *testing.B) {
	data1 := loadHiveData(b, testHives[0])
	data2 := loadHiveData(b, testHives[1])

	runMultiHive := func(b *testing.B, name string, newFunc func(int, int) interface{}, addNK, addVK func(interface{}, uint32, string, uint32)) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				// Process hive 1
				idx1 := newFunc(len(data1.nks), len(data1.vks))
				for _, nk := range data1.nks {
					addNK(idx1, nk[0], data1.names[nk[2]], nk[1])
				}
				for _, vk := range data1.vks {
					addVK(idx1, vk[0], data1.names[vk[2]], vk[1])
				}

				// Process hive 2 (interning should help here!)
				idx2 := newFunc(len(data2.nks), len(data2.vks))
				for _, nk := range data2.nks {
					addNK(idx2, nk[0], data2.names[nk[2]], nk[1])
				}
				for _, vk := range data2.vks {
					addVK(idx2, vk[0], data2.names[vk[2]], vk[1])
				}

				// Prevent optimization
				_ = idx1
				_ = idx2
			}
		})
	}

	runMultiHive(b, "StringIndex",
		func(nk, vk int) interface{} { return NewStringIndex(nk, vk) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*StringIndex).AddNK(p, n, o) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*StringIndex).AddVK(p, n, o) },
	)

	runMultiHive(b, "UniqueV1",
		func(nk, vk int) interface{} { return NewUniqueIndexV1(nk, vk) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV1).AddNK(p, n, o) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV1).AddVK(p, n, o) },
	)

	runMultiHive(b, "UniqueV2",
		func(nk, vk int) interface{} { return NewUniqueIndexV2(nk, vk) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV2).AddNK(p, n, o) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV2).AddVK(p, n, o) },
	)

	runMultiHive(b, "UniqueV3",
		func(nk, vk int) interface{} { return NewUniqueIndexV3(nk, vk) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV3).AddNK(p, n, o) },
		func(idx interface{}, p uint32, n string, o uint32) { idx.(*UniqueIndexV3).AddVK(p, n, o) },
	)
}
