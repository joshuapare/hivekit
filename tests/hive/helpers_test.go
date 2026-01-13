package hive_test

import (
	"unicode/utf16"

	"github.com/joshuapare/hivekit/internal/format"
)

const (
	regfHeaderSize = 4096
	hbinSize       = 4096
)

// synthHive builds a small synthetic hive with the following structure:
//
//	ROOT (NK)
//	  Values: Test (DWORD), Path (SZ), Multi (MULTI_SZ)
//	  Subkey: CHILD (NK without values)
//
// It returns the byte buffer and offsets used for assertions.
func synthHive() ([]byte, map[string]uint32) {
	const (
		rootCellOff  = 0x20
		subListOff   = 0xB0
		valListOff   = 0xC8
		vkDWOff      = 0xD8
		vkStrOff     = 0x110
		vkMultiOff   = 0x148
		dataDWOff    = 0x180
		dataStrOff   = 0x198
		dataMultiOff = 0x1D8
		childOff     = 0x240
	)

	buf := make([]byte, regfHeaderSize+hbinSize)
	copy(buf, []byte{'r', 'e', 'g', 'f'})
	format.PutU32(buf, 0x04, 1) // sequences
	format.PutU32(buf, 0x08, 1)
	format.PutU32(buf, 0x24, rootCellOff)
	format.PutU32(buf, 0x28, hbinSize)

	// HBIN header
	hbin := regfHeaderSize
	copy(buf[hbin:], []byte{'h', 'b', 'i', 'n'})
	format.PutU32(buf, hbin+0x04, uint32(hbin))
	format.PutU32(buf, hbin+0x08, hbinSize)

	writeCell := func(off int, size int32) int {
		format.PutU32(buf, off, uint32(-size))
		return off + 4
	}

	// Root NK (with corrected offsets)
	rootBody := writeCell(hbin+0x20, 0x90)
	copy(buf[rootBody:], []byte{'n', 'k'})
	format.PutU16(buf, rootBody+0x02, 0x20) // compressed ASCII name
	// 0x0C: access bits (skip, write 0)
	format.PutU32(buf, rootBody+0x0C, 0)
	// 0x10: parent offset (skip, write -1)
	format.PutU32(buf, rootBody+0x10, 0xFFFFFFFF)
	// 0x14: subkey count (fixed from 0x10)
	format.PutU32(buf, rootBody+0x14, 1)
	// 0x18: volatile subkey count (skip, write 0)
	format.PutU32(buf, rootBody+0x18, 0)
	// 0x1C: subkey list offset (fixed from 0x18)
	format.PutU32(buf, rootBody+0x1C, subListOff)
	// 0x20: volatile subkey list offset (skip, write -1)
	format.PutU32(buf, rootBody+0x20, 0xFFFFFFFF)
	// 0x24: value count (fixed from 0x20)
	format.PutU32(buf, rootBody+0x24, 3)
	// 0x28: value list offset (fixed from 0x24)
	format.PutU32(buf, rootBody+0x28, valListOff)
	rootName := []byte("HKEY_LOCAL_MACHINE")
	format.PutU32(buf, rootBody+0x34, uint32(len(rootName))) // max name len at 0x34
	// 0x48: name length (fixed from 0x4C)
	format.PutU16(buf, rootBody+0x48, uint16(len(rootName)))
	// 0x4C: name (fixed from 0x50)
	copy(buf[rootBody+0x4C:], rootName)

	// Subkey list (LI with one entry)
	liBody := writeCell(hbin+int(subListOff), 0x18)
	copy(buf[liBody:], []byte{'l', 'i'})
	format.PutU16(buf, liBody+2, 1)
	format.PutU32(buf, liBody+4, childOff)

	// Value list (three entries)
	valBody := writeCell(hbin+int(valListOff), 0x18)
	format.PutU32(buf, valBody, vkDWOff)
	format.PutU32(buf, valBody+4, vkStrOff)
	format.PutU32(buf, valBody+8, vkMultiOff)

	// DWORD value
	vkDWBody := writeCell(hbin+int(vkDWOff), 0x38)
	copy(buf[vkDWBody:], []byte{'v', 'k'})
	name := []byte("Test")
	format.PutU16(buf, vkDWBody+0x02, uint16(len(name)))
	format.PutU32(buf, vkDWBody+0x04, 4)
	format.PutU32(buf, vkDWBody+0x08, dataDWOff)
	format.PutU32(buf, vkDWBody+0x0C, 4)
	format.PutU16(buf, vkDWBody+0x10, 0x0001) // ASCII name
	copy(buf[vkDWBody+0x14:], name)
	dataDWBody := writeCell(hbin+int(dataDWOff), 0x18)
	copy(buf[dataDWBody:], []byte{0x78, 0x56, 0x34, 0x12})

	// String value
	vkStrBody := writeCell(hbin+int(vkStrOff), 0x38)
	copy(buf[vkStrBody:], []byte{'v', 'k'})
	name = []byte("Path")
	format.PutU16(buf, vkStrBody+0x02, uint16(len(name)))
	strData := utf16le("C:\\Temp", true)
	format.PutU32(buf, vkStrBody+0x04, uint32(len(strData)))
	format.PutU32(buf, vkStrBody+0x08, dataStrOff)
	format.PutU32(buf, vkStrBody+0x0C, 1)
	format.PutU16(buf, vkStrBody+0x10, 0x0001)
	copy(buf[vkStrBody+0x14:], name)

	dataStrBody := writeCell(hbin+int(dataStrOff), 0x40)
	copy(buf[dataStrBody:], strData)

	// Multi-string value
	vkMultiBody := writeCell(hbin+int(vkMultiOff), 0x38)
	copy(buf[vkMultiBody:], []byte{'v', 'k'})
	name = []byte("Multi")
	format.PutU16(buf, vkMultiBody+0x02, uint16(len(name)))
	multiData := multiUTF16([]string{"One", "Two"})
	format.PutU32(buf, vkMultiBody+0x04, uint32(len(multiData)))
	format.PutU32(buf, vkMultiBody+0x08, dataMultiOff)
	format.PutU32(buf, vkMultiBody+0x0C, 7)
	format.PutU16(buf, vkMultiBody+0x10, 0x0001)
	copy(buf[vkMultiBody+0x14:], name)

	dataMultiBody := writeCell(hbin+int(dataMultiOff), 0x60)
	copy(buf[dataMultiBody:], multiData)

	// Child NK (with corrected offsets)
	childBody := writeCell(hbin+int(childOff), 0x60)
	copy(buf[childBody:], []byte{'n', 'k'})
	format.PutU16(buf, childBody+0x02, 0x20)
	// Fill in required fields with proper offsets
	format.PutU32(buf, childBody+0x0C, 0)           // access bits
	format.PutU32(buf, childBody+0x10, rootCellOff) // parent
	format.PutU32(buf, childBody+0x14, 0)           // subkey count
	format.PutU32(buf, childBody+0x18, 0)           // volatile subkey count
	format.PutU32(buf, childBody+0x1C, 0xFFFFFFFF)  // subkey list offset
	format.PutU32(buf, childBody+0x20, 0xFFFFFFFF)  // volatile subkey list offset
	format.PutU32(buf, childBody+0x24, 0)           // value count
	format.PutU32(buf, childBody+0x28, 0xFFFFFFFF)  // value list offset
	childName := []byte("SOFTWARE")
	// 0x48: name length (fixed from 0x4C)
	format.PutU16(buf, childBody+0x48, uint16(len(childName)))
	// 0x4C: name (fixed from 0x50)
	copy(buf[childBody+0x4C:], childName)

	offsets := map[string]uint32{
		"root":  rootCellOff,
		"child": childOff,
	}
	return buf, offsets
}

func utf16le(s string, withNull bool) []byte {
	u16 := utf16Enc(s)
	if withNull {
		u16 = append(u16, 0)
	}
	out := make([]byte, len(u16)*2)
	for i, v := range u16 {
		format.PutU16(out, i*2, v)
	}
	return out
}

func multiUTF16(parts []string) []byte {
	// Calculate approximate capacity: each string + null terminator + final null
	capacity := 0
	for _, p := range parts {
		capacity += len(p) + 1 // approximate: each rune ~1 uint16 + null terminator
	}
	capacity += 1 // final null terminator
	seq := make([]uint16, 0, capacity)
	for _, p := range parts {
		seq = append(seq, utf16Enc(p)...)
		seq = append(seq, 0)
	}
	seq = append(seq, 0)
	out := make([]byte, len(seq)*2)
	for i, v := range seq {
		format.PutU16(out, i*2, v)
	}
	return out
}

func utf16Enc(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

// BuildMinimalHive creates a minimal valid hive for testing.
// Exported for use in other test packages.
func BuildMinimalHive() []byte {
	hive, _ := synthHive()
	return hive
}
