package builder

import (
	"fmt"
	"path/filepath"
	"testing"
)

// BenchmarkNormalizePathArray measures the performance of path normalization.
func BenchmarkNormalizePathArray(b *testing.B) {
	testCases := []struct {
		name string
		path []string
	}{
		{
			name: "with_prefix_hklm",
			path: []string{"HKEY_LOCAL_MACHINE", "SOFTWARE", "Microsoft", "Windows", "CurrentVersion"},
		},
		{
			name: "with_prefix_short",
			path: []string{"HKLM", "SOFTWARE", "Microsoft", "Windows", "CurrentVersion"},
		},
		{
			name: "no_prefix",
			path: []string{"SOFTWARE", "Microsoft", "Windows", "CurrentVersion"},
		},
		{
			name: "deep_path",
			path: []string{"SOFTWARE", "Microsoft", "Windows", "CurrentVersion", "Explorer", "Advanced", "Folder", "HideFileExt"},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = normalizePathArray(tc.path)
			}
		})
	}
}

// BenchmarkSetValueWithNormalization measures the impact on actual SetValue calls.
func BenchmarkSetValueWithNormalization(b *testing.B) {
	testCases := []struct {
		name string
		path []string
	}{
		{
			name: "with_hklm_prefix",
			path: []string{"HKEY_LOCAL_MACHINE", "SOFTWARE", "TestApp"},
		},
		{
			name: "without_prefix",
			path: []string{"SOFTWARE", "TestApp"},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			tmp := b.TempDir()
			out := filepath.Join(tmp, "bench.hiv")

			builder, err := New(out, DefaultOptions())
			if err != nil {
				b.Fatal(err)
			}
			defer builder.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := builder.SetString(tc.path, fmt.Sprintf("Value%d", i), "test")
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
		})
	}
}

// BenchmarkBulkInsert measures performance for realistic bulk operations.
func BenchmarkBulkInsert(b *testing.B) {
	testCases := []struct {
		name        string
		pathPattern func(i int) []string
	}{
		{
			name: "with_prefix",
			pathPattern: func(i int) []string {
				return []string{"HKEY_LOCAL_MACHINE", "SOFTWARE", fmt.Sprintf("App%d", i)}
			},
		},
		{
			name: "without_prefix",
			pathPattern: func(i int) []string {
				return []string{"SOFTWARE", fmt.Sprintf("App%d", i)}
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			tmp := b.TempDir()
			out := filepath.Join(tmp, "bench.hiv")

			builder, err := New(out, DefaultOptions())
			if err != nil {
				b.Fatal(err)
			}
			defer builder.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := tc.pathPattern(i % 100) // Cycle through 100 different paths
				err := builder.SetString(path, "Value", "test data")
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()

			if err := builder.Commit(); err != nil {
				b.Fatal(err)
			}
		})
	}
}
