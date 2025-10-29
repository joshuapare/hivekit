// Package format houses low-level decoders for the Windows Registry hive file
// format. The goal is to keep the parsing focused, allocation-free where
// possible, and independent from the public API so higher-level packages can
// orchestrate the data in a more ergonomic form.
package format

var (
	// REGFSignature is the four-byte signature at the start of every hive file.
	// Layout (little-endian):
	//   0x00  'r' 'e' 'g' 'f'
	REGFSignature = []byte{'r', 'e', 'g', 'f'}

	// HBINSignature is the four-byte signature at the beginning of each hive bin.
	// Layout:
	//   0x00  'h' 'b' 'i' 'n'
	HBINSignature = []byte{'h', 'b', 'i', 'n'}

	// NKSignature identifies an NK (Node Key) cell payload.
	NKSignature = []byte{'n', 'k'}

	// VKSignature identifies a VK (Value Key) cell payload.
	VKSignature = []byte{'v', 'k'}

	// LFSignature, LHSignature, and LISignature identify subkey list variants.
	// LF/LH include hashed names, while LI is a linear list without hashes.
	LFSignature = []byte{'l', 'f'}
	LHSignature = []byte{'l', 'h'}
	LISignature = []byte{'l', 'i'}

	// RISignature identifies an RI (indirect) subkey list record used when
	// a key has many subkeys. RI lists contain offsets to multiple LF/LH lists.
	RISignature = []byte{'r', 'i'}

	// SKSignature identifies a security descriptor (SK) cell.
	SKSignature = []byte{'s', 'k'}

	// DBSignature identifies a Big Data (DB) record for large registry values.
	DBSignature = []byte{'d', 'b'}
)

