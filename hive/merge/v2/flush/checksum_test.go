package flush

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeltaChecksum(t *testing.T) {
	// Fill a 508-byte header region with a known pattern.
	header := make([]byte, 508)
	for i := range 127 {
		header[i*4] = byte(i)
	}

	// Compute the full checksum before the change.
	fullXOR := ComputeFullChecksum(header)

	// Change one field at offset 4 and verify delta matches full recompute.
	oldVal := readU32(header, 4)
	newVal := uint32(0xDEADBEEF)
	writeU32(header, 4, newVal)
	fullAfter := ComputeFullChecksum(header)

	delta := DeltaChecksum(fullXOR, 4, oldVal, newVal)
	require.Equal(t, fullAfter, delta, "delta checksum should match full recompute after field change")
}

func TestComputeFullChecksum(t *testing.T) {
	// Build a header with deterministic content and verify against manual XOR.
	header := make([]byte, 508)
	for i := range 127 {
		writeU32(header, i*4, uint32(i+1))
	}

	var expected uint32
	for i := range 127 {
		expected ^= uint32(i + 1)
	}
	// Apply special-case remapping.
	if expected == 0 {
		expected = 1
	} else if expected == 0xFFFFFFFF {
		expected = 0xFFFFFFFE
	}

	got := ComputeFullChecksum(header)
	require.Equal(t, expected, got, "ComputeFullChecksum should match manual XOR")
}

func TestComputeFullChecksum_ZeroAndMax(t *testing.T) {
	t.Run("zero maps to one", func(t *testing.T) {
		// Construct a 508-byte header whose XOR is 0.
		// Use two identical DWORDs: A XOR A == 0. Fill all 127 slots with the same value.
		// 127 is odd so xor of 127 identical values == that value.
		// Instead, use pairs that cancel: write dword[0] = X, dword[1] = X, ... even pairs = 0,
		// leave one dword as 0 so total remains 0.
		header := make([]byte, 508)
		// All zero bytes means XOR is 0 → special case → 1.
		got := ComputeFullChecksum(header)
		require.Equal(t, uint32(1), got, "XOR==0 should return 1")
	})

	t.Run("0xFFFFFFFF maps to 0xFFFFFFFE", func(t *testing.T) {
		// Construct header whose XOR is 0xFFFFFFFF.
		// Write all 127 DWORDs as 0xFFFFFFFF; since 127 is odd, XOR = 0xFFFFFFFF.
		header := make([]byte, 508)
		for i := range 127 {
			writeU32(header, i*4, 0xFFFFFFFF)
		}
		got := ComputeFullChecksum(header)
		require.Equal(t, uint32(0xFFFFFFFE), got, "XOR==0xFFFFFFFF should return 0xFFFFFFFE")
	})
}
