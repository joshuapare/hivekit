package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joshuapare/hivekit/internal/repair"
)

// -----------------------------------------------------------------------------
// Diagnostic System - Zero-Cost Forensics
// -----------------------------------------------------------------------------
//
// This diagnostic system provides forensics-grade issue tracking and repair
// suggestions WITHOUT impacting normal operation performance. Key design:
//   - Opt-in via OpenOptions.CollectDiagnostics (hot path has ~0ns overhead)
//   - Collects ALL issues, not just first error
//   - Provides exact byte offsets, context, and repair suggestions
//   - Supports multiple output formats (JSON, text, hex dump, repair scripts)
//
// Usage:
//   1. Passive: Open with CollectDiagnostics=true, call GetDiagnostics()
//   2. Active: Call Diagnose() to scan entire hive

// Severity classifies how serious a diagnostic issue is
// Re-exported from internal/repair for public API.
type Severity = repair.Severity

const (
	SevInfo     = repair.SevInfo     // Informational (unusual but valid)
	SevWarning  = repair.SevWarning  // Non-critical issue, may affect performance
	SevError    = repair.SevError    // Data loss or access failure, key/value inaccessible
	SevCritical = repair.SevCritical // Structural corruption, prevents opening/parsing
)

// DiagCategory classifies the type of issue found
// Re-exported from internal/repair for public API.
type DiagCategory = repair.DiagCategory

const (
	DiagStructure   = repair.DiagStructure   // REGF/HBIN/cell structure problems
	DiagData        = repair.DiagData        // Value data corruption or truncation
	DiagIntegrity   = repair.DiagIntegrity   // Checksums, links, references broken
	DiagPerformance = repair.DiagPerformance // Fragmentation, inefficiency (info only)
)

// RepairType describes what kind of repair action is suggested
// Re-exported from internal/repair for public API.
type RepairType = repair.RepairType

const (
	RepairTruncate = repair.RepairTruncate // Reduce size to fit available data
	RepairRebuild  = repair.RepairRebuild  // Reconstruct index or structure
	RepairRemove   = repair.RepairRemove   // Remove corrupt entry
	RepairReplace  = repair.RepairReplace  // Replace with corrected value
	RepairDefault  = repair.RepairDefault  // Use default/safe value
)

// RiskLevel indicates how dangerous a repair action is
// Re-exported from internal/repair for public API.
type RiskLevel = repair.RiskLevel

const (
	RiskNone   = repair.RiskNone   // No risk, purely metadata
	RiskLow    = repair.RiskLow    // Minimal risk, easy to undo
	RiskMedium = repair.RiskMedium // Moderate risk, may lose data
	RiskHigh   = repair.RiskHigh   // High risk, significant data loss possible
)

// Diagnostic represents a single issue found in the hive
// Re-exported from internal/repair for public API.
type Diagnostic = repair.Diagnostic

// DiagContext provides hierarchical context for better reporting
// Re-exported from internal/repair for public API.
type DiagContext = repair.DiagContext

// RepairAction describes how to fix the issue
// Re-exported from internal/repair for public API.
type RepairAction = repair.RepairAction

// DiagnosticReport collects all diagnostics found during scan.
type DiagnosticReport struct {
	// Metadata
	FilePath string        `json:"file_path,omitempty"`
	FileSize int64         `json:"file_size"`
	ScanTime time.Duration `json:"scan_time"`

	// Issues
	Diagnostics []Diagnostic `json:"diagnostics"`

	// Summary statistics
	Summary DiagSummary `json:"summary"`

	// Pre-computed groupings for efficient querying
	BySeverity  map[Severity][]Diagnostic `json:"by_severity,omitempty"`
	ByStructure map[string][]Diagnostic   `json:"by_structure,omitempty"`
	ByOffset    []Diagnostic              `json:"by_offset,omitempty"` // sorted by offset
}

// DiagSummary provides quick statistics.
type DiagSummary struct {
	Critical int `json:"critical"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`

	Repairable     int `json:"repairable"`      // Issues with repair actions
	AutoRepairable int `json:"auto_repairable"` // Safe for auto-repair
}

// NewDiagnosticReport creates an empty report.
func NewDiagnosticReport() *DiagnosticReport {
	return &DiagnosticReport{
		BySeverity:  make(map[Severity][]Diagnostic),
		ByStructure: make(map[string][]Diagnostic),
	}
}

