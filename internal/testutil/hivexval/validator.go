package hivexval

import (
	"errors"
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Validator provides hivex validation utilities for testing.
type Validator struct {
	// Path to the hive file (empty if created from bytes)
	path string

	// Active backends
	hive   *bindings.Hive // Go bindings backend
	reader types.Reader   // Reader backend

	// Configuration
	opts *Options

	// Primary backend being used
	backend Backend

	// Data buffer (if created from bytes)
	data []byte
}

// New creates a validator for a hive file.
//
// If opts is nil, DefaultOptions() is used.
//
// Example:
//
//	v, err := hivexval.New("/path/to/hive", nil)
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer v.Close()
func New(path string, opts *Options) (*Validator, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Check file exists
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("hive file not found: %w", err)
	}

	v := &Validator{
		path: path,
		opts: opts,
	}

	// Open with bindings if enabled
	if opts.UseBindings {
		hive, err := bindings.Open(path, 0)
		if err != nil {
			return nil, fmt.Errorf("open with bindings: %w", err)
		}
		v.hive = hive
		v.backend = BackendBindings
	}

	// Open with reader if enabled
	if opts.UseReader {
		r, err := reader.Open(path, types.OpenOptions{})
		if err != nil {
			// Clean up bindings if already opened
			if v.hive != nil {
				v.hive.Close()
			}
			return nil, fmt.Errorf("open with reader: %w", err)
		}
		v.reader = r

		// Use reader as primary if bindings not enabled
		if v.backend == BackendNone {
			v.backend = BackendReader
		}
	}

	// Validate at least one backend is enabled
	if v.backend == BackendNone && !opts.UseHivexsh {
		if v.hive != nil {
			v.hive.Close()
		}
		if v.reader != nil {
			v.reader.Close()
		}
		return nil, errors.New("at least one backend must be enabled")
	}

	// If only hivexsh is enabled, set backend
	if v.backend == BackendNone && opts.UseHivexsh {
		v.backend = BackendHivexsh
	}

	return v, nil
}

// NewBytes creates a validator from a byte buffer.
//
// If opts is nil, DefaultOptions() is used.
//
// Note: Hivexsh backend requires writing to a temporary file.
//
// Example:
//
//	data, _ := os.ReadFile("test.hive")
//	v, err := hivexval.NewBytes(data, nil)
func NewBytes(data []byte, opts *Options) (*Validator, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	if len(data) == 0 {
		return nil, errors.New("empty data buffer")
	}

	v := &Validator{
		data: data,
		opts: opts,
	}

	// Open with bindings if enabled
	// Note: bindings doesn't support opening from bytes directly
	// We need to write to a temp file
	if opts.UseBindings {
		tmpFile, err := os.CreateTemp("", "hivexval-*.hive")
		if err != nil {
			return nil, fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return nil, fmt.Errorf("write temp file: %w", err)
		}
		tmpFile.Close()

		hive, err := bindings.Open(tmpPath, 0)
		if err != nil {
			os.Remove(tmpPath)
			return nil, fmt.Errorf("open with bindings: %w", err)
		}

		v.hive = hive
		v.path = tmpPath // Will be cleaned up on Close()
		v.backend = BackendBindings
	}

	// Open with reader if enabled
	if opts.UseReader {
		r, err := reader.OpenBytes(data, types.OpenOptions{})
		if err != nil {
			// Clean up bindings if already opened
			if v.hive != nil {
				v.hive.Close()
				if v.path != "" {
					os.Remove(v.path)
				}
			}
			return nil, fmt.Errorf("open with reader: %w", err)
		}
		v.reader = r

		// Use reader as primary if bindings not enabled
		if v.backend == BackendNone {
			v.backend = BackendReader
		}
	}

	// Validate at least one backend is enabled
	if v.backend == BackendNone && !opts.UseHivexsh {
		return nil, errors.New("at least one backend must be enabled")
	}

	// If only hivexsh is enabled, we'll need to write temp file on demand
	if v.backend == BackendNone && opts.UseHivexsh {
		v.backend = BackendHivexsh
	}

	return v, nil
}

// Must panics on error (for tests where failure is fatal).
//
// Example:
//
//	v := hivexval.Must(hivexval.New(path, nil))
//	defer v.Close()
func Must(v *Validator, err error) *Validator {
	if err != nil {
		panic(err)
	}
	return v
}

// Close releases all resources.
//
// Always call Close() when done with a validator.
//
// Example:
//
//	v, _ := hivexval.New(path, nil)
//	defer v.Close()
func (v *Validator) Close() error {
	var errs []error

	// Close bindings
	if v.hive != nil {
		if err := v.hive.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close bindings: %w", err))
		}
		v.hive = nil
	}

	// Close reader
	if v.reader != nil {
		if err := v.reader.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close reader: %w", err))
		}
		v.reader = nil
	}

	// Clean up temp file if created from bytes
	if v.data != nil && v.path != "" {
		if err := os.Remove(v.path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove temp file: %w", err))
		}
	}

	// Return first error
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Backend returns which backend is currently active.
func (v *Validator) Backend() Backend {
	return v.backend
}

// ensurePath ensures we have a file path for hivexsh.
// Creates a temp file if needed (for byte-based validators).
func (v *Validator) ensurePath() (string, error) {
	if v.path != "" {
		return v.path, nil
	}

	// Create temp file from data
	if v.data == nil {
		return "", errors.New("no path or data available")
	}

	tmpFile, err := os.CreateTemp("", "hivexval-*.hive")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(v.data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	v.path = tmpPath
	return tmpPath, nil
}
