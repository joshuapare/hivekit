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

	// Base of where the hive data starts (first HBIN).
	HiveDataBase = 0x1000

	// HBINAlignment is the required alignment of hive bins. On-disk structures
	// are aligned to 4 KiB.
	HBINAlignment = 0x1000

	// CellAlignment is the required alignment of cells within HBINs.
	// Cells are aligned to 8-byte boundaries.
	CellAlignment = 8

	// CellAlignmentMask is the bitmask used for aligning to 8-byte boundaries (CellAlignment - 1).
	CellAlignmentMask = CellAlignment - 1

	// HBINAlignmentMask is the bitmask used for aligning to 4KB boundaries (HBINAlignment - 1).
	HBINAlignmentMask = HBINAlignment - 1

	// Align16Boundary is the 16-byte alignment boundary.
	Align16Boundary = 16

	// Align16Mask is the bitmask used for aligning to 16-byte boundaries (Align16Boundary - 1).
	Align16Mask = Align16Boundary - 1

	// HBIN field offsets within the header structure.
	HBINSignatureOffset  = 0x000 // 4
	HBINSignatureSize    = 4
	HBINOffsetEchoOffset = 0x04
	HBINFileOffsetField  = 0x04 // Offset to file offset field (4 bytes)
	HBINSizeOffset       = 0x08 // Offset to HBIN size field (4 bytes)

	// HBINDataSize is the usable data space in an HBIN (total size minus header).
	HBINDataSize = 0xFE0 // 4096 - 32 = 4064 bytes

	// InvalidOffset is a placeholder value used for unused/invalid offset fields.
	InvalidOffset = 0xFFFFFFFF

	// ============================================================================
	// UTF-16 Encoding Constants
	// ============================================================================.

	// UTF-16 surrogate pair ranges for encoding supplementary characters (U+10000 and above).
	UTF16HighSurrogateStart = 0xD800  // Start of high surrogate range
	UTF16HighSurrogateEnd   = 0xDBFF  // End of high surrogate range
	UTF16LowSurrogateStart  = 0xDC00  // Start of low surrogate range
	UTF16LowSurrogateEnd    = 0xDFFF  // End of low surrogate range
	UTF16SurrogateBase      = 0x10000 // Base value for surrogate pair calculations
	UTF16BMPMax             = 0xFFFF  // Maximum codepoint in Basic Multilingual Plane
	UTF16SurrogateMask      = 0x3FF   // Mask for extracting 10-bit surrogate values

	// UTF16ASCIIThreshold is the threshold for ASCII characters in UTF-16LE.
	UTF16ASCIIThreshold = 0x80
)

// ============================================================================
// DB Record (Big Data) Constants
// ============================================================================
// DB field offsets within the record structure (_CM_BIG_DATA).
const (
	DBSignatureOffset = 0x00 // USHORT, "db"
	DBCountOffset     = 0x02 // USHORT, number of data blocks (must be 2-65535)
	DBListOffset      = 0x04 // ULONG, HCELL_INDEX to blocklist cell
	DBUnknown1Offset  = 0x08 // ULONG, unknown field (never accessed)
)

// derived lengths.
const (
	DBSignatureLen = DBCountOffset - DBSignatureOffset // 0x02
	DBCountLen     = DBListOffset - DBCountOffset      // 0x02
	DBListLen      = DBUnknown1Offset - DBListOffset   // 0x04
	DBUnknown1Len  = 4                                 // 0x04
)

// header size and minimum size.
const (
	DBHeaderSize = DBUnknown1Offset + DBUnknown1Len // 0x0C (12 bytes)
	DBMinSize    = DBHeaderSize                     // minimum valid DB cell size
)

// DB data block constraints.
const (
	// DBChunkSize is the size of each data block in a big data record.
	//
	// Why 16,344 bytes: Starting in Windows XP (hive version 1.4), Microsoft
	// added support for registry values exceeding 16KB by splitting them into
	// chunks. Each chunk stores 16,344 bytes of actual data (16KB minus the
	// 4-byte cell header overhead that follows each data block).
	//
	// Why this matters: The last chunk may be smaller if the total value length
	// isn't evenly divisible by 16,344. All other chunks should be exactly this
	// size. Deviations may indicate corruption or non-standard formatting.
	DBChunkSize = 16344

	// DBMinBlockCount is the minimum number of blocks in a valid DB record.
	//
	// Why 2 minimum: Per the Windows spec, if a value is empty (0 blocks) or
	// fits in a single block (1 block), it should use inline storage or a
	// direct cell reference instead of the DB format. A count of 0 or 1
	// indicates structural corruption or misuse of the DB format.
	//
	// Security note: Older Windows versions had integer overflow bugs that
	// could result in count = 0 or 1, though without exploitable impact.
	DBMinBlockCount = 2

	// DBMaxBlockCount is the maximum number of blocks (limited by uint16).
	//
	// Why 65535: The Count field is a 16-bit unsigned integer, so the maximum
	// value is 2^16 - 1 = 65535. This allows values up to ~1GB in size
	// (65535 chunks × 16,344 bytes ≈ 1,071,104,040 bytes).
	DBMaxBlockCount = 65535

	// DBBlockPadding is the number of padding bytes at the end of each DB data block.
	//
	// Why 4 bytes: Each data block is followed by the next cell's header (4 bytes),
	// which must be trimmed when assembling the value data. This is the standard
	// cell header size that appears between all registry cells.
	DBBlockPadding = 4
)

