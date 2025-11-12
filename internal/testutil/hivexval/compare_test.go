package hivexval

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompare tests cross-validator comparison.
func TestCompare(t *testing.T) {
	hivePath := createTestHive(t)

	t.Run("same backend should match", func(t *testing.T) {
		v1 := Must(New(hivePath, &Options{UseReader: true}))
		defer v1.Close()

		v2 := Must(New(hivePath, &Options{UseReader: true}))
		defer v2.Close()

		result, err := v1.Compare(v2)
		require.NoError(t, err)
		require.True(t, result.Match, "same backend should match perfectly")
		require.Empty(t, result.Mismatches)
	})

	t.Run("bindings vs reader should match", func(t *testing.T) {
		v1 := Must(New(hivePath, &Options{UseBindings: true}))
		defer v1.Close()

		v2 := Must(New(hivePath, &Options{UseReader: true}))
		defer v2.Close()

		result, err := v1.Compare(v2)
		require.NoError(t, err)

		if !result.Match {
			t.Logf("Found %d mismatches:", len(result.Mismatches))
			for i, m := range result.Mismatches {
				if i >= 10 {
					t.Logf("... and %d more", len(result.Mismatches)-10)
					break
				}
				t.Logf("  [%s] %s: %s", m.Category, m.Path, m.Message)
			}
		}

		require.True(t, result.Match, "bindings and reader should produce same results")
		require.Positive(t, result.NodesCompared, "should have compared some nodes")
		require.Positive(t, result.ValuesCompared, "should have compared some values")
	})
}

// TestAssertMatchesValidator tests the assertion for matching validators.
func TestAssertMatchesValidator(t *testing.T) {
	hivePath := createTestHive(t)

	v1 := Must(New(hivePath, &Options{UseBindings: true}))
	defer v1.Close()

	v2 := Must(New(hivePath, &Options{UseReader: true}))
	defer v2.Close()

	v1.AssertMatchesValidator(t, v2)
}
