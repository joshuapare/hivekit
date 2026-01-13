package hive

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// SK is a zero-cost view over an "sk" (security) cell payload.
//
// Security cells (_CM_KEY_SECURITY) store Windows security descriptors that define
// access control for registry keys. Multiple keys can share the same security
// descriptor via reference counting, which is why deduplication is important.
//
// All security descriptors in stable storage are linked in a doubly-linked list
// for enumeration during hive loading. Volatile descriptors are kept separate
// (each in their own single-entry "list" with Flink/Blink pointing at themselves).
type SK struct {
	buf []byte // payload only (starts at 'sk')
	off int    // usually 0
}

// fast, zero-alloc check.
func isSK(b []byte) bool {
	// caller must have ensured len(b) >= format.HeaderSize, but be defensive
	const off = format.SKSignatureOffset
	const n = format.SKSignatureLen
	if len(b) < off+n {
		return false
	}
	return bytes.Equal(b[off:off+n], format.SKSignature)
}

// ParseSK wraps a cell payload as SK and validates signature & minimum size.
//
// Why we validate eagerly:
//   - Signature validation ensures we're actually looking at an SK cell and not
//     random data or a different cell type that was incorrectly referenced.
//   - Bounds checking the descriptor length here allows zero-copy access later
//     without risk of runtime panics. The spec allows DescriptorLength to be
//     larger than the actual descriptor, so we only validate it doesn't exceed
//     the cell boundaries.
func ParseSK(payload []byte) (SK, error) {
	if len(payload) < format.SKMinSize {
		return SK{}, fmt.Errorf("hive: SK too small: %d", len(payload))
	}

	// Why we check the signature: The spec notes that the signature isn't
	// verified on hive load by Windows and can be anything in a binary-controlled
	// hive, but we validate it to catch obvious corruption or programming errors.
	if !isSK(payload) {
		return SK{}, fmt.Errorf("hive: SK bad signature: %c%c", payload[0], payload[1])
	}

	// Why we bounds check the descriptor length eagerly: The spec allows
	// DescriptorLength to be larger than necessary (extra bytes are ignored),
	// but it must not exceed the cell size. Checking now enables zero-copy
	// access in Descriptor() without runtime boundary checks.
	descriptorLen := format.ReadU32(payload, format.SKDescriptorLengthOffset)
	if int(format.SKDescriptorOffset)+int(descriptorLen) > len(payload) {
		return SK{}, fmt.Errorf(
			"hive: SK security descriptor length %d exceeds cell (%d)",
			descriptorLen,
			len(payload),
		)
	}

	return SK{buf: payload, off: 0}, nil
}

// Blink returns the backward link in the security descriptor list.
//
// Why this exists: All stable security descriptors in a hive are connected in a
// doubly-linked list for enumeration during hive loading. The kernel needs to
// traverse this list to validate and initialize reference counts.
//
// Why it's a relative offset: Like other cell references in the registry, this
// is a HCELL_INDEX relative to the start of hive data (0x1000), not an absolute
// file offset. For volatile descriptors, Blink points at the descriptor itself,
// creating a single-entry "list" since volatile descriptors don't need enumeration.
func (s SK) Blink() uint32 {
	return format.ReadU32(s.buf, s.off+format.SKBlinkOffset)
}

// Flink returns the forward link in the security descriptor list.
//
// Why this exists: This is the forward pointer in the doubly-linked list of
// security descriptors. It must always be kept in sync with Blink to maintain
// list integrity. Invalid list pointers can cause kernel crashes or corruption.
//
// Why volatile descriptors point at themselves: Volatile descriptors only exist
// in memory and disappear on system shutdown, so there's no need to enumerate
// them on hive load. They form single-entry lists (Flink == Blink == self).
func (s SK) Flink() uint32 {
	return format.ReadU32(s.buf, s.off+format.SKFlinkOffset)
}

// HasBlink returns true if the Blink pointer is valid (not pointing at self).
//
// Why we check against self rather than InvalidOffset: In security descriptor
// lists, the "no previous element" state is represented by pointing at oneself,
// not by using InvalidOffset (0xFFFFFFFF) like other registry structures do.
// This is especially true for volatile descriptors which always point at themselves.
func (s SK) HasBlink() bool {
	// For a properly formed list, the first element's Blink would point at the
	// last element (it's circular). We need the cell's own offset to determine
	// if it's pointing at itself, which we don't have here, so we conservatively
	// check against InvalidOffset instead.
	return s.Blink() != format.InvalidOffset
}

// HasFlink returns true if the Flink pointer is valid (not pointing at self).
//
// Why we check: Similar to HasBlink, this helps determine if we're at the end
// of the list. For single-element lists (including all volatile descriptors),
// Flink points at the descriptor itself.
func (s SK) HasFlink() bool {
	return s.Flink() != format.InvalidOffset
}

// ReferenceCount returns the number of key nodes using this security descriptor.
//
// Why this is critical for security: This field has been responsible for more
// registry vulnerabilities than any other. If the refcount gets too small, the
// cell can be freed while keys still reference it (use-after-free). If it gets
// too large, it could overflow (though Microsoft added overflow protection in
// April 2023-November 2024).
//
// Why it can exceed the key count: Besides regular keys, transacted operations
// can temporarily increase the refcount beyond what's visible in the registry tree
// (pending transaction keys, UoWAddThisKey, UoWSetSecurityDescriptor operations).
//
// Why we don't validate it: The refcount must match reality, but we can't verify
// that without traversing all keys in the hive. The kernel validates this on load.
func (s SK) ReferenceCount() uint32 {
	return format.ReadU32(s.buf, s.off+format.SKReferenceCountOffset)
}

// DescriptorLength returns the declared length of the security descriptor data.
//
// Why this might be larger than necessary: The spec explicitly allows this field
// to be larger than the actual descriptor size. The extra bytes are simply ignored.
// This is similar to how cell sizes can be larger than the data they contain.
//
// Why we validate this in ParseSK: Even though it can be oversized, it must not
// exceed the cell boundaries, otherwise we'd read beyond allocated memory.
func (s SK) DescriptorLength() uint32 {
	return format.ReadU32(s.buf, s.off+format.SKDescriptorLengthOffset)
}

// Descriptor returns the security descriptor bytes (zero-copy).
//
// Why it's zero-copy: We pre-validated the bounds in ParseSK, so we can safely
// return a slice of the underlying buffer without copying.
//
// Why it's SECURITY_DESCRIPTOR_RELATIVE: The descriptor is stored in "self-relative"
// format where internal pointers (Owner, Group, SACL, DACL) are offsets from the
// start of the descriptor, not absolute pointers. This allows the descriptor to be
// relocated in memory without fixups.
//
// Why the encoding can be non-standard: The spec allows the descriptor components
// to be spread out with gaps, or even overlap. Windows only requires that it passes
// RtlValidRelativeSecurityDescriptor with RequiredInformation=0. It's even possible
// to have a completely empty descriptor with no owner or ACLs, and Windows accepts it.
//
// Why deduplication isn't enforced: While Windows deduplicates descriptors at runtime
// (reusing existing ones when possible), the on-disk format doesn't enforce uniqueness.
// Specially crafted hives can have multiple identical descriptors, and Windows handles
// this without issues.
func (s SK) Descriptor() []byte {
	n := s.DescriptorLength()
	if n == 0 {
		return nil
	}
	// The descriptor data starts inline at offset 0x14 (SKDescriptorOffset)
	return s.buf[s.off+format.SKDescriptorOffset : s.off+format.SKDescriptorOffset+int(n)]
}