// ============================================================================
// NK Record (Node Key) Constants
// ============================================================================
// NK field offsets within the record structure (payload start == "nk").
const (
	NKSignatureOffset      = 0x00 // USHORT, "nk"
	NKFlagsOffset          = 0x02 // USHORT
	NKLastWriteOffset      = 0x04 // LARGE_INTEGER / FILETIME (8 bytes)
	NKAccessBitsOffset     = 0x0C // ULONG, "Spare" on older hives, AccessBits on Win8+ :contentReference[oaicite:2]{index=2}
	NKParentOffset         = 0x10 // ULONG HCELL_INDEX of parent
	NKSubkeyCountOffset    = 0x14 // ULONG stable subkey count
	NKVolSubkeyCountOffset = 0x18 // ULONG volatile subkey count
	NKSubkeyListOffset     = 0x1C // ULONG HCELL_INDEX to stable subkey list
	NKVolSubkeyListOffset  = 0x20 // ULONG HCELL_INDEX to volatile subkey list
	NKValueCountOffset     = 0x24 // DWORD value count (CHILD_LIST.Count)
	NKValueListOffset      = 0x28 // DWORD HCELL_INDEX to value list (CHILD_LIST.List)
	NKSecurityOffset       = 0x2C // DWORD HCELL_INDEX to SK
	NKClassNameOffset      = 0x30 // DWORD HCELL_INDEX to class data
	NKMaxNameLenOffset     = 0x34 // LOWORD: MaxNameLen, plus flags in high bits
	NKMaxClassLenOffset    = 0x38 // DWORD
	NKMaxValueNameOffset   = 0x3C // DWORD
	NKMaxValueDataOffset   = 0x40 // DWORD
	NKWorkVarOffset        = 0x44 // DWORD
	NKNameLenOffset        = 0x48 // USHORT name length (bytes!)
	NKClassLenOffset       = 0x4A // USHORT class length (bytes)
	NKNameOffset           = 0x4C // start of inline name
)

// derived lengths.
const (
	NKSignatureLen       = NKFlagsOffset - NKSignatureOffset      // 0x02
	NKFlagsLen           = NKLastWriteOffset - NKFlagsOffset      // 0x02
	NKLastWriteLen       = NKAccessBitsOffset - NKLastWriteOffset // 0x08
	NKAccessBitsLen      = NKParentOffset - NKAccessBitsOffset    // 0x04
	NKParentLen          = NKSubkeyCountOffset - NKParentOffset   // 0x04
	NKSubkeyCountLen     = NKVolSubkeyCountOffset - NKSubkeyCountOffset
	NKVolSubkeyCountLen  = NKSubkeyListOffset - NKVolSubkeyCountOffset
	NKSubkeyListLen      = NKVolSubkeyListOffset - NKSubkeyListOffset
	NKVolSubkeyListLen   = NKValueCountOffset - NKVolSubkeyListOffset
	NKValueCountLen      = NKValueListOffset - NKValueCountOffset // 0x04 (CHILD_LIST.Count)
	NKValueListLen       = NKSecurityOffset - NKValueListOffset   // 0x04 (CHILD_LIST.List)
	NKSecurityLen        = NKClassNameOffset - NKSecurityOffset
	NKClassNameLen       = NKMaxNameLenOffset - NKClassNameOffset
	NKMaxNameLenLen      = NKMaxClassLenOffset - NKMaxNameLenOffset
	NKMaxClassLenLen     = NKMaxValueNameOffset - NKMaxClassLenOffset
	NKMaxValueNameLenLen = NKMaxValueDataOffset - NKMaxValueNameOffset
	NKMaxValueDataLen    = NKWorkVarOffset - NKMaxValueDataOffset
	NKWorkVarLen         = NKNameLenOffset - NKWorkVarOffset
	NKNameLenLen         = NKClassLenOffset - NKNameLenOffset // 0x02
	NKClassLenLen        = NKNameOffset - NKClassLenOffset    // 0x02
)

