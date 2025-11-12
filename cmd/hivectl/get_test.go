package main

import (
	"testing"
)

func TestGetCommand(t *testing.T) {
	tests := []struct {
		name           string
		hive           string
		path           string
		valueName      string
		showType       bool
		wantErr        bool
		wantContain    []string
		wantNotContain []string
		wantJSON       bool
	}{
		{
			name:        "get StringValue",
			hive:        "typed_values",
			path:        "TypedValues",
			valueName:   "StringValue",
			showType:    false,
			wantContain: []string{"Test String Value"},
			wantErr:     false,
		},
		{
			name:        "get StringValue with type",
			hive:        "typed_values",
			path:        "TypedValues",
			valueName:   "StringValue",
			showType:    true,
			wantContain: []string{"Test String Value", "REG_SZ"},
			wantErr:     false,
		},
		{
			name:        "get QwordValue",
			hive:        "typed_values",
			path:        "TypedValues",
			valueName:   "QwordValue",
			showType:    false,
			wantContain: []string{},
			wantErr:     false,
		},
		{
			name:        "get value as JSON",
			hive:        "typed_values",
			path:        "TypedValues",
			valueName:   "StringValue",
			showType:    false,
			wantJSON:    true,
			wantContain: []string{"Test String Value"},
			wantErr:     false,
		},
		{
			name:      "nonexistent key",
			hive:      "typed_values",
			path:      "NonexistentKey",
			valueName: "NonexistentValue",
			showType:  false,
			wantErr:   true,
		},
		{
			name:      "nonexistent value",
			hive:      "typed_values",
			path:      "TypedValues",
			valueName: "NonexistentValue",
			showType:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = tt.wantJSON
			getShowType = tt.showType
			getHex = false

			args := []string{testHivePath(t, tt.hive), tt.path, tt.valueName}

			output, err := captureOutput(t, func() error {
				return runGet(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runGet() error = %v, wantErr %v\nOutput: %s", err, tt.wantErr, output)
				return
			}

			if tt.wantJSON && !tt.wantErr {
				assertJSON(t, output)
			}

			assertContains(t, output, tt.wantContain)
			assertNotContains(t, output, tt.wantNotContain)
		})
	}
}
