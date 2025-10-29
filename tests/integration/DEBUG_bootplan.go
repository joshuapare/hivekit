// +build ignore

package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func main() {
	r, err := reader.OpenBytes(mustRead("testdata/suite/windows-xp-system"), hive.OpenOptions{})
	if err != nil {
		panic(err)
	}
	defer r.Close()

	// Navigate to the BootPlan value
	root, _ := r.Root()
	cs001, _ := r.Lookup(root, "controlset001")
	services, _ := r.Lookup(cs001, "services")
	rdyboost, _ := r.Lookup(services, "rdyboost")
	params, _ := r.Lookup(rdyboost, "parameters")

	values, _ := r.Values(params)
	for _, valID := range values {
		meta, _ := r.StatValue(valID)
		if meta.Name == "BootPlan" {
			fmt.Printf("Found BootPlan value:\n")
			fmt.Printf("  ValueID: %d (0x%x)\n", valID, valID)
			fmt.Printf("  Type: %d\n", meta.ValueType)
			fmt.Printf("  DataLength: %d bytes\n", meta.DataLength)

			// Now try to read it and see what happens
			_, err := r.ValueBytes(valID, hive.ReadOptions{})
			if err != nil {
				fmt.Printf("\nERROR: %v\n", err)

				// Let's manually examine the VK record
				buf := r.BaseBuffer()
				offset := uint32(valID)
				abs := int(format.HeaderSize) + int(offset)

				// Read cell header (4 bytes)
				cellSize := int32(buf[abs]) | int32(buf[abs+1])<<8 | int32(buf[abs+2])<<16 | int32(buf[abs+3])<<24
				cellSize = -cellSize // negative = allocated
				fmt.Printf("\nVK Cell info:\n")
				fmt.Printf("  Cell offset: 0x%x\n", abs)
				fmt.Printf("  Cell size: %d bytes\n", cellSize)

				// Read VK record
				vk, err := format.DecodeVK(buf[abs+4 : abs+4+int(cellSize)])
				if err != nil {
					fmt.Printf("  DecodeVK error: %v\n", err)
					return
				}

				fmt.Printf("  VK.DataLength: 0x%08x (%d bytes)\n", vk.DataLength, vk.DataLength&0x7fffffff)
				fmt.Printf("  VK.DataOffset: 0x%08x\n", vk.DataOffset)
				fmt.Printf("  VK.Type: %d\n", vk.Type)
				fmt.Printf("  VK.Flags: 0x%04x\n", vk.Flags)
				fmt.Printf("  VK.DataInline: %v\n", vk.DataInline())

				if !vk.DataInline() {
					// Check the data cell
					dataAbs := int(format.HeaderSize) + int(vk.DataOffset)
					dataCellSize := int32(buf[dataAbs]) | int32(buf[dataAbs+1])<<8 | int32(buf[dataAbs+2])<<16 | int32(buf[dataAbs+3])<<24
					dataCellSize = -dataCellSize

					fmt.Printf("\nData cell info:\n")
					fmt.Printf("  Data cell offset: 0x%x\n", dataAbs)
					fmt.Printf("  Data cell size: %d bytes\n", dataCellSize)
					fmt.Printf("  Data payload size: %d bytes (cell size - 4)\n", dataCellSize-4)

					expectedLen := int(vk.DataLength & 0x7fffffff)
					actualLen := int(dataCellSize - 4)

					fmt.Printf("\nMismatch:\n")
					fmt.Printf("  Expected data length: %d bytes\n", expectedLen)
					fmt.Printf("  Actual cell payload:  %d bytes\n", actualLen)
					fmt.Printf("  Difference: %d bytes\n", expectedLen-actualLen)

					// Check if the cell signature indicates a "db" record
					sig := buf[dataAbs+4 : dataAbs+6]
					fmt.Printf("\nData cell signature: %q (0x%02x 0x%02x)\n", sig, sig[0], sig[1])

					if string(sig) == "db" {
						fmt.Printf("  *** This is a 'db' record (multi-cell large data) ***\n")
					}
				}
			} else {
				fmt.Printf("SUCCESS: Read value\n")
			}
		}
	}
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}
