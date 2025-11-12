package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// NK is a zero-cost view over an "nk" (key node) cell payload.
// It does NOT own memory; it only points into the hive buffer.
type NK struct {
	buf []byte // payload only (starts with "nk")
	off int    // offset inside buf where "nk" starts (usually 0)
}

// ParseNK wraps a cell payload as NK and validates the signature.
func ParseNK(payload []byte) (NK, error) {
	if len(payload) < format.NKFixedHeaderSize { // 0x4C
		return NK{}, fmt.Errorf("hive: NK too small: %d", len(payload))
	}
	// Optimized: Use byte comparison instead of string allocation
	if payload[0] != 'n' || payload[1] != 'k' {
		return NK{}, fmt.Errorf("hive: NK bad sig: %c%c", payload[0], payload[1])
	}
	return NK{buf: payload, off: 0}, nil
}

// Flags returns the NK flags field.
func (n NK) Flags() uint16 {
	return format.ReadU16(n.buf, n.off+format.NKFlagsOffset)
}

// LastWriteFILETIME returns the last write timestamp as raw FILETIME bytes (8 bytes).
func (n NK) LastWriteFILETIME() []byte {
	start := n.off + format.NKLastWriteOffset
	end := start + format.NKLastWriteLen // 8
	if end > len(n.buf) {
		return nil
	}
	return n.buf[start:end]
}

// AccessBits returns the access bits / spare field at offset 0x0C.
func (n NK) AccessBits() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKAccessBitsOffset)
}

// ParentOffsetRel returns the parent cell index (relative).
func (n NK) ParentOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKParentOffset)
}

// SubkeyCount returns the stable subkey count.
func (n NK) SubkeyCount() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKSubkeyCountOffset)
}

// VolatileSubkeyCount returns the volatile subkey count (almost always 0 on-disk).
func (n NK) VolatileSubkeyCount() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKVolSubkeyCountOffset)
}

// SubkeyListOffsetRel returns the stable subkey list offset (HCELL_INDEX).
func (n NK) SubkeyListOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKSubkeyListOffset)
}

// VolatileSubkeyListOffsetRel returns the volatile subkey list offset (HCELL_INDEX).
func (n NK) VolatileSubkeyListOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKVolSubkeyListOffset)
}

// ValueCount returns CHILD_LIST.Count.
func (n NK) ValueCount() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKValueCountOffset)
}

// ValueListOffsetRel returns CHILD_LIST.List (HCELL_INDEX).
func (n NK) ValueListOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKValueListOffset)
}

// SecurityOffsetRel returns the security descriptor cell offset (HCELL_INDEX).
func (n NK) SecurityOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKSecurityOffset)
}

// ClassNameOffsetRel returns the class name cell offset (HCELL_INDEX).
func (n NK) ClassNameOffsetRel() uint32 {
	return format.ReadU32(n.buf, n.off+format.NKClassNameOffset)
}

// MaxNameLen returns the maximum subkey name length field (low 16 bits at offset 0x34).
func (n NK) MaxNameLen() uint16 {
	return format.ReadU16(n.buf, n.off+format.NKMaxNameLenOffset)
}

// NameLength returns the key name length in bytes.
func (n NK) NameLength() uint16 {
	if n.off+format.NKNameLenOffset+2 > len(n.buf) {
		return 0
	}
	return format.ReadU16(n.buf, n.off+format.NKNameLenOffset)
}

// ClassLength returns the class name length in bytes.
func (n NK) ClassLength() uint16 {
	if n.off+format.NKClassLenOffset+2 > len(n.buf) {
		return 0
	}
	return format.ReadU16(n.buf, n.off+format.NKClassLenOffset)
}

// IsCompressedName returns true if the key name is ASCII-encoded (not UTF-16LE).
func (n NK) IsCompressedName() bool {
	return n.Flags()&format.NKFlagCompressedName != 0
}

// Name returns the raw key name bytes (ASCII if compressed, UTF-16LE otherwise).
func (n NK) Name() []byte {
	nl := n.NameLength()
	if nl == 0 {
		return nil
	}
	start := n.off + format.NKNameOffset // 0x4C
	end := start + int(nl)
	if end > len(n.buf) {
		return nil
	}
	return n.buf[start:end]
}

// ============================================================================
// High-Level Resolver Methods
// ============================================================================

// SubkeyListResult represents a resolved subkey list.
// The list can be any of: LF, LH, LI, or RI.
// Only one of the union fields is valid, determined by Kind.
//
// Why not interface{}: Type safety. Caller knows exactly which types are possible.
// Why not separate methods: Common code path for resolution regardless of list type.
//
// Memory layout: This is a union-like struct. Go doesn't have true unions, but
// since LF/LH/LI/RI are all zero-copy wrappers around []byte slices with minimal
// overhead (~16 bytes each), the total struct size is ~64 bytes. Only one field
// is populated. Alternative designs (interface{}, type switch) would require
// heap allocation and indirection, which is worse for performance.
type SubkeyListResult struct {
	Kind SubkeyListKind
	LF   LF
	LH   LH
	LI   LI
	RI   RI
}

