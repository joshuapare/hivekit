package repair

import "fmt"

// RepairError represents an error that occurred during repair operations.
type RepairError struct {
	Module  string // Module that encountered the error
	Offset  uint64 // Offset where the error occurred
	Message string // Human-readable error message
	Cause   error  // Underlying error, if any
}

// Error implements the error interface.
func (e *RepairError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s repair error at offset 0x%X: %s: %v", e.Module, e.Offset, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s repair error at offset 0x%X: %s", e.Module, e.Offset, e.Message)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *RepairError) Unwrap() error {
	return e.Cause
}

// ValidationError represents a validation failure before or after repair.
type ValidationError struct {
	Phase   string // "pre" or "post"
	Module  string
	Offset  uint64
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s-repair validation failed (%s) at offset 0x%X: %s: %v", e.Phase, e.Module, e.Offset, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s-repair validation failed (%s) at offset 0x%X: %s", e.Phase, e.Module, e.Offset, e.Message)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// TransactionError represents an error during transaction log operations.
type TransactionError struct {
	Operation string // "add", "rollback", "commit", etc.
	Message   string
	Cause     error
}

// Error implements the error interface.
func (e *TransactionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("transaction %s failed: %s: %v", e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("transaction %s failed: %s", e.Operation, e.Message)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *TransactionError) Unwrap() error {
	return e.Cause
}

// EngineError represents a high-level engine error.
type EngineError struct {
	Operation string // "prepare", "execute", "verify", "commit", etc.
	Message   string
	Cause     error
}

// Error implements the error interface.
func (e *EngineError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("repair engine %s failed: %s: %v", e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("repair engine %s failed: %s", e.Operation, e.Message)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *EngineError) Unwrap() error {
	return e.Cause
}
