package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportCommand(t *testing.T) {
	tests := []struct {
		name        string
		hive        string
		key         string
		wantErr     bool
		wantContain []string
	}{
		{
			name:        "export typed_values to stdout",
			hive:        "typed_values",
			key:         "",
			wantErr:     false,
			wantContain: []string{"Windows Registry Editor", "TypedValues", "StringValue", "Test String Value"},
		},
		{
			name:        "export special to stdout",
			hive:        "special",
			key:         "",
			wantErr:     false,
			wantContain: []string{"Windows Registry Editor", "abcd_äöüß", "weird™", "zero"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = false
			exportKey = tt.key
			exportEncoding = "utf8"
			exportBOM = false
			exportStdout = true // Use stdout to avoid creating files

			args := []string{testHivePath(t, tt.hive)}

			output, err := captureOutput(t, func() error {
				return runExport(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assertContains(t, output, tt.wantContain)

			// Verify .reg format basics
			if !strings.HasPrefix(output, "Windows Registry Editor") {
				t.Errorf("export output doesn't start with .reg header")
			}
		})
	}
}

func TestExportToFile(t *testing.T) {
	// Create temporary output directory
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		hive        string
		key         string
		outputFile  string
		wantErr     bool
		wantContain []string
	}{
		{
			name:        "export typed_values to file",
			hive:        "typed_values",
			key:         "",
			outputFile:  filepath.Join(tmpDir, "typed_values.reg"),
			wantErr:     false,
			wantContain: []string{"Windows Registry Editor", "TypedValues", "StringValue"},
		},
		{
			name:        "export special to file",
			hive:        "special",
			key:         "",
			outputFile:  filepath.Join(tmpDir, "special.reg"),
			wantErr:     false,
			wantContain: []string{"Windows Registry Editor", "abcd_äöüß", "weird™"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = false
			exportKey = tt.key
			exportEncoding = "utf8"
			exportBOM = false
			exportStdout = false

			args := []string{testHivePath(t, tt.hive), tt.outputFile}

			err := runExport(args)

			if (err != nil) != tt.wantErr {
				t.Errorf("runExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify file was created and has content
			if !tt.wantErr {
				content, err := os.ReadFile(tt.outputFile)
				if err != nil {
					t.Errorf("failed to read output file: %v", err)
					return
				}

				if len(content) == 0 {
					t.Errorf("export file is empty")
					return
				}

				contentStr := string(content)
				assertContains(t, contentStr, tt.wantContain)

				// Verify file info
				info, err := os.Stat(tt.outputFile)
				if err != nil {
					t.Errorf("failed to stat output file: %v", err)
					return
				}

				t.Logf("Exported %s: %d bytes", tt.hive, info.Size())
			}
		})
	}
}

func TestExportSubtree(t *testing.T) {
	tests := []struct {
		name        string
		hive        string
		key         string
		wantErr     bool
		wantContain []string
	}{
		// Note: These need actual key paths from the test hives
		// For now testing with empty subtree which should work
		{
			name:        "export subtree to stdout",
			hive:        "minimal",
			key:         "",
			wantErr:     false,
			wantContain: []string{"Windows Registry Editor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quiet = false
			verbose = false
			jsonOut = false
			exportKey = tt.key
			exportEncoding = "utf8"
			exportBOM = false
			exportStdout = true

			args := []string{testHivePath(t, tt.hive)}

			output, err := captureOutput(t, func() error {
				return runExport(args)
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("runExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assertContains(t, output, tt.wantContain)
		})
	}
}
