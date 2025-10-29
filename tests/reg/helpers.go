package reg_test

import (
	"encoding/binary"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/pkg/hive"
)

func buildSampleHive() ([]byte, map[string]uint32) {
	const (
		headerSize = 4096
		hbinSize   = 4096
		rootOff    = 0x20
		subListOff = 0xB0
		valListOff = 0xC8
		vkStrOff   = 0xD8
		vkDwOff    = 0x110
		dataStrOff = 0x140
		childOff   = 0x1A0
	)

	buf := make([]byte, headerSize+hbinSize)
	copy(buf, []byte{'r', 'e', 'g', 'f'})
	binary.LittleEndian.PutUint32(buf[0x24:], rootOff)
	binary.LittleEndian.PutUint32(buf[0x28:], hbinSize)

	hbin := headerSize
	copy(buf[hbin:], []byte{'h', 'b', 'i', 'n'})
	binary.LittleEndian.PutUint32(buf[hbin+0x04:], uint32(hbin))
	binary.LittleEndian.PutUint32(buf[hbin+0x08:], hbinSize)

	writeCell := func(off int, size int32) int {
		binary.LittleEndian.PutUint32(buf[off:], uint32(-size))
		return off + 4
	}

	rootBody := writeCell(hbin+int(rootOff), 0x90)
	copy(buf[rootBody:], []byte{'n', 'k'})
	binary.LittleEndian.PutUint16(buf[rootBody+0x02:], 0x20) // flags
	binary.LittleEndian.PutUint64(buf[rootBody+0x04:], 0)    // last write (8 bytes)
	binary.LittleEndian.PutUint32(buf[rootBody+0x0C:], 0)    // access bits
	binary.LittleEndian.PutUint32(buf[rootBody+0x10:], 0xFFFFFFFF) // parent offset
	binary.LittleEndian.PutUint32(buf[rootBody+0x14:], 1)    // subkey count (fixed from 0x10)
	binary.LittleEndian.PutUint32(buf[rootBody+0x18:], 0)    // volatile subkey count
	binary.LittleEndian.PutUint32(buf[rootBody+0x1C:], subListOff) // subkey list offset (fixed from 0x18)
	binary.LittleEndian.PutUint32(buf[rootBody+0x20:], 0xFFFFFFFF) // volatile subkey list offset
	binary.LittleEndian.PutUint32(buf[rootBody+0x24:], 2)    // value count (fixed from 0x20)
	binary.LittleEndian.PutUint32(buf[rootBody+0x28:], valListOff) // value list offset (fixed from 0x24)
	binary.LittleEndian.PutUint32(buf[rootBody+0x2C:], 0xFFFFFFFF) // security offset
	binary.LittleEndian.PutUint32(buf[rootBody+0x30:], 0xFFFFFFFF) // class offset
	rootName := []byte("HKEY_LOCAL_MACHINE")
	binary.LittleEndian.PutUint32(buf[rootBody+0x34:], uint32(len(rootName))) // max name length
	binary.LittleEndian.PutUint16(buf[rootBody+0x48:], uint16(len(rootName))) // name length (fixed from 0x4C)
	copy(buf[rootBody+0x4C:], rootName) // name (fixed from 0x50)

	subCell := writeCell(hbin+int(subListOff), 0x18)
	copy(buf[subCell:], []byte{'l', 'i'})
	binary.LittleEndian.PutUint16(buf[subCell+2:], 1)
	binary.LittleEndian.PutUint32(buf[subCell+4:], childOff)

	valCell := writeCell(hbin+int(valListOff), 0x18)
	binary.LittleEndian.PutUint32(buf[valCell:], vkStrOff)
	binary.LittleEndian.PutUint32(buf[valCell+4:], vkDwOff)

	vkStrBody := writeCell(hbin+int(vkStrOff), 0x38)
	copy(buf[vkStrBody:], []byte{'v', 'k'})
	name := []byte("Path")
	binary.LittleEndian.PutUint16(buf[vkStrBody+0x02:], uint16(len(name)))
	dataStr := utf16leBytes("C:\\Temp")
	binary.LittleEndian.PutUint32(buf[vkStrBody+0x04:], uint32(len(dataStr)))
	binary.LittleEndian.PutUint32(buf[vkStrBody+0x08:], dataStrOff)
	binary.LittleEndian.PutUint32(buf[vkStrBody+0x0C:], uint32(hive.REG_SZ))
	binary.LittleEndian.PutUint16(buf[vkStrBody+0x10:], 0x0001)
	copy(buf[vkStrBody+0x14:], name)

	dataStrBody := writeCell(hbin+int(dataStrOff), int32(0x18+len(dataStr)))
	copy(buf[dataStrBody:], dataStr)

	vkDwBody := writeCell(hbin+int(vkDwOff), 0x38)
	copy(buf[vkDwBody:], []byte{'v', 'k'})
	name = []byte("DWORD")
	binary.LittleEndian.PutUint16(buf[vkDwBody+0x02:], uint16(len(name)))
	binary.LittleEndian.PutUint32(buf[vkDwBody+0x04:], 4)
	binary.LittleEndian.PutUint32(buf[vkDwBody+0x08:], dataStrOff+0x40)
	binary.LittleEndian.PutUint32(buf[vkDwBody+0x0C:], uint32(hive.REG_DWORD))
	binary.LittleEndian.PutUint16(buf[vkDwBody+0x10:], 0x0001)
	copy(buf[vkDwBody+0x14:], name)
	dwCell := writeCell(hbin+int(dataStrOff+0x40), 0x18)
	binary.LittleEndian.PutUint32(buf[dwCell:], 0x2a)

	childBody := writeCell(hbin+int(childOff), 0x60)
	copy(buf[childBody:], []byte{'n', 'k'})
	binary.LittleEndian.PutUint16(buf[childBody+0x02:], 0x20) // flags
	binary.LittleEndian.PutUint64(buf[childBody+0x04:], 0)    // last write (8 bytes)
	binary.LittleEndian.PutUint32(buf[childBody+0x0C:], 0)    // access bits
	binary.LittleEndian.PutUint32(buf[childBody+0x10:], rootOff) // parent offset
	binary.LittleEndian.PutUint32(buf[childBody+0x14:], 0)    // subkey count
	binary.LittleEndian.PutUint32(buf[childBody+0x18:], 0)    // volatile subkey count
	binary.LittleEndian.PutUint32(buf[childBody+0x1C:], 0xFFFFFFFF) // subkey list offset
	binary.LittleEndian.PutUint32(buf[childBody+0x20:], 0xFFFFFFFF) // volatile subkey list offset
	binary.LittleEndian.PutUint32(buf[childBody+0x24:], 0)    // value count
	binary.LittleEndian.PutUint32(buf[childBody+0x28:], 0xFFFFFFFF) // value list offset
	childName := []byte("SOFTWARE")
	binary.LittleEndian.PutUint16(buf[childBody+0x48:], uint16(len(childName))) // name length (fixed from 0x4C)
	copy(buf[childBody+0x4C:], childName) // name (fixed from 0x50)

	return buf, map[string]uint32{
		"root":  rootOff,
		"child": childOff,
	}
}

func utf16leBytes(s string) []byte {
	var words []uint16
	for _, r := range s {
		words = append(words, utf16.Encode([]rune{r})...)
	}
	words = append(words, 0)
	buf := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[i*2:], w)
	}
	return buf
}