// Add adds a diagnostic to the report and updates indices.
func (r *DiagnosticReport) Add(d Diagnostic) {
	r.Diagnostics = append(r.Diagnostics, d)

	// Update summary
	switch d.Severity {
	case SevCritical:
		r.Summary.Critical++
	case SevError:
		r.Summary.Errors++
	case SevWarning:
		r.Summary.Warnings++
	case SevInfo:
		r.Summary.Info++
	}

	if d.Repair != nil {
		r.Summary.Repairable++
		if d.Repair.AutoApply {
			r.Summary.AutoRepairable++
		}
	}

	// Update groupings
	r.BySeverity[d.Severity] = append(r.BySeverity[d.Severity], d)
	r.ByStructure[d.Structure] = append(r.ByStructure[d.Structure], d)
}

// Finalize sorts diagnostics by offset and prepares for output.
func (r *DiagnosticReport) Finalize() {
	// Sort by offset for sequential access patterns
	r.ByOffset = make([]Diagnostic, len(r.Diagnostics))
	copy(r.ByOffset, r.Diagnostics)
	sort.Slice(r.ByOffset, func(i, j int) bool {
		return r.ByOffset[i].Offset < r.ByOffset[j].Offset
	})
}

// HasCriticalIssues returns true if any critical issues were found.
func (r *DiagnosticReport) HasCriticalIssues() bool {
	return r.Summary.Critical > 0
}

// HasErrors returns true if any errors or critical issues were found.
func (r *DiagnosticReport) HasErrors() bool {
	return r.Summary.Critical > 0 || r.Summary.Errors > 0
}

// HasAnyIssues returns true if any issues were found (including warnings and info).
func (r *DiagnosticReport) HasAnyIssues() bool {
	return len(r.Diagnostics) > 0
}

// GetAutoRepairable returns all diagnostics that are safe for auto-repair.
func (r *DiagnosticReport) GetAutoRepairable() []Diagnostic {
	result := make([]Diagnostic, 0, r.Summary.AutoRepairable)
	for _, d := range r.Diagnostics {
		if d.Repair != nil && d.Repair.AutoApply {
			result = append(result, d)
		}
	}
	return result
}

// GetByMaxRisk returns all diagnostics with repairs at or below the specified risk level.
func (r *DiagnosticReport) GetByMaxRisk(maxRisk RiskLevel) []Diagnostic {
	result := make([]Diagnostic, 0)
	for _, d := range r.Diagnostics {
		if d.Repair != nil && d.Repair.Risk <= maxRisk {
			result = append(result, d)
		}
	}
	return result
}

// -----------------------------------------------------------------------------
// Output Formatters
// -----------------------------------------------------------------------------

// Helper functions for efficient formatting without allocations

// formatHex8 formats a uint64 as 8-digit uppercase hex string
func formatHex8(val uint64) string {
	const hexChars = "0123456789ABCDEF"
	var buf [8]byte
	for i := 7; i >= 0; i-- {
		buf[i] = hexChars[val&0xF]
		val >>= 4
	}
	return string(buf[:])
}

// writeInt writes an integer to the builder
func writeInt(b *strings.Builder, val int) {
	b.WriteString(strconv.Itoa(val))
}

// writeInt64 writes an int64 to the builder
func writeInt64(b *strings.Builder, val int64) {
	b.WriteString(strconv.FormatInt(val, 10))
}

// formatInterface formats an interface{} value as a string
// This is a cold path (diagnostics/errors only), so fmt.Sprint is acceptable
func formatInterface(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		// For unknown types, use fmt.Sprint
		// This is acceptable since it's a cold path (diagnostics only)
		return fmt.Sprint(v)
	}
}

