package main

import (
	"testing"
)

func TestTreeCommand(t *testing.T) {
	tests := []struct {
		name           string
		hive           string
		path           string
		depth          int
		showValues     bool
		compact        bool
		wantErr        bool
		wantContain    []string
		wantNotContain []string
		wantJSON       bool
	}{
		{
			name:           "tree typed_values depth 2",
			hive:           "typed_values",
			path:           "",
			depth:          2,
			showValues:     false,
			compact:        false,
			wantErr:        false,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"(root)"},
		},
		{
			name:           "tree with values",
			hive:           "typed_values",
			path:           "",
			depth:          2,
			showValues:     true,
			compact:        false,
			wantErr:        false,
			wantContain:    []string{"TypedValues", "StringValue", "Test String Value"},
			wantNotContain: []string{"(root)"},
		},
		{
			name:           "tree as JSON",
			hive:           "typed_values",
			path:           "",
			depth:          2,
			showValues:     false,
			compact:        false,
			wantJSON:       true,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"(root)"},
			wantErr:        false,
		},
		{
			name:           "tree compact mode",
			hive:           "typed_values",
			path:           "",
			depth:          2,
			showValues:     false,
			compact:        true,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"(root)"},
			wantErr:        false,
		},
		{
			name:           "tree special chars",
			hive:           "special",
			path:           "",
			depth:          2,
			showValues:     false,
			compact:        false,
			wantContain:    []string{"abcd_äöüß", "weird™", "zero"},
			wantNotContain: []string{"(root)"},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = tt.wantJSON
			treeDepth = tt.depth
			treeValues = tt.showValues
			treeCompact = tt.compact
			treeASCII = false

			args := []string{testHivePath(t, tt.hive)}
			if tt.path != "" {
				args = append(args, tt.path)
			}

			output, err := captureOutput(t, func() error {
				return runTree(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runTree() error = %v, wantErr %v", err, tt.wantErr)
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