// flags.
const (
	NKFlagCompressedName = 0x20 // KEY_COMP_NAME :contentReference[oaicite:3]{index=3}
)

// "header" is really "offset where name starts".
const (
	NKFixedHeaderSize = NKNameOffset // 0x4C
	NKMinSize         = NKFixedHeaderSize
)

// =====
// List Record Constants
// =====

// Common header layout for all subkey list cells (_CM_KEY_INDEX header).
const (
	IdxSignatureOffset = 0x00 // 2 bytes
	IdxCountOffset     = 0x02 // 2 bytes
	IdxListOffset      = 0x04 // start of variable-length array

	IdxSignatureLen = IdxCountOffset - IdxSignatureOffset // 2
	IdxCountLen     = IdxListOffset - IdxCountOffset      // 2
)

// Element sizes.
const (
	LIEntrySize = 4 // one uint32 cell index
	// For LF/LH leaves, each element is a CM_INDEX consisting of:
	//   uint32 Cell; uint32 HintOrHash;
	LFFHEntrySize = 8
)

// Minimal payload size: header only (no entries). The kernel disallows empty
// *active* lists, but loaders may encounter crafted hives; keep checks separate.
const IdxMinHeader = IdxListOffset // 0x04

// ============================================================================
// VK Record (Value Key) Constants
// ============================================================================.
const (
	// VKMinSize is the minimum size of a VK record structure in bytes.
	VKMinSize = 0x14

	// VK field offsets within the record structure.
	VKSignatureOffset = 0x00 // Offset to 'vk' signature (2 bytes)
	VKNameLenOffset   = 0x02 // Offset to name length (2 bytes)
	VKDataLenOffset   = 0x04 // Offset to data length (4 bytes, high bit = inline flag)
	VKDataOffOffset   = 0x08 // Offset to data offset or inline data (4 bytes)
	VKTypeOffset      = 0x0C // Offset to value type (4 bytes)
	VKFlagsOffset     = 0x10 // Offset to flags (2 bytes)
	VKSpareOffset     = 0x12 // Offset to spare/padding (2 bytes)
	VKNameOffset      = 0x14 // Offset where name bytes begin (variable length)

	// VK flag bit masks.
	VKFlagNameCompressed = 0x0001
	VKFlagASCIIName      = 0x0001     // Bit indicating name is stored in Windows-1252
	VKDataInlineBit      = 0x80000000 // High bit of DataLength indicating data is inline
	VKDataLengthMask     = 0x7FFFFFFF // Mask to extract actual data length from DataLength field

	// VK structure size constants (for hivex compatibility calculations).
	VKFixedHeaderSize   = 0x14 // Size of fixed portion before variable-length name
	VKHivexSizeConstant = 24   // Hivex formula: decoded_name_len + 24

	// DataLen bit31 indicates "small data" (1..4 bytes inline in DataOff).
	VKSmallDataMask = 0x8000_0000

	// Common registry types we’ll see in tests/parsing paths.
	RegNone     = 0
	RegSz       = 1
	RegExpandSz = 2
	RegBinary   = 3
	RegDword    = 4
	RegDwordBE  = 5
	RegLink     = 6
	RegMultiSz  = 7
	RegQword    = 11
)

// ============================================================================
// REGF Header Constants
// ============================================================================

const (
	// 0x000.. fields.
	REGFSignatureOffset     = 0x000 // 4
	REGFSignatureSize       = 4
	REGFPrimarySeqOffset    = 0x004 // Sequence1 (uint32)
	REGFSecondarySeqOffset  = 0x008 // Sequence2 (uint32)
	REGFTimeStampOffset     = 0x00C // _LARGE_INTEGER (uint64 LE, Windows FILETIME)
	REGFMajorVersionOffset  = 0x014 // uint32
	REGFMinorVersionOffset  = 0x018 // uint32
	REGFTypeOffset          = 0x01C // uint32
	REGFFormatOffset        = 0x020 // uint32
	REGFRootCellOffset      = 0x024 // uint32 (HCELL index rel to 0x1000)
	REGFDataSizeOffset      = 0x028 // uint32 (sum of HBIN sizes)
	REGFClusterOffset       = 0x02C // uint32
	REGFFileNameOffset      = 0x030 // [64] byte
	REGFFileNameSize        = 64
	REGFRmIDOffset          = 0x070 // _GUID
	REGFLogIDOffset         = 0x080 // _GUID
	REGFFlagsOffset         = 0x090 // uint32
	REGFTmIDOffset          = 0x094 // _GUID
	REGFGuidSigOffset       = 0x0A4 // uint32 (can be "OfRg" from offreg)
	REGFLastReorgTimeOffset = 0x0A8 // uint64 (FILETIME or special markers 0x1/0x2)
	REGFReserved1Offset     = 0x0B0 // [83] uint32 -> 0x1FC
	REGFCheckSumOffset      = 0x1FC // uint32 (XOR of first 508 bytes)
	// 0x200.. fields.
	REGFReserved2Offset = 0x200 // [882] uint32 -> 0xFC8
	REGFThawTmIdOffset  = 0xFC8 // _GUID
	REGFThawRmIdOffset  = 0xFD8 // _GUID
	REGFThawLogIdOffset = 0xFE8 // _GUID
	REGFBootTypeOffset  = 0xFF8 // uint32
	REGFBootRecovOffset = 0xFFC // uint32
)