// FormatJSON returns the report as formatted JSON (2-space indentation).
func (r *DiagnosticReport) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatText returns a human-readable text report.
func (r *DiagnosticReport) FormatText() string {
	var b strings.Builder

	// Header
	b.WriteString("=" + strings.Repeat("=", 78) + "\n")
	b.WriteString("Registry Hive Diagnostic Report\n")
	b.WriteString("=" + strings.Repeat("=", 78) + "\n\n")

	// Metadata
	if r.FilePath != "" {
		b.WriteString("File:      ")
		b.WriteString(r.FilePath)
		b.WriteByte('\n')
	}
	b.WriteString("Size:      ")
	writeInt64(&b, r.FileSize)
	b.WriteString(" bytes\n")
	b.WriteString("Scan time: ")
	b.WriteString(r.ScanTime.String())
	b.WriteString("\n\n")

	// Summary
	b.WriteString("SUMMARY\n")
	b.WriteString(strings.Repeat("-", 79) + "\n")
	b.WriteString("  Critical: ")
	writeInt(&b, r.Summary.Critical)
	b.WriteByte('\n')
	b.WriteString("  Errors:   ")
	writeInt(&b, r.Summary.Errors)
	b.WriteByte('\n')
	b.WriteString("  Warnings: ")
	writeInt(&b, r.Summary.Warnings)
	b.WriteByte('\n')
	b.WriteString("  Info:     ")
	writeInt(&b, r.Summary.Info)
	b.WriteString("\n\n")

	if r.Summary.Repairable > 0 {
		b.WriteString("  Repairable:      ")
		writeInt(&b, r.Summary.Repairable)
		b.WriteByte('\n')
		b.WriteString("  Auto-repairable: ")
		writeInt(&b, r.Summary.AutoRepairable)
		b.WriteString("\n\n")
	}

	// If no issues, report success
	if len(r.Diagnostics) == 0 {
		b.WriteString("No issues found.\n")
		return b.String()
	}

	// Diagnostics by severity
	b.WriteString("DIAGNOSTICS\n")
	b.WriteString(strings.Repeat("-", 79) + "\n\n")

	for _, severity := range []Severity{SevCritical, SevError, SevWarning, SevInfo} {
		diags := r.BySeverity[severity]
		if len(diags) == 0 {
			continue
		}

		b.WriteString(severity.String())
		b.WriteString(" (")
		writeInt(&b, len(diags))
		b.WriteString(")\n")
		b.WriteString(strings.Repeat("~", 79) + "\n")

		for i, d := range diags {
			b.WriteString("\n")
			writeInt(&b, i+1)
			b.WriteString(". [")
			b.WriteString(d.Structure)
			b.WriteByte('/')
			b.WriteString(d.Category.String())
			b.WriteString("] at offset 0x")
			b.WriteString(formatHex8(d.Offset))
			b.WriteByte('\n')
			b.WriteString("   ")
			b.WriteString(d.Issue)
			b.WriteByte('\n')

			if d.Expected != nil || d.Actual != nil {
				if d.Expected != nil {
					b.WriteString("   Expected: ")
					// Use Sprintf for interface{} values - unavoidable
					b.WriteString(formatInterface(d.Expected))
					b.WriteByte('\n')
				}
				if d.Actual != nil {
					b.WriteString("   Actual:   ")
					b.WriteString(formatInterface(d.Actual))
					b.WriteByte('\n')
				}
			}

			if d.Context != nil {
				if d.Context.KeyPath != "" {
					b.WriteString("   Path:     ")
					b.WriteString(d.Context.KeyPath)
					b.WriteByte('\n')
				}
				if d.Context.ValueName != "" {
					b.WriteString("   Value:    ")
					b.WriteString(d.Context.ValueName)
					b.WriteByte('\n')
				}
			}

			if d.Repair != nil {
				b.WriteString("   Repair:   ")
				b.WriteString(d.Repair.Description)
				b.WriteByte('\n')
				b.WriteString("   Risk:     ")
				b.WriteString(d.Repair.Risk.String())
				b.WriteString(" (confidence: ")
				writeInt(&b, int(d.Repair.Confidence*100))
				b.WriteString("%)\n")
				if d.Repair.AutoApply {
					b.WriteString("   Auto-apply: YES\n")
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatTextCompact returns a compact one-line-per-issue text format.
func (r *DiagnosticReport) FormatTextCompact() string {
	var b strings.Builder

	for _, d := range r.ByOffset {
		b.WriteString("0x")
		b.WriteString(formatHex8(d.Offset))
		b.WriteString(" [")
		b.WriteString(d.Severity.String())
		b.WriteByte('/')
		b.WriteString(d.Structure)
		b.WriteByte('/')
		b.WriteString(d.Category.String())
		b.WriteString("] ")
		b.WriteString(d.Issue)
		b.WriteByte('\n')
	}

	if len(r.Diagnostics) == 0 {
		b.WriteString("No issues found.\n")
	}

	return b.String()
}

// FormatHexAnnotations returns annotations suitable for hex dump overlays
// Format: offset,severity,structure,message.
func (r *DiagnosticReport) FormatHexAnnotations() string {
	var b strings.Builder

	b.WriteString("# Hex dump annotations for diagnostic report\n")
	b.WriteString("# Format: offset,severity,structure,message\n\n")

	for _, d := range r.ByOffset {
		msg := strings.ReplaceAll(d.Issue, ",", ";") // Escape commas
		b.WriteString("0x")
		b.WriteString(formatHex8(d.Offset))
		b.WriteByte(',')
		b.WriteString(d.Severity.String())
		b.WriteByte(',')
		b.WriteString(d.Structure)
		b.WriteByte(',')
		b.WriteString(msg)
		b.WriteByte('\n')
	}

	return b.String()
}
