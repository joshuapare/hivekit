package hivexval

// Options controls validator behavior.
type Options struct {
	// UseBindings enables validation with Go bindings (bindings/wrapper.go).
	// This is the primary validation method - fast and full-featured.
	UseBindings bool

	// UseHivexsh enables validation with hivexsh command-line tool.
	// This is the authoritative reference implementation.
	UseHivexsh bool

	// UseReader enables validation with gohivex reader (internal/reader).
	// Useful for comparing reader implementation against bindings.
	UseReader bool

	// CompareAll compares results between all enabled backends.
	// Requires at least two backends to be enabled.
	CompareAll bool

	// SkipIfHivexshUnavailable skips validation instead of failing
	// if hivexsh is not available on the system.
	SkipIfHivexshUnavailable bool
}

// DefaultOptions returns recommended default options.
// Uses gohivex reader as primary validation method (pure Go, no CGO).
func DefaultOptions() *Options {
	return &Options{
		UseBindings:              false,
		UseHivexsh:               false,
		UseReader:                true,
		CompareAll:               false,
		SkipIfHivexshUnavailable: true,
	}
}

// ValidationResult holds comprehensive validation results.
type ValidationResult struct {
	// StructureValid indicates the hive structure is valid.
	StructureValid bool

	// HivexshPassed indicates hivexsh validation passed (if enabled).
	HivexshPassed bool

	// KeyCount is the total number of keys in the hive.
	KeyCount int

	// ValueCount is the total number of values in the hive.
	ValueCount int

	// Errors contains validation errors.
	Errors []string

	// Warnings contains validation warnings.
	Warnings []string

	// ComparisonResult contains cross-backend comparison (if enabled).
	ComparisonResult *ComparisonResult
}

// ComparisonResult holds cross-validation comparison results.
type ComparisonResult struct {
	// Match indicates whether all backends agree.
	Match bool

	// Mismatches contains differences found between backends.
	Mismatches []Mismatch

	// NodesCompared is the number of keys compared.
	NodesCompared int

	// ValuesCompared is the number of values compared.
	ValuesCompared int
}

// Mismatch describes a difference between validator backends.
type Mismatch struct {
	// Category describes the type of mismatch.
	// Examples: "key_count", "value_type", "value_data", "key_name"
	Category string

	// Path is the registry path where the mismatch occurred.
	Path string

	// Message is a human-readable description.
	Message string

	// Expected is the expected value (from primary backend).
	Expected interface{}

	// Actual is the actual value (from secondary backend).
	Actual interface{}
}

// Backend identifies which validation backend is being used.
type Backend int

const (
	// BackendNone indicates no backend is active.
	BackendNone Backend = iota

	// BackendBindings uses Go bindings (bindings/wrapper.go).
	BackendBindings

	// BackendReader uses gohivex reader (internal/reader).
	BackendReader

	// BackendHivexsh uses hivexsh command-line tool.
	BackendHivexsh
)

// String returns the backend name.
func (b Backend) String() string {
	switch b {
	case BackendBindings:
		return "bindings"
	case BackendReader:
		return "reader"
	case BackendHivexsh:
		return "hivexsh"
	case BackendNone:
		return "none"
	default:
		return "none"
	}
}
