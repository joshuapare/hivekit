package regtext

import (
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefix   string
		expected bool
	}{
		{
			name:     "exact match",
			s:        "HKEY_LOCAL_MACHINE",
			prefix:   "HKEY_LOCAL_MACHINE",
			expected: true,
		},
		{
			name:     "prefix match",
			s:        "HKEY_LOCAL_MACHINE\\SOFTWARE",
			prefix:   "HKEY_LOCAL_MACHINE",
			expected: true,
		},
		{
			name:     "case insensitive match",
			s:        "hkey_local_machine\\software",
			prefix:   "HKEY_LOCAL_MACHINE",
			expected: true,
		},
		{
			name:     "no match",
			s:        "HKEY_CURRENT_USER",
			prefix:   "HKEY_LOCAL_MACHINE",
			expected: false,
		},
		{
			name:     "prefix longer than string",
			s:        "HKEY",
			prefix:   "HKEY_LOCAL_MACHINE",
			expected: false,
		},
		{
			name:     "empty string",
			s:        "",
			prefix:   "HKEY",
			expected: false,
		},
		{
			name:     "empty prefix",
			s:        "HKEY_LOCAL_MACHINE",
			prefix:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPrefix(tt.s, tt.prefix)
			if result != tt.expected {
				t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestExpandRootKeyAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HKLM to HKEY_LOCAL_MACHINE",
			input:    "HKLM\\SOFTWARE\\Microsoft",
			expected: "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft",
		},
		{
			name:     "HKCU to HKEY_CURRENT_USER",
			input:    "HKCU\\Software\\Test",
			expected: "HKEY_CURRENT_USER\\Software\\Test",
		},
		{
			name:     "HKCR to HKEY_CLASSES_ROOT",
			input:    "HKCR\\.txt",
			expected: "HKEY_CLASSES_ROOT\\.txt",
		},
		{
			name:     "HKU to HKEY_USERS",
			input:    "HKU\\S-1-5-21",
			expected: "HKEY_USERS\\S-1-5-21",
		},
		{
			name:     "HKCC to HKEY_CURRENT_CONFIG",
			input:    "HKCC\\System",
			expected: "HKEY_CURRENT_CONFIG\\System",
		},
		{
			name:     "already expanded - no change",
			input:    "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected: "HKEY_LOCAL_MACHINE\\SOFTWARE",
		},
		{
			name:     "no alias - no change",
			input:    "SOFTWARE\\Microsoft",
			expected: "SOFTWARE\\Microsoft",
		},
		{
			name:     "case insensitive alias",
			input:    "hklm\\SOFTWARE",
			expected: "HKEY_LOCAL_MACHINE\\SOFTWARE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandRootKeyAlias(tt.input)
			if result != tt.expected {
				t.Errorf("expandRootKeyAlias(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTryStripPrefix(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		prefix      string
		expected    string
		expectError bool
	}{
		{
			name:        "strip HKLM\\SOFTWARE",
			path:        "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "Microsoft\\Windows",
			expectError: false,
		},
		{
			name:        "strip with trailing backslash",
			path:        "HKEY_LOCAL_MACHINE\\SOFTWARE\\Test",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "Test",
			expectError: false,
		},
		{
			name:        "case insensitive stripping",
			path:        "hkey_local_machine\\software\\Test",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "Test",
			expectError: false,
		},
		{
			name:        "path equals prefix",
			path:        "HKEY_LOCAL_MACHINE\\SOFTWARE",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "",
			expectError: false,
		},
		{
			name:        "prefix mismatch - error",
			path:        "HKEY_CURRENT_USER\\Software",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "",
			expectError: true,
		},
		{
			name:        "partial match not enough - error",
			path:        "HKEY_LOCAL_MACHINE\\SYSTEM",
			prefix:      "HKEY_LOCAL_MACHINE\\SOFTWARE",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tryStripPrefix(tt.path, tt.prefix)
			if tt.expectError {
				if err == nil {
					t.Errorf("tryStripPrefix(%q, %q) expected error, got nil", tt.path, tt.prefix)
				}
			} else {
				if err != nil {
					t.Errorf("tryStripPrefix(%q, %q) unexpected error: %v", tt.path, tt.prefix, err)
				}
				if result != tt.expected {
					t.Errorf("tryStripPrefix(%q, %q) = %q, want %q", tt.path, tt.prefix, result, tt.expected)
				}
			}
		})
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		opts        types.RegParseOptions
		expected    string
		expectError bool
	}{
		{
			name:        "default - strip root key only",
			path:        "HKEY_LOCAL_MACHINE\\SOFTWARE\\Test",
			opts:        types.RegParseOptions{},
			expected:    "SOFTWARE\\Test",
			expectError: false,
		},
		{
			name: "manual prefix stripping",
			path: "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows",
			opts: types.RegParseOptions{
				Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
			},
			expected:    "Microsoft\\Windows",
			expectError: false,
		},
		{
			name: "manual prefix with alias expansion",
			path: "HKLM\\SOFTWARE\\Microsoft\\Windows",
			opts: types.RegParseOptions{
				Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
			},
			expected:    "Microsoft\\Windows",
			expectError: false,
		},
		{
			name: "manual prefix mismatch - error",
			path: "HKEY_LOCAL_MACHINE\\SYSTEM\\Test",
			opts: types.RegParseOptions{
				Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
			},
			expected:    "",
			expectError: true,
		},
		{
			name: "auto prefix - SOFTWARE",
			path: "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Microsoft",
			expectError: false,
		},
		{
			name: "auto prefix - SYSTEM",
			path: "HKEY_LOCAL_MACHINE\\SYSTEM\\CurrentControlSet",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "CurrentControlSet",
			expectError: false,
		},
		{
			name: "auto prefix - SAM",
			path: "HKEY_LOCAL_MACHINE\\SAM\\SAM",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "SAM",
			expectError: false,
		},
		{
			name: "auto prefix - SECURITY",
			path: "HKEY_LOCAL_MACHINE\\SECURITY\\Policy",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Policy",
			expectError: false,
		},
		{
			name: "auto prefix - HARDWARE",
			path: "HKEY_LOCAL_MACHINE\\HARDWARE\\DESCRIPTION",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "DESCRIPTION",
			expectError: false,
		},
		{
			name: "auto prefix - HKEY_CURRENT_USER",
			path: "HKEY_CURRENT_USER\\Software\\Test",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Software\\Test",
			expectError: false,
		},
		{
			name: "auto prefix - HKEY_USERS",
			path: "HKEY_USERS\\S-1-5-21-123456\\Software",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "S-1-5-21-123456\\Software",
			expectError: false,
		},
		{
			name: "auto prefix - no match, return as-is",
			path: "Software\\Microsoft",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Software\\Microsoft",
			expectError: false,
		},
		{
			name: "auto prefix with alias expansion",
			path: "HKLM\\SOFTWARE\\Test",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Test",
			expectError: false,
		},
		{
			name: "auto prefix - case insensitive",
			path: "hkey_local_machine\\software\\Test",
			opts: types.RegParseOptions{
				AutoPrefix: true,
			},
			expected:    "Test",
			expectError: false,
		},
		{
			name:        "empty path",
			path:        "",
			opts:        types.RegParseOptions{AutoPrefix: true},
			expected:    "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := stripPrefix(tt.path, tt.opts)
			if tt.expectError {
				if err == nil {
					t.Errorf("stripPrefix(%q, %+v) expected error, got nil", tt.path, tt.opts)
				}
			} else {
				if err != nil {
					t.Errorf("stripPrefix(%q, %+v) unexpected error: %v", tt.path, tt.opts, err)
				}
				if result != tt.expected {
					t.Errorf("stripPrefix(%q, %+v) = %q, want %q", tt.path, tt.opts, result, tt.expected)
				}
			}
		})
	}
}
