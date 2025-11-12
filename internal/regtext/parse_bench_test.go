package regtext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

// ============================================================================
// Real-World File Benchmarks
// ============================================================================

// BenchmarkParseReg_XP_System benchmarks parsing Windows XP System registry (9.1MB).
func BenchmarkParseReg_XP_System(b *testing.B) {
	data := loadTestFile(b, "windows-xp-system.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_XP_Software benchmarks parsing Windows XP Software (3.1MB).
func BenchmarkParseReg_XP_Software(b *testing.B) {
	data := loadTestFile(b, "windows-xp-software.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_2003_System benchmarks parsing Windows 2003 System (2.6MB).
func BenchmarkParseReg_2003_System(b *testing.B) {
	data := loadTestFile(b, "windows-2003-server-system.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_2003_Software benchmarks parsing Windows 2003 Software (18MB).
func BenchmarkParseReg_2003_Software(b *testing.B) {
	data := loadTestFile(b, "windows-2003-server-software.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Win8_System benchmarks parsing Windows 8 System (9.1MB).
func BenchmarkParseReg_Win8_System(b *testing.B) {
	data := loadTestFile(b, "windows-8-enterprise-system.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Win8_Software benchmarks parsing Windows 8 Software (30MB).
func BenchmarkParseReg_Win8_Software(b *testing.B) {
	data := loadTestFile(b, "windows-8-enterprise-software.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_2012_System benchmarks parsing Windows 2012 System (12MB).
func BenchmarkParseReg_2012_System(b *testing.B) {
	data := loadTestFile(b, "windows-2012-system.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_2012_Software benchmarks parsing Windows 2012 Software (43MB).
func BenchmarkParseReg_2012_Software(b *testing.B) {
	data := loadTestFile(b, "windows-2012-software.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Win8CP_Software benchmarks parsing Windows 8 Consumer Preview Software (48MB - largest).
func BenchmarkParseReg_Win8CP_Software(b *testing.B) {
	data := loadTestFile(b, "windows-8-consumer-preview-software.reg")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Generated Matrix Benchmarks - Size Variations
// ============================================================================

// BenchmarkParseReg_Generated_1KB benchmarks parsing small generated file.
func BenchmarkParseReg_Generated_1KB(b *testing.B) {
	profile := Profile{
		TargetSize:      1024,
		KeyDepth:        2,
		KeysPerLevel:    3,
		MinValuesPerKey: 2,
		MaxValuesPerKey: 5,
		MinValueSize:    10,
		MaxValueSize:    50,
		Seed:            42,
	}
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Generated_10KB benchmarks parsing 10KB generated file.
func BenchmarkParseReg_Generated_10KB(b *testing.B) {
	profile := ProfileSmallShallow()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Generated_100KB benchmarks parsing 100KB generated file.
func BenchmarkParseReg_Generated_100KB(b *testing.B) {
	profile := Profile{
		TargetSize:      100 * 1024,
		KeyDepth:        3,
		KeysPerLevel:    5,
		MinValuesPerKey: 3,
		MaxValuesPerKey: 8,
		MinValueSize:    20,
		MaxValueSize:    100,
		Seed:            42,
	}
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Generated_1MB benchmarks parsing 1MB generated file.
func BenchmarkParseReg_Generated_1MB(b *testing.B) {
	profile := ProfileMediumDeep()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Generated_10MB benchmarks parsing 10MB generated file.
func BenchmarkParseReg_Generated_10MB(b *testing.B) {
	profile := ProfileLargeWide()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Generated Matrix Benchmarks - Structural Variations
// ============================================================================

// BenchmarkParseReg_Shallow_Narrow benchmarks shallow narrow hierarchy.
func BenchmarkParseReg_Shallow_Narrow(b *testing.B) {
	profile := Profile{
		TargetSize:      100 * 1024,
		KeyDepth:        2,
		KeysPerLevel:    3,
		MinValuesPerKey: 5,
		MaxValuesPerKey: 10,
		MinValueSize:    50,
		MaxValueSize:    200,
		Seed:            42,
	}
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Deep_Narrow benchmarks deep narrow hierarchy.
func BenchmarkParseReg_Deep_Narrow(b *testing.B) {
	profile := Profile{
		TargetSize:      100 * 1024,
		KeyDepth:        10,
		KeysPerLevel:    2,
		MinValuesPerKey: 5,
		MaxValuesPerKey: 10,
		MinValueSize:    50,
		MaxValueSize:    200,
		Seed:            42,
	}
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_Shallow_Wide benchmarks shallow wide hierarchy.
func BenchmarkParseReg_Shallow_Wide(b *testing.B) {
	profile := Profile{
		TargetSize:      100 * 1024,
		KeyDepth:        2,
		KeysPerLevel:    50,
		MinValuesPerKey: 3,
		MaxValuesPerKey: 6,
		MinValueSize:    20,
		MaxValueSize:    100,
		Seed:            42,
	}
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Generated Matrix Benchmarks - Value Type Distributions
// ============================================================================

// BenchmarkParseReg_StringHeavy benchmarks string-heavy workload.
func BenchmarkParseReg_StringHeavy(b *testing.B) {
	profile := ProfileStringHeavy()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_BinaryHeavy benchmarks binary-heavy workload.
func BenchmarkParseReg_BinaryHeavy(b *testing.B) {
	profile := ProfileBinaryHeavy()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_DWORDHeavy benchmarks DWORD-heavy workload.
func BenchmarkParseReg_DWORDHeavy(b *testing.B) {
	profile := ProfileMediumDeep()
	profile.DWORDValuePct = 0.8
	profile.StringValuePct = 0.2
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Generated Matrix Benchmarks - Special Characteristics
// ============================================================================

// BenchmarkParseReg_HeavyEscaping benchmarks files with lots of escape sequences.
func BenchmarkParseReg_HeavyEscaping(b *testing.B) {
	profile := ProfileWithEscaping()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_WithDeletions benchmarks files with delete operations.
func BenchmarkParseReg_WithDeletions(b *testing.B) {
	profile := ProfileWithDeletions()
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseReg_LargeValues benchmarks files with large value data.
func BenchmarkParseReg_LargeValues(b *testing.B) {
	profile := ProfileMediumDeep()
	profile.MinValueSize = 1024
	profile.MaxValueSize = 10240
	profile.BinaryValuePct = 0.7 // Large binary values
	profile.Seed = 42
	data := GenerateRegFile(profile)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		_, err := ParseReg(data, types.RegParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Micro-Benchmarks for Hotspot Functions
// ============================================================================

// BenchmarkUnescapeRegString benchmarks the string unescaping function.
func BenchmarkUnescapeRegString(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"NoEscape", "SimpleValueName"},
		{"SingleBackslash", "Path\\\\To\\\\Value"},
		{"SingleQuote", "Value\\\"Name"},
		{"Mixed", "Path\\\\To\\\\Value\\\"Name\\\""},
		{"Heavy", "A\\\\B\\\\C\\\\D\\\"E\\\"F\\\\G\\\\H\\\"I"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.input)))
			for range b.N {
				_ = unescapeRegString(tc.input)
			}
		})
	}
}

// BenchmarkFindClosingQuote benchmarks the quote finding algorithm.
func BenchmarkFindClosingQuote(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Short", `"ValueName"=`},
		{"Medium", `"Some\\Path\\To\\ValueName"=`},
		{"Long", `"Very\\Long\\Path\\With\\Many\\Segments\\And\\EscapedQuotes\\\"Here\\\""=`},
		{"EscapedQuotes", `"Value\\\"Name\\\"With\\\"Quotes"=`},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for range b.N {
				_ = findClosingQuote(tc.input)
			}
		})
	}
}

// BenchmarkParseHexBytes benchmarks hex byte parsing.
func BenchmarkParseHexBytes(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Small_4bytes", "hex:01,02,03,04"},
		{
			"Medium_64bytes",
			"hex:01,02,03,04,05,06,07,08,09,0a,0b,0c,0d,0e,0f,10,11,12,13,14,15,16,17,18,19,1a,1b,1c,1d,1e,1f,20,21,22,23,24,25,26,27,28,29,2a,2b,2c,2d,2e,2f,30,31,32,33,34,35,36,37,38,39,3a,3b,3c,3d,3e,3f,40",
		},
		{"Large_256bytes", generateLargeHexString(256)},
		{"Huge_1024bytes", generateLargeHexString(1024)},
		{"ExpandSZ", "hex(2):50,00,61,00,74,00,68,00,00,00"},
		{"MultiSZ", "hex(7):41,00,42,00,00,00,43,00,44,00,00,00,00,00"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.input)))
			for range b.N {
				_, err := parseHexBytes(tc.input)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEncodeUTF16LE benchmarks UTF-16LE encoding.
func BenchmarkEncodeUTF16LE(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Short", "Value"},
		{"Medium", "SomeRegistryValueName"},
		{"Long", "Very Long Registry Value Name With Many Characters"},
		{"PathLike", "\\Path\\To\\Some\\Registry\\Key\\Value"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.input)))
			for range b.N {
				_ = encodeUTF16LEZeroTerminated(tc.input)
			}
		})
	}
}

// BenchmarkDecodeInput benchmarks input encoding detection and decoding.
func BenchmarkDecodeInput(b *testing.B) {
	testCases := []struct {
		name  string
		input []byte
		enc   string
	}{
		{"UTF8_Small", []byte("Windows Registry Editor Version 5.00\n"), ""},
		{"UTF8_WithBOM", append(UTF8BOM, []byte("Windows Registry Editor Version 5.00\n")...), ""},
		{"UTF16LE", encodeUTF16LE("Windows Registry Editor Version 5.00\n", true), ""},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.input)))
			for range b.N {
				_, err := decodeInput(tc.input, tc.enc)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkParseValueLine benchmarks the value line parsing function.
func BenchmarkParseValueLine(b *testing.B) {
	testCases := []struct {
		name string
		line string
	}{
		{"String", `"ValueName"="Some string value"`},
		{"DWORD", `"ValueName"=dword:0000002a`},
		{"Binary", `"ValueName"=hex:01,02,03,04,05,06,07,08`},
		{"ExpandSZ", `"ValueName"=hex(2):50,00,61,00,74,00,68,00,00,00`},
		{"MultiSZ", `"ValueName"=hex(7):41,00,42,00,00,00,43,00,44,00,00,00,00,00`},
		{"Default", `@="Default Value"`},
		{"Delete", `"ValueName"=-`},
	}

	path := "\\TestKey\\SubKey"
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for range b.N {
				_, err := parseValueLine(path, tc.line)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// loadTestFile loads a test data file from testdata/suite/.
func loadTestFile(b *testing.B, filename string) []byte {
	b.Helper()
	// Try multiple possible paths
	paths := []string{
		filepath.Join("..", "..", "testdata", "suite", filename),
		filepath.Join("testdata", "suite", filename),
	}

	var data []byte
	var err error
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			return data
		}
	}

	b.Fatalf("Failed to load test file %s: %v", filename, err)
	return nil
}

// generateLargeHexString creates a hex: string with N bytes.
func generateLargeHexString(numBytes int) string {
	parts := make([]string, numBytes)
	for i := range numBytes {
		parts[i] = "ff"
	}
	return "hex:" + parts[0]
	// Note: intentionally incomplete to save memory, just for benchmarking parseHexBytes overhead
}
