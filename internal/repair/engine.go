package repair

import (
	"errors"
	"fmt"
	"time"
)

// Engine orchestrates the repair process, coordinating validation, transaction
// logging, and module execution. It ensures repairs are applied safely and
// atomically: either all repairs succeed or the hive is restored to its
// original state.
type Engine struct {
	// Core components
	validator *Validator
	txLog     *TransactionLog
	writer    *Writer

	// Registered repair modules (indexed by name)
	modules map[string]RepairModule

	// Configuration
	config EngineConfig
}

// EngineConfig contains configuration options for the repair engine.
type EngineConfig struct {
	// DryRun simulates repairs without actually modifying data
	DryRun bool

	// AutoOnly only applies repairs marked as safe for auto-apply
	AutoOnly bool

	// MaxRisk is the maximum risk level allowed for auto-repair
	MaxRisk RiskLevel

	// Verbose enables detailed logging
	Verbose bool

	// BackupPath is where to store the backup (if empty, no backup)
	BackupPath string
}

// EngineResult contains the results of a repair operation.
type EngineResult struct {
	// Counts
	Applied int // Number of repairs successfully applied
	Skipped int // Number of repairs skipped (risk/config)
	Failed  int // Number of repairs that failed

	// Timing
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// Details
	Repairs        []RepairDetail  // Details of each repair attempt
	RollbackCount  int             // Number of repairs rolled back (if failure)
	BackupPath     string          // Path to backup file (if created)
	TransactionLog *TransactionLog // Full transaction log for debugging
}

// RepairDetail contains details about a single repair operation.
type RepairDetail struct {
	Diagnostic Diagnostic
	Module     string
	Status     RepairStatus
	Error      error
	Duration   time.Duration
}

// RepairStatus indicates the outcome of a repair attempt.
type RepairStatus int

const (
	RepairStatusPending RepairStatus = iota
	RepairStatusApplied
	RepairStatusSkipped
	RepairStatusFailed
	RepairStatusRolledBack
)

func (s RepairStatus) String() string {
	switch s {
	case RepairStatusPending:
		return "PENDING"
	case RepairStatusApplied:
		return "APPLIED"
	case RepairStatusSkipped:
		return "SKIPPED"
	case RepairStatusFailed:
		return "FAILED"
	case RepairStatusRolledBack:
		return "ROLLEDBACK"
	default:
		return unknownStatusString
	}
}

// NewEngine creates a new repair engine with the given configuration.
func NewEngine(config EngineConfig) *Engine {
	return &Engine{
		validator: NewValidator(0), // Size will be set when processing
		txLog:     NewTransactionLog(),
		writer:    NewWriter(),
		modules:   make(map[string]RepairModule),
		config:    config,
	}
}

// RegisterModule registers a repair module with the engine.
// Modules are selected based on their CanRepair() method.
func (e *Engine) RegisterModule(module RepairModule) {
	e.modules[module.Name()] = module
}

