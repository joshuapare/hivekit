package regtext

const (
	// ============================================================================
	// .reg File Format Tokens
	// ============================================================================

	// RegFileHeader is the required header line for .reg files version 5.00
	RegFileHeader = "Windows Registry Editor Version 5.00"

	// ============================================================================
	// Delimiters and Structural Tokens
	// ============================================================================

	// KeyOpenBracket marks the start of a registry key path
	KeyOpenBracket = "["

	// KeyCloseBracket marks the end of a registry key path
	KeyCloseBracket = "]"

	// DeleteKeyPrefix marks a key for deletion (e.g., [-HKEY_LOCAL_MACHINE\...])
	DeleteKeyPrefix = "-"

	// ValueAssignment separates value names from their data
	ValueAssignment = "="

	// DefaultValuePrefix marks the default (unnamed) value
	DefaultValuePrefix = "@="

	// CommentPrefix marks a comment line
	CommentPrefix = ";"

	// ============================================================================
	// Quote and Escape Characters
	// ============================================================================

	// Quote is the double-quote character for value names and string data
	Quote = "\""

	// Backslash is used for escaping and path separators
	Backslash = "\\"

	// EscapedQuote is the escaped double-quote sequence
	EscapedQuote = "\\\""

	// EscapedBackslash is the escaped backslash sequence
	EscapedBackslash = "\\\\"

	// ForwardSlash is an alternative path separator (normalized to backslash)
	ForwardSlash = "/"

	// ============================================================================
	// Line Endings
	// ============================================================================

	// CRLF is the Windows line ending (carriage return + line feed)
	CRLF = "\r\n"

	// CR is the carriage return character
	CR = "\r"

	// LF is the line feed character
	LF = "\n"

	// ============================================================================
	// Value Type Prefixes
	// ============================================================================

	// DWORDPrefix identifies a DWORD value in .reg format
	DWORDPrefix = "dword:"

	// HexPrefix identifies binary data in .reg format
	HexPrefix = "hex:"

	// HexExpandSZPrefix identifies REG_EXPAND_SZ values (type 2)
	HexExpandSZPrefix = "hex(2):"

	// HexMultiSZPrefix identifies REG_MULTI_SZ values (type 7)
	HexMultiSZPrefix = "hex(7):"

	// HexTypeFormat is the format string for typed hex values: hex(%s):
	HexTypeFormat = "hex(%s):"

	// ============================================================================
	// Encoding Names
	// ============================================================================

	// EncodingUTF8 is the identifier for UTF-8 encoding
	EncodingUTF8 = "UTF-8"

	// EncodingUTF16LE is the identifier for UTF-16 little-endian encoding
	EncodingUTF16LE = "UTF-16LE"

	// ============================================================================
	// Hex Data Formatting
	// ============================================================================

	// HexByteSeparator separates bytes in hex data
	HexByteSeparator = ","

	// HexByteFormat is the format string for a single hex byte
	HexByteFormat = "%02x"

	// DWORDHexFormat is the format string for DWORD values (8 hex digits)
	DWORDHexFormat = "%08x"

	// DWORDHexLength is the expected length of a DWORD hex string
	DWORDHexLength = 8

	// ============================================================================
	// Registry Key Path Prefixes (HKEY roots)
	// ============================================================================

	// These are the standard Windows registry root key names and abbreviations

	HKEYLocalMachine      = "HKEY_LOCAL_MACHINE"
	HKEYLocalMachineShort = "HKLM"

	HKEYClassesRoot      = "HKEY_CLASSES_ROOT"
	HKEYClassesRootShort = "HKCR"

	HKEYCurrentUser      = "HKEY_CURRENT_USER"
	HKEYCurrentUserShort = "HKCU"

	HKEYUsers      = "HKEY_USERS"
	HKEYUsersShort = "HKU"

	HKEYCurrentConfig      = "HKEY_CURRENT_CONFIG"
	HKEYCurrentConfigShort = "HKCC"

	// ============================================================================
	// Value Type Detection Strings
	// ============================================================================

	// ValueTypeString identifies string values in .reg format
	ValueTypeString = "string"

	// ValueTypeDWORD identifies DWORD values
	ValueTypeDWORD = "dword"

	// ValueTypeBinary identifies binary values
	ValueTypeBinary = "binary"

	// ValueTypeHex is an alias for binary
	ValueTypeHex = "hex"

	// ValueTypeHex2 identifies REG_EXPAND_SZ
	ValueTypeHex2 = "hex(2)"

	// ValueTypeHex7 identifies REG_MULTI_SZ
	ValueTypeHex7 = "hex(7)"

	// ValueTypeUnknown is used for unrecognized value types
	ValueTypeUnknown = "unknown"

	// ============================================================================
	// Value Deletion Token
	// ============================================================================

	// DeleteValueToken marks a value for deletion
	DeleteValueToken = "-"

	// ============================================================================
	// Buffer and Parsing Sizes
	// ============================================================================

	// ScannerInitialBufferSize is the initial buffer size for the .reg file scanner
	ScannerInitialBufferSize = 64 * 1024 // 64KB

	// ScannerMaxLineSize is the maximum line size for the .reg file scanner
	ScannerMaxLineSize = 1024 * 1024 // 1MB

	// InitialKeyCapacity is the estimated number of keys for pre-allocation
	InitialKeyCapacity = 1000

	// ============================================================================
	// UTF-16 Encoding Constants
	// ============================================================================

	// UTF16CodeUnitSize is the size of a UTF-16 code unit in bytes
	UTF16CodeUnitSize = 2
)

var (
	// UTF16LEBOM is the byte order mark for UTF-16 little-endian
	UTF16LEBOM = []byte{0xFF, 0xFE}

	// UTF8BOM is the byte order mark for UTF-8
	UTF8BOM = []byte{0xEF, 0xBB, 0xBF}

	// DoubleNullTerminator is used to terminate REG_MULTI_SZ values
	DoubleNullTerminator = []byte{0x00, 0x00}
)
