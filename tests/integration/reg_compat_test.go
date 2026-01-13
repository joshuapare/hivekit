package integration

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Suite hives with corresponding .reg files.
var suiteHives = []struct {
	name        string
	hivePath    string
	regPath     string
	description string
}{
	{
		"windows-xp-system",
		"../../testdata/suite/windows-xp-system",
		"../../testdata/suite/windows-xp-system.reg",
		"Windows XP SYSTEM hive",
	},
	{
		"windows-xp-software",
		"../../testdata/suite/windows-xp-software",
		"../../testdata/suite/windows-xp-software.reg",
		"Windows XP SOFTWARE hive",
	},
	{
		"windows-2003-server-system",
		"../../testdata/suite/windows-2003-server-system",
		"../../testdata/suite/windows-2003-server-system.reg",
		"Windows Server 2003 SYSTEM",
	},
	{
		"windows-2003-server-software",
		"../../testdata/suite/windows-2003-server-software",
		"../../testdata/suite/windows-2003-server-software.reg",
		"Windows Server 2003 SOFTWARE",
	},
	{
		"windows-8-enterprise-system",
		"../../testdata/suite/windows-8-enterprise-system",
		"../../testdata/suite/windows-8-enterprise-system.reg",
		"Windows 8 Enterprise SYSTEM",
	},
	{
		"windows-8-enterprise-software",
		"../../testdata/suite/windows-8-enterprise-software",
		"../../testdata/suite/windows-8-enterprise-software.reg",
		"Windows 8 Enterprise SOFTWARE",
	},
	{
		"windows-2012-system",
		"../../testdata/suite/windows-2012-system",
		"../../testdata/suite/windows-2012-system.reg",
		"Windows Server 2012 SYSTEM",
	},
	{
		"windows-2012-software",
		"../../testdata/suite/windows-2012-software",
		"../../testdata/suite/windows-2012-software.reg",
		"Windows Server 2012 SOFTWARE",
	},
}

// TestRegFileCompatibility tests gohivex against .reg file golden reference data.
func TestRegFileCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			// Check if hive file exists
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				t.Skipf("Hive file not found: %s", tc.hivePath)
			}

			// Check if .reg file exists
			if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
				t.Skipf(".reg file not found: %s", tc.regPath)
			}

			// Parse .reg file to get expected counts
			regFile, err := os.Open(tc.regPath)
			if err != nil {
				t.Fatalf("Failed to open .reg file: %v", err)
			}
			defer regFile.Close()

			regStats, err := regtext.ParseRegFile(regFile)
			if err != nil {
				t.Fatalf("Failed to parse .reg file: %v", err)
			}

			t.Logf(".reg file: %d keys, %d values", regStats.KeyCount, regStats.ValueCount)

			// Open hive with gohivex
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Fatalf("Failed to read hive: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			// Count nodes and values using hivexval
			v := hivexval.Must(hivexval.New(tc.hivePath, nil))
			defer v.Close()

			gohivexNodeCount, gohivexValueCount, err := v.CountTree()
			if err != nil {
				t.Fatalf("Failed to count tree: %v", err)
			}

			t.Logf("gohivex:  %d keys, %d values", gohivexNodeCount, gohivexValueCount)

			// Compare counts
			if gohivexNodeCount != regStats.KeyCount {
				t.Errorf("Key count mismatch: gohivex=%d, .reg=%d (diff: %d)",
					gohivexNodeCount, regStats.KeyCount, gohivexNodeCount-regStats.KeyCount)
			}

			if gohivexValueCount != regStats.ValueCount {
				t.Errorf("Value count mismatch: gohivex=%d, .reg=%d (diff: %d)",
					gohivexValueCount, regStats.ValueCount, gohivexValueCount-regStats.ValueCount)
			}

			// If counts match, structure is correct
			if gohivexNodeCount == regStats.KeyCount && gohivexValueCount == regStats.ValueCount {
				t.Logf("Matches .reg reference data perfectly!")
			}
		})
	}
}

// TestRegFileCompatibilitySummary provides a summary of all hive compatibility.
func TestRegFileCompatibilitySummary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	perfectMatches := 0
	totalHives := 0

	for _, tc := range suiteHives {
		// Check if both files exist
		if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
			continue
		}

		totalHives++

		// Parse .reg file
		regFile, err := os.Open(tc.regPath)
		if err != nil {
			continue
		}
		regStats, err := regtext.ParseRegFile(regFile)
		regFile.Close()
		if err != nil {
			continue
		}

		// Open hive with hivexval to count nodes/values
		v, err := hivexval.New(tc.hivePath, nil)
		if err != nil {
			continue
		}
		gohivexNodes, gohivexValues, err := v.CountTree()
		v.Close()
		if err != nil {
			continue
		}

		// Check if perfect match
		if gohivexNodes == regStats.KeyCount && gohivexValues == regStats.ValueCount {
			perfectMatches++
		}
	}

	t.Logf("")
	t.Logf("=== .reg File Compatibility Summary ===")
	t.Logf(
		"Perfect matches: %d/%d (%.1f%%)",
		perfectMatches,
		totalHives,
		float64(perfectMatches)/float64(totalHives)*100,
	)
	t.Logf("")

	if perfectMatches == totalHives {
		t.Logf("ALL hives match .reg reference data perfectly!")
	}
}
