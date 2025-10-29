package repair

// Severity classifies how serious a diagnostic issue is
type Severity int

const (
	SevInfo     Severity = iota // Informational (unusual but valid)
	SevWarning                   // Non-critical issue, may affect performance
	SevError                     // Data loss or access failure, key/value inaccessible
	SevCritical                  // Structural corruption, prevents opening/parsing
)

func (s Severity) String() string {
	switch s {
	case SevInfo:
		return "INFO"
	case SevWarning:
		return "WARNING"
	case SevError:
		return "ERROR"
	case SevCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// DiagCategory classifies the type of issue found
type DiagCategory int

const (
	DiagStructure   DiagCategory = iota // REGF/HBIN/cell structure problems
	DiagData                             // Value data corruption or truncation
	DiagIntegrity                        // Checksums, links, references broken
	DiagPerformance                      // Fragmentation, inefficiency (info only)
)

func (c DiagCategory) String() string {
	switch c {
	case DiagStructure:
		return "STRUCTURE"
	case DiagData:
		return "DATA"
	case DiagIntegrity:
		return "INTEGRITY"
	case DiagPerformance:
		return "PERFORMANCE"
	default:
		return "UNKNOWN"
	}
}

// RepairType describes what kind of repair action is suggested
type RepairType int

const (
	RepairTruncate RepairType = iota // Reduce size to fit available data
	RepairRebuild                     // Reconstruct index or structure
	RepairRemove                      // Remove corrupt entry
	RepairReplace                     // Replace with corrected value
	RepairDefault                     // Use default/safe value
)

func (r RepairType) String() string {
	switch r {
	case RepairTruncate:
		return "TRUNCATE"
	case RepairRebuild:
		return "REBUILD"
	case RepairRemove:
		return "REMOVE"
	case RepairReplace:
		return "REPLACE"
	case RepairDefault:
		return "DEFAULT"
	default:
		return "UNKNOWN"
	}
}

// RiskLevel indicates how dangerous a repair action is
type RiskLevel int

const (
	RiskNone   RiskLevel = iota // No risk, purely metadata
	RiskLow                      // Minimal risk, easy to undo
	RiskMedium                   // Moderate risk, may lose data
	RiskHigh                     // High risk, significant data loss possible
)

func (r RiskLevel) String() string {
	switch r {
	case RiskNone:
		return "NONE"
	case RiskLow:
		return "LOW"
	case RiskMedium:
		return "MEDIUM"
	case RiskHigh:
		return "HIGH"
	default:
		return "UNKNOWN"
	}
}

// RepairAction describes how to fix the issue
type RepairAction struct {
	Type        RepairType `json:"type"`
	Description string     `json:"description"`
	Confidence  float32    `json:"confidence"`  // 0.0-1.0, how confident we are this will work
	Risk        RiskLevel  `json:"risk"`        // Safety level of this repair
	AutoApply   bool       `json:"auto_apply"` // Safe for automatic repair without user confirmation?
}

// Diagnostic represents a single issue found in the hive
type Diagnostic struct {
	// Classification
	Severity Severity     `json:"severity"`
	Category DiagCategory `json:"category"`

	// Location
	Offset       uint64 `json:"offset"`                  // Absolute byte offset in file
	Structure    string `json:"structure"`               // "REGF", "HBIN", "NK", "VK", "LH", "DB", etc
	StructOffset uint32 `json:"struct_offset,omitempty"` // Offset within structure (if applicable)

	// Description
	Issue    string      `json:"issue"`              // Human-readable description
	Expected interface{} `json:"expected,omitempty"` // Expected value (for validation errors)
	Actual   interface{} `json:"actual,omitempty"`   // Actual value found

	// Context (optional)
	Context *DiagContext `json:"context,omitempty"`

	// Repair suggestion (optional)
	Repair *RepairAction `json:"repair,omitempty"`
}

// DiagContext provides hierarchical context for better reporting
type DiagContext struct {
	KeyPath      string `json:"key_path,omitempty"`      // e.g., "HKLM\Software\Microsoft"
	ParentOffset uint32 `json:"parent_offset,omitempty"` // Parent NK offset
	CellOffset   uint32 `json:"cell_offset,omitempty"`   // Cell offset being processed
	ValueName    string `json:"value_name,omitempty"`    // Value name (if applicable)
}
