package subkeys

import (
	"testing"
	"unicode"
	"unicode/utf8"
)

// hashReference is the original Unicode-based implementation, kept for
// correctness comparison against the optimized Hash().
func hashReference(name string) uint32 {
	var hash uint32
	for _, r := range name {
		hash = hash*hashMultiplier + uint32(unicode.ToUpper(r))
	}
	return hash
}

// TestHash_KnownValues verifies that Hash produces specific expected values.
// These values were computed with the original unicode.ToUpper-based
// implementation and must remain stable across optimizations.
func TestHash_KnownValues(t *testing.T) {
	tests := []struct {
		name string
		want uint32
	}{
		{"", 0},
		{"a", 65},
		{"software", 3925742691},
		{"CurrentVersion", 2116417181},
		{"HKEY_LOCAL_MACHINE", 3340597407},
		{"test", 4352468},
		{"Software\\Microsoft\\Windows\\CurrentVersion", 2016943709},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hash(tt.name)
			if got != tt.want {
				t.Errorf("Hash(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// FuzzHash verifies that the optimized Hash function produces bit-identical
// output to the reference implementation for arbitrary valid UTF-8 strings.
func FuzzHash(f *testing.F) {
	f.Add("software")
	f.Add("CurrentVersion")
	f.Add("HKEY_LOCAL_MACHINE")
	f.Add("Ünïcödé")
	f.Add("")
	f.Add("a")
	f.Add("test")
	f.Add("Software\\Microsoft\\Windows\\CurrentVersion")
	f.Add("ControlSet001")
	f.Add("currentversion_settings_key_name")
	f.Fuzz(func(t *testing.T, name string) {
		if !utf8.ValidString(name) {
			t.Skip()
		}
		got := Hash(name)
		want := hashReference(name)
		if got != want {
			t.Errorf("Hash(%q) = %d, want %d", name, got, want)
		}
	})
}

// BenchmarkMicro_Hash measures Hash performance for ASCII and Unicode inputs.
func BenchmarkMicro_Hash(b *testing.B) {
	b.Run("ASCII_Short", func(b *testing.B) {
		for range b.N {
			Hash("software")
		}
	})
	b.Run("ASCII_Long", func(b *testing.B) {
		for range b.N {
			Hash("currentversion_settings_key_name")
		}
	})
	b.Run("Unicode", func(b *testing.B) {
		for range b.N {
			Hash("Ünïcödé_Kéy_Nàmé")
		}
	})
}
