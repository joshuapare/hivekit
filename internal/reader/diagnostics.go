package reader

import (
	"sync"

	"github.com/joshuapare/hivekit/pkg/types"
)

// diagnosticCollector accumulates diagnostics during hive operations.
// It's nil in normal mode (zero overhead), and only allocated when
// OpenOptions.CollectDiagnostics is true or Diagnose() is called.
type diagnosticCollector struct {
	report *types.DiagnosticReport
	mu     sync.Mutex // protects concurrent access during parallel scans
}

// newDiagnosticCollector creates a new collector.
func newDiagnosticCollector() *diagnosticCollector {
	return &diagnosticCollector{
		report: types.NewDiagnosticReport(),
	}
}

// record adds a diagnostic to the collection.
func (dc *diagnosticCollector) record(d types.Diagnostic) {
	if dc == nil {
		return // hot path: no-op when collector is nil
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.report.Add(d)
}

// getReport returns the diagnostic report, finalizing it first.
func (dc *diagnosticCollector) getReport() *types.DiagnosticReport {
	if dc == nil {
		return nil
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.report.Finalize()
	return dc.report
}

// Helper functions for creating common diagnostics

// diagStructure creates a structure corruption diagnostic.
func diagStructure(
	severity types.Severity,
	offset uint64,
	structure string,
	issue string,
	expected, actual interface{},
	repair *types.RepairAction,
) types.Diagnostic {
	return types.Diagnostic{
		Severity:  severity,
		Category:  types.DiagStructure,
		Offset:    offset,
		Structure: structure,
		Issue:     issue,
		Expected:  expected,
		Actual:    actual,
		Repair:    repair,
	}
}

// diagData creates a data corruption diagnostic.
func diagData(
	severity types.Severity,
	offset uint64,
	structure string,
	issue string,
	expected, actual interface{},
	ctx *types.DiagContext,
	repair *types.RepairAction,
) types.Diagnostic {
	return types.Diagnostic{
		Severity:  severity,
		Category:  types.DiagData,
		Offset:    offset,
		Structure: structure,
		Issue:     issue,
		Expected:  expected,
		Actual:    actual,
		Context:   ctx,
		Repair:    repair,
	}
}

// diagIntegrity creates an integrity issue diagnostic.
func diagIntegrity(
	severity types.Severity,
	offset uint64,
	structure string,
	issue string,
	expected, actual interface{},
	ctx *types.DiagContext,
	repair *types.RepairAction,
) types.Diagnostic {
	return types.Diagnostic{
		Severity:  severity,
		Category:  types.DiagIntegrity,
		Offset:    offset,
		Structure: structure,
		Issue:     issue,
		Expected:  expected,
		Actual:    actual,
		Context:   ctx,
		Repair:    repair,
	}
}