// ResolveSubkeyList resolves and parses the subkey list referenced by this NK.
// Returns SubkeyListResult which contains the parsed list (LF, LH, LI, or RI).
//
// Why this exists: NK.SubkeyListOffsetRel() returns a raw offset, but the
// referenced cell can be any of 4 different list types. This method handles
// resolution, type detection, and parsing in one call.
//
// Why SubkeyListResult: The caller needs to know which type was parsed to
// access the correct field. Using interface{} would lose type safety.
//
// Returns error if:
//   - SubkeyCount() is 0 (no subkeys to list)
//   - SubkeyListOffsetRel() is 0xFFFFFFFF (invalid/missing list)
//   - Cell resolution fails
//   - List signature is unrecognized
//   - List parsing fails
func (n NK) ResolveSubkeyList(h *Hive) (SubkeyListResult, error) {
	count := n.SubkeyCount()
	if count == 0 {
		return SubkeyListResult{}, errors.New("hive: NK has no subkeys")
	}

	offset := n.SubkeyListOffsetRel()
	if offset == format.InvalidOffset {
		return SubkeyListResult{}, errors.New("hive: NK subkey list offset is invalid (0xFFFFFFFF)")
	}

	// Resolve the cell
	payload, err := resolveRelCellPayload(h.Bytes(), offset)
	if err != nil {
		return SubkeyListResult{}, fmt.Errorf("hive: failed to resolve subkey list: %w", err)
	}

	// Detect list type and parse
	kind := DetectListKind(payload)
	var result SubkeyListResult
	result.Kind = kind

	switch kind {
	case ListLF:
		result.LF, err = ParseLF(payload)
	case ListLH:
		result.LH, err = ParseLH(payload)
	case ListLI:
		result.LI, err = ParseLI(payload)
	case ListRI:
		result.RI, err = ParseRI(payload)
	case ListUnknown:
		return SubkeyListResult{}, fmt.Errorf("hive: unknown subkey list signature: %q", payload[:2])
	}

	if err != nil {
		return SubkeyListResult{}, fmt.Errorf("hive: failed to parse subkey list: %w", err)
	}

	return result, nil
}

// ResolveValueList resolves and parses the value list referenced by this NK.
// Returns ValueList which provides indexed access to VK offsets.
//
// Why this exists: NK.ValueListOffsetRel() returns a raw offset to a cell
// containing an array of VK offsets. This method handles resolution and
// parsing in one call.
//
// Returns error if:
//   - ValueCount() is 0 (no values)
//   - ValueListOffsetRel() is 0xFFFFFFFF (invalid/missing list)
//   - Cell resolution fails
//   - ValueList parsing fails (e.g., cell too small for claimed count)
func (n NK) ResolveValueList(h *Hive) (ValueList, error) {
	count := n.ValueCount()
	if count == 0 {
		return ValueList{}, errors.New("hive: NK has no values")
	}

	offset := n.ValueListOffsetRel()
	if offset == format.InvalidOffset {
		return ValueList{}, errors.New("hive: NK value list offset is invalid (0xFFFFFFFF)")
	}

	// Resolve the cell
	payload, err := resolveRelCellPayload(h.Bytes(), offset)
	if err != nil {
		return ValueList{}, fmt.Errorf("hive: failed to resolve value list: %w", err)
	}

	// Parse the value list
	vl, err := ParseValueList(payload, int(count))
	if err != nil {
		return ValueList{}, fmt.Errorf("hive: failed to parse value list: %w", err)
	}

	return vl, nil
}

// ResolveSecurity resolves and parses the security descriptor referenced by this NK.
// Returns SK which provides access to the security descriptor data.
//
// Why this exists: NK.SecurityOffsetRel() returns a raw offset to an SK cell.
// This method handles resolution and parsing in one call.
//
// Why security descriptors matter: They control access permissions for the
// registry key. Multiple NK cells often share the same SK (via refcounting).
//
// Returns error if:
//   - SecurityOffsetRel() is 0xFFFFFFFF (no security descriptor)
//   - Cell resolution fails
//   - SK parsing fails
func (n NK) ResolveSecurity(h *Hive) (SK, error) {
	offset := n.SecurityOffsetRel()
	if offset == format.InvalidOffset {
		return SK{}, errors.New("hive: NK security offset is invalid (0xFFFFFFFF)")
	}

	// Resolve the cell
	payload, err := resolveRelCellPayload(h.Bytes(), offset)
	if err != nil {
		return SK{}, fmt.Errorf("hive: failed to resolve security cell: %w", err)
	}

	// Parse the SK cell
	sk, err := ParseSK(payload)
	if err != nil {
		return SK{}, fmt.Errorf("hive: failed to parse SK cell: %w", err)
	}

	return sk, nil
}

// ResolveClassName resolves the class name data referenced by this NK.
// Returns the raw class name bytes (typically UTF-16LE encoded).
//
// Why this exists: NK.ClassNameOffsetRel() returns a raw offset to a data
// cell containing the class name. This method handles resolution.
//
// Why class names: The class name is a rarely-used registry feature that
// associates arbitrary data with a key. Most keys have no class name
// (offset = 0xFFFFFFFF).
//
// Returns error if:
//   - ClassLength() is 0 (no class name)
//   - ClassNameOffsetRel() is 0xFFFFFFFF (no class name)
//   - Cell resolution fails
func (n NK) ResolveClassName(h *Hive) ([]byte, error) {
	classLen := n.ClassLength()
	if classLen == 0 {
		return nil, errors.New("hive: NK has no class name (length = 0)")
	}

	offset := n.ClassNameOffsetRel()
	if offset == format.InvalidOffset {
		return nil, errors.New("hive: NK class name offset is invalid (0xFFFFFFFF)")
	}

	// Resolve the cell
	payload, err := resolveRelCellPayload(h.Bytes(), offset)
	if err != nil {
		return nil, fmt.Errorf("hive: failed to resolve class name cell: %w", err)
	}

	// Validate length
	if len(payload) < int(classLen) {
		return nil, fmt.Errorf("hive: class name cell too small: need %d bytes, have %d",
			classLen, len(payload))
	}

	// Return the class name bytes (typically UTF-16LE)
	return payload[:classLen], nil
}
