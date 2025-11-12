package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// makeSKPayload builds a synthetic SK payload for testing.
// The mutate callback allows test-specific modifications.
func makeSKPayload(t *testing.T, descriptorLen uint32, mutate func([]byte)) []byte {
	t.Helper()

	// Allocate buffer: header (0x14) + descriptor data + some padding
	buf := make([]byte, format.SKHeaderSize+int(descriptorLen)+32)

	// Signature "sk" @ 0x00
	copy(buf[format.SKSignatureOffset:], format.SKSignature)

	// Reserved @ 0x02 (can be arbitrary)
	format.PutU16(buf, format.SKReservedOffset, 0x0000)

	// Flink @ 0x04 (forward link)
	format.PutU32(buf, format.SKFlinkOffset, 0x2000)

	// Blink @ 0x08 (backward link)
	format.PutU32(buf, format.SKBlinkOffset, 0x1000)

	// ReferenceCount @ 0x0C
	format.PutU32(buf, format.SKReferenceCountOffset, 1)

	// DescriptorLength @ 0x10
	format.PutU32(buf, format.SKDescriptorLengthOffset, descriptorLen)

	// Descriptor data @ 0x14 (fill with dummy data)
	for i := range descriptorLen {
		buf[format.SKDescriptorOffset+int(i)] = byte(i & 0xFF)
	}

	if mutate != nil {
		mutate(buf)
	}

	return buf[:format.SKDescriptorOffset+int(descriptorLen)]
}

// ============================================================================
// Basic Parsing Tests
// ============================================================================

func TestSK_ParseOK(t *testing.T) {
	// Why this test: Validates basic parsing and all field accessor methods.
	const descLen = 0x50
	payload := makeSKPayload(t, descLen, nil)

	sk, err := ParseSK(payload)
	require.NoError(t, err)

	// Verify signature
	require.Equal(t, "sk", string(payload[0:2]))

	// Verify Flink/Blink
	require.Equal(t, uint32(0x2000), sk.Flink())
	require.Equal(t, uint32(0x1000), sk.Blink())
	require.True(t, sk.HasFlink())
	require.True(t, sk.HasBlink())

	// Verify ReferenceCount
	require.Equal(t, uint32(1), sk.ReferenceCount())

	// Verify DescriptorLength
	require.Equal(t, uint32(descLen), sk.DescriptorLength())

	// Verify Descriptor returns correct slice
	desc := sk.Descriptor()
	require.Len(t, desc, int(descLen))
	// Verify it's the actual data (starts at offset 0x14)
	require.Equal(t, payload[format.SKDescriptorOffset:format.SKDescriptorOffset+descLen], desc)
}

func TestSK_BadSignature(t *testing.T) {
	// Why this test: Ensures signature validation catches corruption or
	// incorrect cell type references, even though Windows doesn't validate it.
	payload := makeSKPayload(t, 0x20, func(b []byte) {
		b[0] = 'x'
		b[1] = 'x'
	})

	_, err := ParseSK(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad signature")
}

func TestSK_TooSmall(t *testing.T) {
	// Why this test: Ensures we catch truncated cells that don't have
	// enough bytes for the minimum SK header (0x14 bytes).
	payload := make([]byte, 10)
	copy(payload, format.SKSignature)

	_, err := ParseSK(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too small")
}

func TestSK_DescriptorLengthExceedsCell(t *testing.T) {
	// Why this test: Critical security check - ensures we don't read beyond
	// the cell boundaries even if DescriptorLength claims more space exists.
	// This prevents out-of-bounds reads.
	payload := makeSKPayload(t, 0x20, func(b []byte) {
		// Set DescriptorLength to exceed the actual cell size
		format.PutU32(b, format.SKDescriptorLengthOffset, 0x1000)
	})

	_, err := ParseSK(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds cell")
}

// ============================================================================
// Security Descriptor Length Tests (Spec-specific)
// ============================================================================

func TestSK_ZeroLengthDescriptor(t *testing.T) {
	// Why this test: The spec notes that completely empty security descriptors
	// (no owner, no ACLs) are allowed and Windows accepts them.
	payload := makeSKPayload(t, 0, nil)

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(0), sk.DescriptorLength())

	desc := sk.Descriptor()
	require.Nil(t, desc)
}

func TestSK_OversizedDescriptorLength(t *testing.T) {
	// Why this test: The spec explicitly allows DescriptorLength to be larger
	// than the actual descriptor size. Extra bytes are simply ignored. This
	// tests that we handle this case correctly as long as it fits in the cell.
	const actualDescSize = 0x30
	const declaredDescSize = 0x50 // larger than actual

	buf := make([]byte, format.SKHeaderSize+declaredDescSize)
	copy(buf[format.SKSignatureOffset:], format.SKSignature)
	format.PutU32(buf, format.SKFlinkOffset, 0x2000)
	format.PutU32(buf, format.SKBlinkOffset, 0x1000)
	format.PutU32(buf, format.SKReferenceCountOffset, 1)
	format.PutU32(buf, format.SKDescriptorLengthOffset, declaredDescSize)

	// Fill only actualDescSize bytes with data
	for i := range actualDescSize {
		buf[format.SKDescriptorOffset+i] = byte(i)
	}

	sk, err := ParseSK(buf)
	require.NoError(t, err)
	require.Equal(t, uint32(declaredDescSize), sk.DescriptorLength())

	// Descriptor() returns what's declared, not what's "used"
	desc := sk.Descriptor()
	require.Len(t, desc, int(declaredDescSize))
}

func TestSK_MinimalDescriptor(t *testing.T) {
	// Why this test: Tests the smallest valid security descriptor.
	const minDescLen = 1
	payload := makeSKPayload(t, minDescLen, nil)

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(minDescLen), sk.DescriptorLength())

	desc := sk.Descriptor()
	require.Len(t, desc, minDescLen)
}

// ============================================================================
// Link List Tests (Flink/Blink)
// ============================================================================

func TestSK_SelfReferentialLinks(t *testing.T) {
	// Why this test: Volatile security descriptors form single-entry lists
	// where Flink and Blink point at the descriptor itself. This is the
	// standard way volatile descriptors are stored.
	const selfOffset = 0x3000
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKFlinkOffset, selfOffset)
		format.PutU32(b, format.SKBlinkOffset, selfOffset)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(selfOffset), sk.Flink())
	require.Equal(t, uint32(selfOffset), sk.Blink())
}

