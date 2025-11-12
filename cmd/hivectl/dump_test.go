package main

import (
	"testing"
)

func TestDumpCommand(t *testing.T) {
	tests := []struct {
		name           string
		hive           string
		key            string
		depth          int
		compact        bool
		wantErr        bool
		wantContain    []string
		wantNotContain []string
		wantJSON       bool
	}{
		{
			name:           "dump typed_values",
			hive:           "typed_values",
			key:            "",
			depth:          0,
			compact:        false,
			wantErr:        false,
			wantContain:    []string{"TypedValues", "StringValue", "Test String Value"},
			wantNotContain: []string{"Registry Hive Dump", "═"},
		},
		{
			name:           "dump compact",
			hive:           "typed_values",
			key:            "",
			depth:          0,
			compact:        true,
			wantErr:        false,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"Registry Hive Dump", "═"},
		},
		{
			name:           "dump as JSON",
			hive:           "typed_values",
			key:            "",
			depth:          0,
			compact:        false,
			wantJSON:       true,
			wantContain:    []string{"TypedValues", "StringValue"},
			wantNotContain: []string{"Registry Hive Dump", "═"},
			wantErr:        false,
		},
		{
			name:           "dump with depth limit",
			hive:           "typed_values",
			key:            "",
			depth:          1,
			compact:        false,
			wantErr:        false,
			wantContain:    []string{"TypedValues", "Subkeys:", "Values:", "StringValue"},
			wantNotContain: []string{"Registry Hive Dump", "═"},
		},
		{
			name:           "dump special chars",
			hive:           "special",
			key:            "",
			depth:          2,
			compact:        false,
			wantErr:        false,
			wantContain:    []string{"abcd_äöüß", "weird™", "zero"},
			wantNotContain: []string{"Registry Hive Dump", "═", "$$$PROTO.HIV"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = tt.wantJSON
			dumpKey = tt.key
			dumpDepth = tt.depth
			dumpValuesOnly = false
			dumpCompact = tt.compact

			args := []string{testHivePath(t, tt.hive)}

			output, err := captureOutput(t, func() error {
				return runDump(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runDump() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantJSON {
				assertJSON(t, output)
			}

			assertContains(t, output, tt.wantContain)
			assertNotContains(t, output, tt.wantNotContain)
		})
	}
}