const (
	// HeaderSize is the size of the REGF header in bytes. In all observed hive
	// variants this is 4096 bytes (the size of a single memory page).
	HeaderSize = 4096

	// HBINHeaderSize is the size of the HBIN header in bytes.
	HBINHeaderSize = 0x20

	// CellHeaderSize is the number of bytes used by the cell header preceding
	// every allocation (free or in-use) within an HBIN.
	CellHeaderSize = 4

	// HBINAlignment is the required alignment of hive bins. On-disk structures
	// are aligned to 4 KiB.
	HBINAlignment = 0x1000

	// CellAlignment is the required alignment of cells within HBINs.
	// Cells are aligned to 8-byte boundaries.
	CellAlignment = 8

	// HBIN field offsets within the header structure
	HBINFileOffsetField = 0x04 // Offset to file offset field (4 bytes)
	HBINSizeOffset      = 0x08 // Offset to HBIN size field (4 bytes)

	// HBINDataSize is the usable data space in an HBIN (total size minus header)
	HBINDataSize = 0xFE0 // 4096 - 32 = 4064 bytes

	// InvalidOffset is a placeholder value used for unused/invalid offset fields
	InvalidOffset = 0xFFFFFFFF

	// ============================================================================
	// UTF-16 Encoding Constants
	// ============================================================================

	// UTF-16 surrogate pair ranges for encoding supplementary characters (U+10000 and above)
	UTF16HighSurrogateStart = 0xD800 // Start of high surrogate range
	UTF16HighSurrogateEnd   = 0xDBFF // End of high surrogate range
	UTF16LowSurrogateStart  = 0xDC00 // Start of low surrogate range
	UTF16LowSurrogateEnd    = 0xDFFF // End of low surrogate range
	UTF16SurrogateBase      = 0x10000 // Base value for surrogate pair calculations
	UTF16BMPMax             = 0xFFFF  // Maximum codepoint in Basic Multilingual Plane
	UTF16SurrogateMask      = 0x3FF   // Mask for extracting 10-bit surrogate values

	// UTF16ASCIIThreshold is the threshold for ASCII characters in UTF-16LE
	UTF16ASCIIThreshold = 0x80

	// ============================================================================
	// DB Record (Big Data) Constants
	// ============================================================================

	// DBMinSize is the minimum size of a DB record structure in bytes.
	DBMinSize = 12

	// DB field offsets within the record structure
	DBSignatureOffset  = 0x00 // Offset to 'db' signature (2 bytes)
	DBNumBlocksOffset  = 0x02 // Offset to number of blocks (2 bytes)
	DBBlocklistOffset  = 0x04 // Offset to blocklist offset (4 bytes)
	DBUnknown1Offset   = 0x08 // Offset to unknown1 field (4 bytes)

	// ============================================================================
	// NK Record (Node Key) Constants
	// ============================================================================

	// NKMinSize is the minimum size of an NK record structure in bytes.
	NKMinSize = 0x50

	// NK field offsets within the record structure
	NKSignatureOffset      = 0x00 // Offset to 'nk' signature (2 bytes)
	NKFlagsOffset          = 0x02 // Offset to flags field (2 bytes)
	NKLastWriteOffset      = 0x04 // Offset to last write timestamp (8 bytes, FILETIME)
	NKAccessBitsOffset     = 0x0C // Offset to access bits (4 bytes, Windows 8+, ignored)
	NKParentOffset         = 0x10 // Offset to parent cell offset (4 bytes)
	NKSubkeyCountOffset    = 0x14 // Offset to subkey count (4 bytes)
	NKVolSubkeyCountOffset = 0x18 // Offset to volatile subkey count (4 bytes, ignored)
	NKSubkeyListOffset     = 0x1C // Offset to subkey list offset (4 bytes)
	NKVolSubkeyListOffset  = 0x20 // Offset to volatile subkey list (4 bytes, ignored)
	NKValueCountOffset     = 0x24 // Offset to value count (4 bytes)
	NKValueListOffset      = 0x28 // Offset to value list offset (4 bytes)
	NKSecurityOffset       = 0x2C // Offset to security descriptor offset (4 bytes)
	NKClassNameOffset      = 0x30 // Offset to class name offset (4 bytes)
	NKMaxNameLenOffset     = 0x34 // Offset to max subkey name length (4 bytes)
	NKMaxClassLenOffset    = 0x38 // Offset to max class length (4 bytes)
	NKMaxValueNameOffset   = 0x3C // Offset to max value name length (4 bytes)
	NKMaxValueDataOffset   = 0x40 // Offset to max value data length (4 bytes)
	NKWorkVarOffset        = 0x44 // Offset to work var (4 bytes, ignored)
	NKNameLenOffset        = 0x48 // Offset to name length (2 bytes)
	NKClassLenOffset       = 0x4A // Offset to class length (2 bytes)
	NKNameOffset           = 0x4C // Offset where name bytes begin (variable length)

	// NK flag bit masks
	NKFlagCompressedName = 0x20 // Bit indicating name is stored in Windows-1252 (not UTF-16LE)

	// NK structure size constants (for hivex compatibility calculations)
	NKFixedHeaderSize = 0x4C // Size of fixed portion before variable-length name

	// ============================================================================
	// VK Record (Value Key) Constants
	// ============================================================================

	// VKMinSize is the minimum size of a VK record structure in bytes.
	VKMinSize = 0x14

	// VK field offsets within the record structure
	VKSignatureOffset = 0x00 // Offset to 'vk' signature (2 bytes)
	VKNameLenOffset   = 0x02 // Offset to name length (2 bytes)
	VKDataLenOffset   = 0x04 // Offset to data length (4 bytes, high bit = inline flag)
	VKDataOffsetField = 0x08 // Offset to data offset or inline data (4 bytes)
	VKTypeOffset      = 0x0C // Offset to value type (4 bytes)
	VKFlagsOffset     = 0x10 // Offset to flags (2 bytes)
	VKSpareOffset     = 0x12 // Offset to spare/padding (2 bytes)
	VKNameOffset      = 0x14 // Offset where name bytes begin (variable length)

	// VK flag bit masks
	VKFlagASCIIName  = 0x0001     // Bit indicating name is stored in Windows-1252
	VKDataInlineBit  = 0x80000000 // High bit of DataLength indicating data is inline
	VKDataLengthMask = 0x7FFFFFFF // Mask to extract actual data length from DataLength field

	// VK structure size constants (for hivex compatibility calculations)
	VKFixedHeaderSize   = 0x14 // Size of fixed portion before variable-length name
	VKHivexSizeConstant = 24   // Hivex formula: decoded_name_len + 24

	// ============================================================================
	// REGF Header Constants
	// ============================================================================

	// REGF field offsets within the header structure
	REGFSignatureOffset    = 0x00 // Offset to 'regf' signature (4 bytes)
	REGFSignatureSize      = 4    // Size of the signature
	REGFPrimarySeqOffset   = 0x04 // Offset to primary sequence number (4 bytes)
	REGFSecondarySeqOffset = 0x08 // Offset to secondary sequence number (4 bytes)
	REGFLastWriteOffset    = 0x0C // Offset to last write timestamp (8 bytes, FILETIME)
	REGFMajorVersionOffset = 0x14 // Offset to major version (4 bytes)
	REGFMinorVersionOffset = 0x18 // Offset to minor version (4 bytes)
	REGFTypeOffset         = 0x1C // Offset to type field (4 bytes: 0=primary, 1=alternate)
	REGFRootCellOffset     = 0x24 // Offset to root cell offset (4 bytes, relative to first HBIN)
	REGFDataSizeOffset     = 0x28 // Offset to total HBIN data size (4 bytes)
	REGFClusteringOffset   = 0x2C // Offset to clustering factor (4 bytes, rarely used)

	// ============================================================================
	// SK Record (Security Descriptor) Constants
	// ============================================================================

	// SKMinSize is the minimum size of an SK record structure in bytes.
	SKMinSize = 0x14

	// SK field offsets within the record structure
	SKSignatureOffset  = 0x00 // Offset to 'sk' signature (2 bytes)
	SKRevisionOffset   = 0x02 // Offset to revision (2 bytes)
	SKLengthOffset     = 0x04 // Offset to descriptor length (4 bytes)
	SKControlOffset    = 0x08 // Offset to control flags/ref count (4 bytes, unused)
	SKDescOffsetField  = 0x0C // Offset to descriptor offset relative to cell (4 bytes)
	SKReservedOffset   = 0x10 // Offset to reserved field (4 bytes)
	SKDataOffset       = 0x14 // Offset where SECURITY_DESCRIPTOR data begins

	// ============================================================================
	// Generic Constants
	// ============================================================================

	// SignatureSize is the standard size for most record signatures (NK, VK, SK, etc.).
	SignatureSize = 2

	// ============================================================================
	// List Structure Constants
	// ============================================================================

	// ListHeaderSize is the size of list headers (signature 2 bytes + count 2 bytes).
	// This applies to LI, LF, LH, and RI list structures.
	ListHeaderSize = 4

	// OffsetFieldSize is the size of cell offset fields (uint32).
	// Used in list entries, value lists, and various record references.
	OffsetFieldSize = 4

	// LFEntrySize is the size of each entry in LF/LH lists.
	// Each entry contains an offset (4 bytes) and a hash (4 bytes).
	LFEntrySize = 8

	// RIListEstimatedCapacity is the estimated number of entries per RI sub-list.
	// Used for pre-allocating buffers when decoding RI (indirect) subkey lists.
	RIListEstimatedCapacity = 100

	// ============================================================================
	// Registry Value Data Type Sizes
	// ============================================================================

	// DWORDSize is the size of REG_DWORD and REG_DWORD_BE values in bytes (uint32).
	DWORDSize = 4

	// QWORDSize is the size of REG_QWORD values in bytes (uint64).
	QWORDSize = 8

	// ============================================================================
	// DB (Big Data) Record Constants
	// ============================================================================

	// DBBlockPadding is the number of padding bytes at the end of each DB data block.
	// DB blocks include the next cell's header as padding that must be trimmed.
	DBBlockPadding = 4
)