func TestSK_InvalidOffsetLinks(t *testing.T) {
	// Why this test: Tests the edge case where links are set to InvalidOffset
	// (0xFFFFFFFF), which shouldn't normally happen but could in corrupted hives.
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKFlinkOffset, format.InvalidOffset)
		format.PutU32(b, format.SKBlinkOffset, format.InvalidOffset)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(format.InvalidOffset), sk.Flink())
	require.Equal(t, uint32(format.InvalidOffset), sk.Blink())
	require.False(t, sk.HasFlink())
	require.False(t, sk.HasBlink())
}

func TestSK_AsymmetricLinks(t *testing.T) {
	// Why this test: In a proper list, Flink and Blink should form a
	// consistent chain. This tests that we can read asymmetric values,
	// even if they represent a corrupted list structure.
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKFlinkOffset, 0x5000)
		format.PutU32(b, format.SKBlinkOffset, 0x2000)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(0x5000), sk.Flink())
	require.Equal(t, uint32(0x2000), sk.Blink())
}

// ============================================================================
// Reference Count Tests
// ============================================================================

func TestSK_ReferenceCount_Zero(t *testing.T) {
	// Why this test: A reference count of zero is dangerous (potential UAF)
	// but we should be able to parse it. The spec notes this has been a major
	// source of vulnerabilities.
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKReferenceCountOffset, 0)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(0), sk.ReferenceCount())
}

func TestSK_ReferenceCount_MaxUint32(t *testing.T) {
	// Why this test: Tests the maximum possible reference count value.
	// The spec notes that Microsoft added overflow protection (2023-2024)
	// to prevent wrapping from 0xFFFFFFFF back to 0.
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKReferenceCountOffset, 0xFFFFFFFF)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(0xFFFFFFFF), sk.ReferenceCount())
}

func TestSK_ReferenceCount_Typical(t *testing.T) {
	// Why this test: Tests a typical reference count value. Most descriptors
	// have refcounts between 1 and ~24.4 million (max keys in a hive).
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKReferenceCountOffset, 42)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(42), sk.ReferenceCount())
}

func TestSK_ReferenceCount_ExceedsKeyCount(t *testing.T) {
	// Why this test: The spec notes that refcounts can legitimately exceed
	// the visible key count due to transacted operations (pending transaction
	// keys, UoWAddThisKey, UoWSetSecurityDescriptor operations).
	const highRefCount = 100_000_000
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU32(b, format.SKReferenceCountOffset, highRefCount)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(highRefCount), sk.ReferenceCount())
}

// ============================================================================
// Reserved Field Tests
// ============================================================================

