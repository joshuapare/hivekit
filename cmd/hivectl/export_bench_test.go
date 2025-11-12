package main

import (
	"io"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
)

// Benchmark exporting large real-world hives
func BenchmarkExport_Windows2003Software(b *testing.B) {
	benchmarkExport(b, "../../testdata/suite/windows-2003-server-software")
}

func BenchmarkExport_Windows2012Software(b *testing.B) {
	benchmarkExport(b, "../../testdata/suite/windows-2012-software")
}

func BenchmarkExport_Windows8Software(b *testing.B) {
	benchmarkExport(b, "../../testdata/suite/windows-8-consumer-preview-software")
}

func benchmarkExport(b *testing.B, hivePath string) {
	// Open hive once
	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatalf("failed to open hive: %v", err)
	}
	defer h.Close()

	// Configure printer options
	opts := printer.DefaultOptions()
	opts.Format = printer.FormatReg
	opts.ShowValues = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := h.PrintTree(io.Discard, "", opts); err != nil {
			b.Fatalf("print tree failed: %v", err)
		}
	}
}
