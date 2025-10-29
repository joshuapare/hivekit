package reader

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/repair"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ApplyRepairs applies repair actions to a hive file using the repair engine.
// This is the internal implementation called by types.DiagnosticReport.ApplyRepairs().
func ApplyRepairs(hivePath string, diagnostics []types.Diagnostic, opts types.RepairOptions) (*types.RepairResult, error) {
	// Read hive data
	data, err := os.ReadFile(hivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read hive: %w", err)
	}

	// Create repair engine with configuration
	engine := repair.NewEngine(repair.EngineConfig{
		DryRun:   opts.DryRun,
		AutoOnly: opts.AutoOnly,
		MaxRisk:  opts.MaxRisk,
		Verbose:  opts.Verbose,
	})

	// Register all repair modules
	engine.RegisterModule(repair.NewNKModule())
	engine.RegisterModule(repair.NewREGFModule())
	engine.RegisterModule(repair.NewHBINModule())
	engine.RegisterModule(repair.NewVKModule())

	// Execute repairs
	result, err := engine.ExecuteRepairs(data, diagnostics)
	if err != nil {
		return nil, err
	}

	// Convert internal result to public result
	publicResult := &types.RepairResult{
		Applied:     result.Applied,
		Skipped:     result.Skipped,
		Failed:      result.Failed,
		DryRun:      opts.DryRun,
		Duration:    result.Duration,
		Diagnostics: make([]types.RepairDiagnostic, 0, len(result.Repairs)),
	}

	for _, detail := range result.Repairs {
		errMsg := ""
		if detail.Error != nil {
			errMsg = detail.Error.Error()
		}

		publicResult.Diagnostics = append(publicResult.Diagnostics, types.RepairDiagnostic{
			Offset:      detail.Diagnostic.Offset,
			Description: detail.Diagnostic.Repair.Description,
			Applied:     detail.Status == repair.RepairStatusApplied,
			Error:       errMsg,
		})
	}

	// If not dry-run and repairs were applied, write the repaired data back
	if !opts.DryRun && result.Applied > 0 {
		writer := engine.GetWriter()

		// Create backup unless explicitly disabled
		if !opts.NoBackup {
			backupSuffix := opts.BackupSuffix
			if backupSuffix == "" {
				backupSuffix = ".backup"
			}

			backupPath, err := writer.CreateBackup(hivePath, backupSuffix)
			if err != nil {
				return nil, fmt.Errorf("failed to create backup: %w", err)
			}
			publicResult.BackupPath = backupPath
		}

		// Use atomic write to apply repairs
		if err := writer.WriteAtomic(hivePath, data); err != nil {
			return nil, fmt.Errorf("failed to write repaired hive: %w", err)
		}
	}

	return publicResult, nil
}
