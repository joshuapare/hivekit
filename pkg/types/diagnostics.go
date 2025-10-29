package types

import (
	"encoding/json"
	"fmt"
	"sort"
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
// Re-exported from internal/repair for public API
type Severity = repair.Severity

const (
	SevInfo     = repair.SevInfo     // Informational (unusual but valid)
	SevWarning  = repair.SevWarning  // Non-critical issue, may affect performance
	SevError    = repair.SevError    // Data loss or access failure, key/value inaccessible
	SevCritical = repair.SevCritical // Structural corruption, prevents opening/parsing
)

// DiagCategory classifies the type of issue found
// Re-exported from internal/repair for public API
type DiagCategory = repair.DiagCategory

const (
	DiagStructure   = repair.DiagStructure   // REGF/HBIN/cell structure problems
	DiagData        = repair.DiagData        // Value data corruption or truncation
	DiagIntegrity   = repair.DiagIntegrity   // Checksums, links, references broken
	DiagPerformance = repair.DiagPerformance // Fragmentation, inefficiency (info only)
)

// RepairType describes what kind of repair action is suggested
// Re-exported from internal/repair for public API
type RepairType = repair.RepairType

const (
	RepairTruncate = repair.RepairTruncate // Reduce size to fit available data
	RepairRebuild  = repair.RepairRebuild  // Reconstruct index or structure
	RepairRemove   = repair.RepairRemove   // Remove corrupt entry
	RepairReplace  = repair.RepairReplace  // Replace with corrected value
	RepairDefault  = repair.RepairDefault  // Use default/safe value
)

// RiskLevel indicates how dangerous a repair action is
// Re-exported from internal/repair for public API
type RiskLevel = repair.RiskLevel

const (
	RiskNone   = repair.RiskNone   // No risk, purely metadata
	RiskLow    = repair.RiskLow    // Minimal risk, easy to undo
	RiskMedium = repair.RiskMedium // Moderate risk, may lose data
	RiskHigh   = repair.RiskHigh   // High risk, significant data loss possible
)

// Diagnostic represents a single issue found in the hive
// Re-exported from internal/repair for public API
type Diagnostic = repair.Diagnostic

// DiagContext provides hierarchical context for better reporting
// Re-exported from internal/repair for public API
type DiagContext = repair.DiagContext

// RepairAction describes how to fix the issue
// Re-exported from internal/repair for public API
type RepairAction = repair.RepairAction

// DiagnosticReport collects all diagnostics found during scan
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

// DiagSummary provides quick statistics
type DiagSummary struct {
	Critical int `json:"critical"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`

	Repairable     int `json:"repairable"`      // Issues with repair actions
	AutoRepairable int `json:"auto_repairable"` // Safe for auto-repair
}

// NewDiagnosticReport creates an empty report
func NewDiagnosticReport() *DiagnosticReport {
	return &DiagnosticReport{
		BySeverity:  make(map[Severity][]Diagnostic),
		ByStructure: make(map[string][]Diagnostic),
	}
}

// Add adds a diagnostic to the report and updates indices
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

// Finalize sorts diagnostics by offset and prepares for output
func (r *DiagnosticReport) Finalize() {
	// Sort by offset for sequential access patterns
	r.ByOffset = make([]Diagnostic, len(r.Diagnostics))
	copy(r.ByOffset, r.Diagnostics)
	sort.Slice(r.ByOffset, func(i, j int) bool {
		return r.ByOffset[i].Offset < r.ByOffset[j].Offset
	})
}

// HasCriticalIssues returns true if any critical issues were found
func (r *DiagnosticReport) HasCriticalIssues() bool {
	return r.Summary.Critical > 0
}

// HasErrors returns true if any errors or critical issues were found
func (r *DiagnosticReport) HasErrors() bool {
	return r.Summary.Critical > 0 || r.Summary.Errors > 0
}

// HasAnyIssues returns true if any issues were found (including warnings and info)
func (r *DiagnosticReport) HasAnyIssues() bool {
	return len(r.Diagnostics) > 0
}

// GetAutoRepairable returns all diagnostics that are safe for auto-repair
func (r *DiagnosticReport) GetAutoRepairable() []Diagnostic {
	result := make([]Diagnostic, 0, r.Summary.AutoRepairable)
	for _, d := range r.Diagnostics {
		if d.Repair != nil && d.Repair.AutoApply {
			result = append(result, d)
		}
	}
	return result
}

// GetByMaxRisk returns all diagnostics with repairs at or below the specified risk level
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

// FormatJSON returns the report as formatted JSON (2-space indentation)
func (r *DiagnosticReport) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatText returns a human-readable text report
func (r *DiagnosticReport) FormatText() string {
	var b strings.Builder

	// Header
	b.WriteString("=" + strings.Repeat("=", 78) + "\n")
	b.WriteString("Registry Hive Diagnostic Report\n")
	b.WriteString("=" + strings.Repeat("=", 78) + "\n\n")

	// Metadata
	if r.FilePath != "" {
		b.WriteString(fmt.Sprintf("File:      %s\n", r.FilePath))
	}
	b.WriteString(fmt.Sprintf("Size:      %d bytes\n", r.FileSize))
	b.WriteString(fmt.Sprintf("Scan time: %v\n\n", r.ScanTime))

	// Summary
	b.WriteString("SUMMARY\n")
	b.WriteString(strings.Repeat("-", 79) + "\n")
	b.WriteString(fmt.Sprintf("  Critical: %d\n", r.Summary.Critical))
	b.WriteString(fmt.Sprintf("  Errors:   %d\n", r.Summary.Errors))
	b.WriteString(fmt.Sprintf("  Warnings: %d\n", r.Summary.Warnings))
	b.WriteString(fmt.Sprintf("  Info:     %d\n\n", r.Summary.Info))

	if r.Summary.Repairable > 0 {
		b.WriteString(fmt.Sprintf("  Repairable:      %d\n", r.Summary.Repairable))
		b.WriteString(fmt.Sprintf("  Auto-repairable: %d\n\n", r.Summary.AutoRepairable))
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

		b.WriteString(fmt.Sprintf("%s (%d)\n", severity, len(diags)))
		b.WriteString(strings.Repeat("~", 79) + "\n")

		for i, d := range diags {
			b.WriteString(fmt.Sprintf("\n%d. [%s/%s] at offset 0x%X\n", i+1, d.Structure, d.Category, d.Offset))
			b.WriteString(fmt.Sprintf("   %s\n", d.Issue))

			if d.Expected != nil || d.Actual != nil {
				if d.Expected != nil {
					b.WriteString(fmt.Sprintf("   Expected: %v\n", d.Expected))
				}
				if d.Actual != nil {
					b.WriteString(fmt.Sprintf("   Actual:   %v\n", d.Actual))
				}
			}

			if d.Context != nil {
				if d.Context.KeyPath != "" {
					b.WriteString(fmt.Sprintf("   Path:     %s\n", d.Context.KeyPath))
				}
				if d.Context.ValueName != "" {
					b.WriteString(fmt.Sprintf("   Value:    %s\n", d.Context.ValueName))
				}
			}

			if d.Repair != nil {
				b.WriteString(fmt.Sprintf("   Repair:   %s\n", d.Repair.Description))
				b.WriteString(fmt.Sprintf("   Risk:     %s (confidence: %.0f%%)\n",
					d.Repair.Risk, d.Repair.Confidence*100))
				if d.Repair.AutoApply {
					b.WriteString("   Auto-apply: YES\n")
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatTextCompact returns a compact one-line-per-issue text format
func (r *DiagnosticReport) FormatTextCompact() string {
	var b strings.Builder

	for _, d := range r.ByOffset {
		b.WriteString(fmt.Sprintf("0x%08X [%s/%s/%s] %s\n",
			d.Offset, d.Severity, d.Structure, d.Category, d.Issue))
	}

	if len(r.Diagnostics) == 0 {
		b.WriteString("No issues found.\n")
	}

	return b.String()
}

// FormatHexAnnotations returns annotations suitable for hex dump overlays
// Format: offset,severity,structure,message
func (r *DiagnosticReport) FormatHexAnnotations() string {
	var b strings.Builder

	b.WriteString("# Hex dump annotations for diagnostic report\n")
	b.WriteString("# Format: offset,severity,structure,message\n\n")

	for _, d := range r.ByOffset {
		msg := strings.ReplaceAll(d.Issue, ",", ";") // Escape commas
		b.WriteString(fmt.Sprintf("0x%08X,%s,%s,%s\n",
			d.Offset, d.Severity, d.Structure, msg))
	}

	return b.String()
}
