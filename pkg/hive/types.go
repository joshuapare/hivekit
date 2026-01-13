package hive

import "github.com/joshuapare/hivekit/pkg/types"

// Re-export commonly used types from pkg/types so users only need to import pkg/hive

// Core types.
type (
	NodeID  = types.NodeID
	ValueID = types.ValueID
)

// Metadata types.
type (
	ValueMeta        = types.ValueMeta
	KeyMeta          = types.KeyMeta
	KeyDetail        = types.KeyDetail
	HiveInfo         = types.HiveInfo
	DiagnosticReport = types.DiagnosticReport
	RiskLevel        = types.RiskLevel
	RepairOptions    = types.RepairOptions
)

// Diagnostic types.
type (
	Severity     = types.Severity
	DiagCategory = types.DiagCategory
	RepairType   = types.RepairType
	Diagnostic   = types.Diagnostic
	DiagContext  = types.DiagContext
	RepairAction = types.RepairAction
)

// Diagnostic constructor.
var NewDiagnosticReport = types.NewDiagnosticReport

// ReadOptions controls value reading behavior.
type ReadOptions = types.ReadOptions

// WriteOptions controls transaction write behavior (for Tx.Commit).
// For high-level operation options, see OperationOptions.
type WriteOptions = types.WriteOptions

// CreateKeyOptions controls CreateKey behavior.
type CreateKeyOptions = types.CreateKeyOptions

// DeleteKeyOptions controls DeleteKey behavior.
type DeleteKeyOptions = types.DeleteKeyOptions

// RegType enumerates Windows registry value types.
type RegType = types.RegType

// Registry type constants.
const (
	REG_NONE      = types.REG_NONE
	REG_SZ        = types.REG_SZ
	REG_EXPAND_SZ = types.REG_EXPAND_SZ
	REG_BINARY    = types.REG_BINARY
	REG_DWORD     = types.REG_DWORD
	REG_DWORD_LE  = types.REG_DWORD_LE
	REG_DWORD_BE  = types.REG_DWORD_BE
	REG_LINK      = types.REG_LINK
	REG_MULTI_SZ  = types.REG_MULTI_SZ
	REG_QWORD     = types.REG_QWORD
)

// Interface re-exports for advanced users.
type (
	Reader = types.Reader
)

// .reg file types.
type (
	RegParseOptions  = types.RegParseOptions
	RegExportOptions = types.RegExportOptions
	RegCodec         = types.RegCodec
)

// Error types.
type (
	Error   = types.Error
	ErrKind = types.ErrKind
)

// Error kind constants.
const (
	ErrKindFormat      = types.ErrKindFormat
	ErrKindCorrupt     = types.ErrKindCorrupt
	ErrKindUnsupported = types.ErrKindUnsupported
	ErrKindNotFound    = types.ErrKindNotFound
	ErrKindType        = types.ErrKindType
	ErrKindState       = types.ErrKindState
)

// Severity constants.
const (
	SevInfo     = types.SevInfo
	SevWarning  = types.SevWarning
	SevError    = types.SevError
	SevCritical = types.SevCritical
)

// Diagnostic category constants.
const (
	DiagStructure   = types.DiagStructure
	DiagData        = types.DiagData
	DiagIntegrity   = types.DiagIntegrity
	DiagPerformance = types.DiagPerformance
)

// Repair type constants.
const (
	RepairTruncate = types.RepairTruncate
	RepairRebuild  = types.RepairRebuild
	RepairRemove   = types.RepairRemove
	RepairReplace  = types.RepairReplace
	RepairDefault  = types.RepairDefault
)

// Risk level constants.
const (
	RiskNone   = types.RiskNone
	RiskLow    = types.RiskLow
	RiskMedium = types.RiskMedium
	RiskHigh   = types.RiskHigh
)

// Common error sentinels.
var (
	ErrNotHive      = types.ErrNotHive
	ErrCorrupt      = types.ErrCorrupt
	ErrUnsupported  = types.ErrUnsupported
	ErrNotFound     = types.ErrNotFound
	ErrTypeMismatch = types.ErrTypeMismatch
	ErrReadonly     = types.ErrReadonly
)
