package main

import (
	"testing"
)

func TestValuesCommand(t *testing.T) {
	tests := []struct {
		name           string
		hive           string
		path           string
		showData       bool
		showType       bool
		wantErr        bool
		wantContain    []string
		wantNotContain []string
		wantJSON       bool
	}{
		{
			name:           "list values in TypedValues key",
			hive:           "typed_values",
			path:           "TypedValues",
			showData:       true,
			showType:       true,
			wantContain:    []string{"StringValue", "QwordValue", "MultiStringValue", "Test String Value"},
			wantNotContain: []string{"Values in"},
			wantErr:        false,
		},
		{
			name:           "list values as JSON",
			hive:           "typed_values",
			path:           "TypedValues",
			showData:       true,
			showType:       true,
			wantJSON:       true,
			wantContain:    []string{"StringValue", "QwordValue", "MultiStringValue"},
			wantNotContain: []string{"Values in"},
			wantErr:        false,
		},
		{
			name:           "list values without type",
			hive:           "typed_values",
			path:           "TypedValues",
			showData:       true,
			showType:       false,
			wantContain:    []string{"StringValue", "Test String Value"},
			wantNotContain: []string{"Values in", "REG_"},
			wantErr:        false,
		},
		{
			name:    "list values from nonexistent key",
			hive:    "typed_values",
			path:    "NonexistentKey",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = tt.wantJSON
			valuesShowData = tt.showData
			valuesShowType = tt.showType
			valuesHex = false

			args := []string{testHivePath(t, tt.hive), tt.path}

			output, err := captureOutput(t, func() error {
				return runValues(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runValues() error = %v, wantErr %v", err, tt.wantErr)
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
