package acceptance

import "github.com/joshuapare/hivekit/internal/format"

// TestHives provides paths to test hive files used across acceptance tests.
var TestHives = struct {
	Minimal   string
	Special   string
	RLenValue string
	Large     string
}{
	Minimal:   "../../testdata/minimal",
	Special:   "../../testdata/special",
	RLenValue: "../../testdata/rlenvalue_test_hive",
	Large:     "../../testdata/large",
}

// SpecialHiveKnownData contains known data in the "special" test hive
// This hive contains keys and values with special characters to test encoding.
var SpecialHiveKnownData = struct {
	RootName string
	Keys     []string
	Values   map[string]map[string]string // map[keyPath][valueName]valueData
}{
	RootName: "$$$PROTO.HIV",
	Keys: []string{
		"abcd_äöüß",
		"weird™",
		"zero\x00key",
	},
	Values: map[string]map[string]string{
		"weird™": {
			"symbols": "$£₤₧€",
		},
	},
}

// MinimalHiveKnownData contains known data in the "minimal" test hive.
var MinimalHiveKnownData = struct {
	RootName      string
	SubkeyCount   int
	ValueCount    int
	ExpectedFlags uint16
}{
	RootName:      "$$$PROTO.HIV",
	SubkeyCount:   0,
	ValueCount:    0,
	ExpectedFlags: format.NKFlagCompressedName, // Compressed name flag
}

// LargeHiveKnownData contains known data in the "large" test hive.
var LargeHiveKnownData = struct {
	RootName    string
	SubkeyCount int // Approximate - for sanity checks
}{
	RootName:    "$$$PROTO.HIV",
	SubkeyCount: 3, // Known to have "A", "Another", "The"
}

// RLenValueHiveKnownData contains known data in the "rlenvalue_test_hive".
var RLenValueHiveKnownData = struct {
	RootName        string
	TestKeyName     string
	TestValueName   string
	TestValueLength int
}{
	RootName:        "$$$PROTO.HIV",
	TestKeyName:     "ModerateValueParent",
	TestValueName:   "33Bytes",
	TestValueLength: 33,
}