// GUID size and helpers.
const GUIDSize = 16

// Header checksum covers the first 508 bytes (0x000..0x1FB), i.e. 127 dwords.
const (
	REGFChecksumRegionLen = 508
	REGFChecksumDwords    = 127
)

// Flags at 0x90 (observed):.
const (
	REGFFlagPendingTransactions = 0x00000001
	REGFFlagDifferencingHive    = 0x00000002 // layered keys (v1.6 typical)
)

// ============================================================================
// SK Record (Security Descriptor) Constants
// ============================================================================
// SK field offsets within the record structure (_CM_KEY_SECURITY).
const (
	SKSignatureOffset        = 0x00 // USHORT, "sk"
	SKReservedOffset         = 0x02 // USHORT, unused/arbitrary data
	SKFlinkOffset            = 0x04 // ULONG, forward link in security descriptor list
	SKBlinkOffset            = 0x08 // ULONG, backward link in security descriptor list
	SKReferenceCountOffset   = 0x0C // ULONG, number of key nodes using this descriptor
	SKDescriptorLengthOffset = 0x10 // ULONG, length of descriptor data in bytes
	SKDescriptorOffset       = 0x14 // start of SECURITY_DESCRIPTOR_RELATIVE data
)

// derived lengths.
const (
	SKSignatureLen        = SKReservedOffset - SKSignatureOffset              // 0x02
	SKReservedLen         = SKFlinkOffset - SKReservedOffset                  // 0x02
	SKFlinkLen            = SKBlinkOffset - SKFlinkOffset                     // 0x04
	SKBlinkLen            = SKReferenceCountOffset - SKBlinkOffset            // 0x04
	SKReferenceCountLen   = SKDescriptorLengthOffset - SKReferenceCountOffset // 0x04
	SKDescriptorLengthLen = SKDescriptorOffset - SKDescriptorLengthOffset     // 0x04
)

// header size and minimum size.
const (
	SKHeaderSize = SKDescriptorOffset // 0x14 (20 bytes before descriptor data)
	SKMinSize    = SKHeaderSize       // minimum valid SK cell size
)

// ============================================================================
// Generic Constants
// ============================================================================.
const (
	// SignatureSize is the standard size for most record signatures (NK, VK, SK, etc.).
	SignatureSize = 2

	// ============================================================================
	// List Structure Constants
	// ============================================================================.

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
	// ============================================================================.

	// DWORDSize is the size of REG_DWORD and REG_DWORD_BE values in bytes (uint32).
	DWORDSize = 4

	// QWORDSize is the size of REG_QWORD values in bytes (uint64).
	QWORDSize = 8

	// ============================================================================
	// Windows Registry Value Type Codes
	// ============================================================================
	// See: https://docs.microsoft.com/en-us/windows/win32/sysinfo/registry-value-types

	// REGNone indicates no defined value type.
	REGNone uint32 = 0

	// REGSZ is a null-terminated string (Unicode).
	REGSZ uint32 = 1

	// REGExpandSZ is a null-terminated string with environment variable references.
	REGExpandSZ uint32 = 2

	// REGBinary is arbitrary binary data.
	REGBinary uint32 = 3

	// REGDWORD is a 32-bit little-endian number.
	REGDWORD uint32 = 4

	// REGDWORDBigEndian is a 32-bit big-endian number.
	REGDWORDBigEndian uint32 = 5

	// REGLink is a symbolic link (Unicode).
	REGLink uint32 = 6

	// REGMultiSZ is a sequence of null-terminated strings, terminated by an empty string.
	REGMultiSZ uint32 = 7

	// REGResourceList is a device-driver resource list.
	REGResourceList uint32 = 8

	// REGFullResourceDescriptor is a hardware resource descriptor.
	REGFullResourceDescriptor uint32 = 9

	// REGResourceRequirementsList is a hardware resource requirements list.
	REGResourceRequirementsList uint32 = 10

	// REGQWORD is a 64-bit little-endian number.
	REGQWORD uint32 = 11
)