func TestSK_ReservedField_NonZero(t *testing.T) {
	// Why this test: The spec states the Reserved field (offset 0x02) may
	// contain arbitrary data and is never accessed by the kernel. This tests
	// that we don't validate or care about its value.
	payload := makeSKPayload(t, 0x40, func(b []byte) {
		format.PutU16(b, format.SKReservedOffset, 0xDEAD)
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	// Should parse successfully regardless of Reserved field value
	require.Equal(t, uint32(0x40), sk.DescriptorLength())
}

// ============================================================================
// Descriptor Data Tests
// ============================================================================

func TestSK_Descriptor_ZeroCopy(t *testing.T) {
	// Why this test: Validates that Descriptor() returns a zero-copy slice
	// of the underlying buffer, not a copy.
	const descLen = 0x80
	payload := makeSKPayload(t, descLen, func(b []byte) {
		// Fill descriptor with recognizable pattern
		for i := range int(descLen) {
			b[format.SKDescriptorOffset+i] = byte(0xAA + (i % 16))
		}
	})

	sk, err := ParseSK(payload)
	require.NoError(t, err)

	desc := sk.Descriptor()
	require.Len(t, desc, int(descLen))

	// Verify it's the same underlying data (zero-copy)
	expectedDesc := payload[format.SKDescriptorOffset : format.SKDescriptorOffset+descLen]
	require.Equal(t, expectedDesc, desc)

	// Verify pattern
	require.Equal(t, byte(0xAA), desc[0])
	require.Equal(t, byte(0xAA+15), desc[15])
}

func TestSK_Descriptor_MultipleCallsSameSlice(t *testing.T) {
	// Why this test: Ensures multiple calls to Descriptor() return equivalent
	// slices (pointing to the same underlying data).
	payload := makeSKPayload(t, 0x40, nil)

	sk, err := ParseSK(payload)
	require.NoError(t, err)

	desc1 := sk.Descriptor()
	desc2 := sk.Descriptor()

	require.Equal(t, desc1, desc2)
	require.Len(t, desc2, len(desc1))
}

func TestSK_Descriptor_LargeDescriptor(t *testing.T) {
	// Why this test: Tests handling of large security descriptors.
	// While most are small, complex ACLs can create larger descriptors.
	const largeDescLen = 4096
	payload := makeSKPayload(t, largeDescLen, nil)

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(largeDescLen), sk.DescriptorLength())

	desc := sk.Descriptor()
	require.Len(t, desc, largeDescLen)
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestSK_ExactlyMinSize(t *testing.T) {
	// Why this test: Tests the boundary condition where the cell is exactly
	// the minimum size (0x14 bytes = header only, no descriptor data).
	payload := make([]byte, format.SKMinSize)
	copy(payload[format.SKSignatureOffset:], format.SKSignature)
	format.PutU32(payload, format.SKFlinkOffset, 0x2000)
	format.PutU32(payload, format.SKBlinkOffset, 0x1000)
	format.PutU32(payload, format.SKReferenceCountOffset, 1)
	format.PutU32(payload, format.SKDescriptorLengthOffset, 0) // zero length descriptor

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(0), sk.DescriptorLength())
	require.Nil(t, sk.Descriptor())
}

func TestSK_OneByteShortOfMinSize(t *testing.T) {
	// Why this test: Tests that we properly reject cells that are just
	// one byte short of the minimum size.
	payload := make([]byte, format.SKMinSize-1)
	copy(payload, format.SKSignature)

	_, err := ParseSK(payload)
	require.Error(t, err)
}

func TestSK_DescriptorAtExactBoundary(t *testing.T) {
	// Why this test: Tests that we can parse a descriptor that exactly
	// fills the cell with no extra padding bytes.
	const descLen = 0x60
	payload := make([]byte, format.SKHeaderSize+descLen)
	copy(payload[format.SKSignatureOffset:], format.SKSignature)
	format.PutU32(payload, format.SKFlinkOffset, 0x2000)
	format.PutU32(payload, format.SKBlinkOffset, 0x1000)
	format.PutU32(payload, format.SKReferenceCountOffset, 1)
	format.PutU32(payload, format.SKDescriptorLengthOffset, descLen)

	// Fill descriptor
	for i := range int(descLen) {
		payload[format.SKDescriptorOffset+i] = byte(i)
	}

	sk, err := ParseSK(payload)
	require.NoError(t, err)
	require.Equal(t, uint32(descLen), sk.DescriptorLength())
	require.Len(t, sk.Descriptor(), int(descLen))
}

// ============================================================================
// Integration with CellSpec Pattern
// ============================================================================

func TestSK_InHBINContext(t *testing.T) {
	// Why this test: Tests SK parsing in the context of a full HBIN structure,
	// similar to how it would be encountered in a real hive file.
	const descLen = 0x50
	skPayload := makeSKPayload(t, descLen, nil)

	// Build an HBIN with the SK cell
	cells := []CellSpec{
		{
			Allocated: true,
			Size:      len(skPayload) + format.CellHeaderSize,
			Payload:   skPayload,
		},
	}

	hbin := buildHBINFromSpec(t, cells)

	// Extract the cell payload (skip cell header)
	cellStart := format.HBINHeaderSize + format.CellHeaderSize
	extractedPayload := hbin[cellStart : cellStart+len(skPayload)]

	// Parse the extracted SK
	sk, err := ParseSK(extractedPayload)
	require.NoError(t, err)
	require.Equal(t, uint32(descLen), sk.DescriptorLength())
	require.Len(t, sk.Descriptor(), int(descLen))
}
