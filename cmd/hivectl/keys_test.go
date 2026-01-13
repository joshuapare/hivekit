package main

import (
	"testing"
)

func TestKeysCommand(t *testing.T) {
	tests := []struct {
		name           string
		hive           string
		path           string
		recursive      bool
		depth          int
		wantErr        bool
		wantContain    []string
		wantNotContain []string // Headers/info that shouldn't be in output
		wantJSON       bool
	}{
		{
			name:           "list root keys typed_values",
			hive:           "typed_values",
			path:           "",
			recursive:      false,
			depth:          1,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"Keys in", "Keys at root:", "Subkeys:", "Values:"},
			wantErr:        false,
		},
		{
			name:           "list root keys special",
			hive:           "special",
			path:           "",
			recursive:      false,
			depth:          1,
			wantContain:    []string{"abcd_äöüß", "weird™"},
			// Note: The "zero" key has an embedded null byte (\x00) - it's actually "zero\x00key"
			// This tests our handling of special characters. We output the full name including null byte.
			wantNotContain: []string{"Keys in", "Keys at root:", "Subkeys:", "Values:", "$$$PROTO.HIV"},
			wantErr:        false,
		},
		{
			name:           "list keys as JSON",
			hive:           "typed_values",
			path:           "",
			recursive:      false,
			depth:          1,
			wantJSON:       true,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"Keys in"},
			wantErr:        false,
		},
		{
			name:           "list keys recursive",
			hive:           "typed_values",
			path:           "",
			recursive:      true,
			depth:          2,
			wantContain:    []string{"TypedValues"},
			wantNotContain: []string{"Keys in", "Keys at root:"},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = tt.wantJSON
			keysRecursive = tt.recursive
			keysDepth = tt.depth

			args := []string{testHivePath(t, tt.hive)}
			if tt.path != "" {
				args = append(args, tt.path)
			}

			output, err := captureOutput(t, func() error {
				return runKeys(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runKeys() error = %v, wantErr %v", err, tt.wantErr)
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