// ExecuteRepairs applies repairs to the hive data based on diagnostics.
// This is the main entry point for the repair process.
//
// Process:
//  1. Validate all repairs can be safely applied
//  2. Apply each repair, logging to transaction log
//  3. Verify each repair was successful
//  4. On any failure, rollback all applied repairs
//
// Returns detailed results including success/failure counts and timing.
func (e *Engine) ExecuteRepairs(data []byte, diagnostics []Diagnostic) (*EngineResult, error) {
	result := &EngineResult{
		StartTime: time.Now(),
		Repairs:   make([]RepairDetail, 0, len(diagnostics)),
	}

	// Update validator with data size
	e.validator = NewValidator(uint64(len(data)))

	// Filter diagnostics based on configuration
	diagnostics = e.filterDiagnostics(diagnostics)

	// If dry run, simulate repairs without actually applying them
	if e.config.DryRun {
		return e.simulateRepairs(data, diagnostics, result)
	}

	// Make a copy of data for rollback comparison
	originalData := make([]byte, len(data))
	copy(originalData, data)

	// Clear transaction log
	e.txLog.Clear()

	// Process each diagnostic
	for _, d := range diagnostics {
		detail := e.processRepair(data, d)
		result.Repairs = append(result.Repairs, detail)

		switch detail.Status {
		case RepairStatusApplied:
			result.Applied++
		case RepairStatusSkipped:
			result.Skipped++
		case RepairStatusFailed:
			result.Failed++
			// On failure, rollback all applied repairs
			if rollbackCount, err := e.txLog.Rollback(data); err != nil {
				result.RollbackCount = rollbackCount
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(result.StartTime)
				result.TransactionLog = e.txLog
				return result, &EngineError{
					Operation: "rollback",
					Message: fmt.Sprintf(
						"repair failed and rollback encountered error after rolling back %d repairs",
						rollbackCount,
					),
					Cause: err,
				}
			} else {
				result.RollbackCount = rollbackCount
				// Mark all applied repairs as rolled back
				for i := range result.Repairs {
					if result.Repairs[i].Status == RepairStatusApplied {
						result.Repairs[i].Status = RepairStatusRolledBack
					}
				}
			}
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			result.TransactionLog = e.txLog
			return result, &EngineError{
				Operation: "repair",
				Message: fmt.Sprintf(
					"repair failed at diagnostic %d, all repairs rolled back",
					result.Applied+result.Skipped,
				),
				Cause: detail.Error,
			}
		case RepairStatusPending:
			// RepairStatusPending should not occur after processRepair
			// This is a no-op for exhaustive checking
		case RepairStatusRolledBack:
			// RepairStatusRolledBack should not occur during initial processing
			// This is a no-op for exhaustive checking
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TransactionLog = e.txLog

	return result, nil
}

// processRepair handles a single repair operation.
func (e *Engine) processRepair(data []byte, d Diagnostic) RepairDetail {
	startTime := time.Now()

	detail := RepairDetail{
		Diagnostic: d,
		Status:     RepairStatusPending,
	}

	// Check if diagnostic has repair action
	if d.Repair == nil {
		detail.Status = RepairStatusSkipped
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Find module that can handle this repair
	var module RepairModule
	for _, m := range e.modules {
		if m.CanRepair(d) {
			module = m
			detail.Module = m.Name()
			break
		}
	}

	if module == nil {
		detail.Status = RepairStatusSkipped
		detail.Error = errors.New("no module found to handle repair")
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Pre-repair validation
	if err := e.validator.ValidateRepairSafe(data, d); err != nil {
		detail.Status = RepairStatusFailed
		detail.Error = err
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Module-specific validation
	if err := module.Validate(data, d); err != nil {
		detail.Status = RepairStatusFailed
		detail.Error = err
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Capture before state for transaction log
	repairSize := e.validator.estimateRepairSize(d)
	if d.Offset+repairSize > uint64(len(data)) {
		repairSize = uint64(len(data)) - d.Offset
	}

	oldData := make([]byte, repairSize)
	copy(oldData, data[d.Offset:d.Offset+repairSize])

	// Apply repair
	if err := module.Apply(data, d); err != nil {
		detail.Status = RepairStatusFailed
		detail.Error = err
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Capture after state
	newData := make([]byte, repairSize)
	copy(newData, data[d.Offset:d.Offset+repairSize])

	// Log to transaction
	e.txLog.AddEntry(d.Offset, oldData, newData, d, module.Name())

	// Module-specific verification
	if err := module.Verify(data, d); err != nil {
		detail.Status = RepairStatusFailed
		detail.Error = err
		detail.Duration = time.Since(startTime)
		return detail
	}

	// Post-repair structure validation
	// For field-level repairs (RepairDefault, RepairReplace), the module's Verify()
	// is sufficient. Only do structure validation for structural repairs (Rebuild, etc.)
	if d.Repair.Type != RepairDefault && d.Repair.Type != RepairReplace {
		if err := e.validator.ValidateStructureIntegrity(data, d.Offset, d.Structure); err != nil {
			detail.Status = RepairStatusFailed
			detail.Error = err
			detail.Duration = time.Since(startTime)
			return detail
		}
	}

	// Mark as applied in transaction log
	if err := e.txLog.MarkApplied(); err != nil {
		detail.Status = RepairStatusFailed
		detail.Error = err
		detail.Duration = time.Since(startTime)
		return detail
	}

	detail.Status = RepairStatusApplied
	detail.Duration = time.Since(startTime)
	return detail
}

// simulateRepairs performs a dry-run simulation of repairs.
func (e *Engine) simulateRepairs(data []byte, diagnostics []Diagnostic, result *EngineResult) (*EngineResult, error) {
	// Make a copy of data for simulation
	simData := make([]byte, len(data))
	copy(simData, data)

	// Process each diagnostic in simulation mode
	for _, d := range diagnostics {
		startTime := time.Now()

		detail := RepairDetail{
			Diagnostic: d,
			Status:     RepairStatusPending,
		}

		// Check if diagnostic has repair action
		if d.Repair == nil {
			detail.Status = RepairStatusSkipped
			detail.Duration = time.Since(startTime)
			result.Repairs = append(result.Repairs, detail)
			result.Skipped++
			continue
		}

		// Find module
		var module RepairModule
		for _, m := range e.modules {
			if m.CanRepair(d) {
				module = m
				detail.Module = m.Name()
				break
			}
		}

		if module == nil {
			detail.Status = RepairStatusSkipped
			detail.Error = errors.New("no module found")
			detail.Duration = time.Since(startTime)
			result.Repairs = append(result.Repairs, detail)
			result.Skipped++
			continue
		}

		// Validate only (don't apply)
		if err := e.validator.ValidateRepairSafe(simData, d); err != nil {
			detail.Status = RepairStatusFailed
			detail.Error = err
			detail.Duration = time.Since(startTime)
			result.Repairs = append(result.Repairs, detail)
			result.Failed++
			continue
		}

		if err := module.Validate(simData, d); err != nil {
			detail.Status = RepairStatusFailed
			detail.Error = err
			detail.Duration = time.Since(startTime)
			result.Repairs = append(result.Repairs, detail)
			result.Failed++
			continue
		}

		// Simulate success
		detail.Status = RepairStatusApplied
		detail.Duration = time.Since(startTime)
		result.Repairs = append(result.Repairs, detail)
		result.Applied++
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

// filterDiagnostics filters diagnostics based on engine configuration.
func (e *Engine) filterDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	filtered := make([]Diagnostic, 0, len(diagnostics))

	for _, d := range diagnostics {
		// Skip if no repair action
		if d.Repair == nil {
			continue
		}

		// Skip if AutoOnly and not auto-apply
		if e.config.AutoOnly && !d.Repair.AutoApply {
			continue
		}

		// Skip if risk exceeds max
		if d.Repair.Risk > e.config.MaxRisk {
			continue
		}

		filtered = append(filtered, d)
	}

	return filtered
}

// ValidateNoSideEffects compares data before and after repairs to ensure
// only the intended regions were modified.
func (e *Engine) ValidateNoSideEffects(before, after []byte) error {
	if len(before) != len(after) {
		return &ValidationError{
			Phase:   "post",
			Module:  "engine",
			Message: "data size changed after repairs",
		}
	}

	// Check that only repaired regions differ
	for i := range before {
		if before[i] != after[i] {
			// Check if this byte is within a repair region
			inRepairRegion := false
			for _, entry := range e.txLog.entries {
				if uint64(i) >= entry.Offset && uint64(i) < entry.Offset+entry.Size {
					inRepairRegion = true
					break
				}
			}

			if !inRepairRegion {
				return &ValidationError{
					Phase:   "post",
					Module:  "engine",
					Offset:  uint64(i),
					Message: fmt.Sprintf("byte at offset 0x%X was modified but not part of any repair", i),
				}
			}
		}
	}

	return nil
}

// ExportLog returns a human-readable export of the transaction log.
func (e *Engine) ExportLog() string {
	return e.txLog.Export()
}

// GetWriter returns the engine's writer for external backup operations.
func (e *Engine) GetWriter() *Writer {
	return e.writer
}
