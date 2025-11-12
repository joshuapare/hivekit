package builder

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/internal/format"
)

// Builder provides a high-performance, path-based API for building Windows
// Registry hive files programmatically.
//
// The builder uses progressive writes to maintain constant memory usage even
// for arbitrarily large hives. Operations are automatically batched and flushed
// to disk based on AutoFlushThreshold.
//
// Example usage:
//
//	b, err := builder.New("/tmp/app.hive", nil)
//	if err != nil {
//	    return err
//	}
//	defer b.Close()
//
//	b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
//	b.SetDWORD([]string{"Software", "MyApp"}, "Enabled", 1)
//
//	return b.Commit()
//
// Thread safety: Builder instances are NOT thread-safe. Use one builder per goroutine.
type Builder struct {
	session *merge.Session
	plan    *merge.Plan
	opCount int
	opts    *Options
	closed  bool
}

// New creates a new builder for the specified hive file.
//
// If opts.CreateIfNotExists is true (default) and the file doesn't exist,
// a minimal valid hive is created with an empty root key.
//
// If the file exists, it's opened for modification.
//
// Parameters:
//   - path: Absolute path to hive file
//   - opts: Configuration options (nil uses DefaultOptions())
//
// Returns:
//   - *Builder: Ready-to-use builder instance
//   - error: If file can't be created/opened or is invalid
func New(path string, opts *Options) (*Builder, error) {
	// Use defaults if no options provided
	if opts == nil {
		opts = DefaultOptions()
	}

	// Check if file exists
	_, err := os.Stat(path)
	fileExists := err == nil

	// Create new hive if needed
	if !fileExists {
		if !opts.CreateIfNotExists {
			return nil, fmt.Errorf("hive file does not exist: %s", path)
		}

		if _, err := createMinimalHive(path, opts.HiveVersion); err != nil {
			return nil, fmt.Errorf("create minimal hive: %w", err)
		}
	}

	// Open hive
	h, err := hive.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open hive: %w", err)
	}

	// Pre-allocate HBINs if requested
	// Get allocator access via session (we'll create session below, but need to grow first)
	// Actually, we need to do this after creating the session. Let's defer this.
	_ = opts.PreallocPages // TODO: implement preallocation

	// Create merge options from builder options
	mergeOpts := merge.Options{
		Strategy: opts.Strategy.toMergeStrategy(),
	}

	// Create session
	session, err := merge.NewSession(h, mergeOpts)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Pre-allocate HBINs if requested
	// Access allocator from session (via reflection or direct access if exposed)
	// For now, we'll skip this as it requires exposing the allocator
	// TODO: Add GrowByPages method to session or expose allocator
	_ = opts.PreallocPages // TODO: implement preallocation

	// Create builder
	b := &Builder{
		session: session,
		plan:    merge.NewPlan(),
		opCount: 0,
		opts:    opts,
		closed:  false,
	}

	return b, nil
}

// SetValue sets a registry value with explicit type and raw data.
//
// This is the generic setter used by all type-specific helpers. Use this when
// you have pre-encoded data or need to set a custom value type.
//
// Parameters:
//   - path: Full path to parent key (e.g., []string{"Software", "MyApp"})
//   - name: Value name (empty string "" for default value)
//   - typ: Registry value type (REG_SZ, REG_DWORD, etc.)
//   - data: Pre-encoded value data
//
// Returns:
//   - error: If path is invalid or operation fails
func (b *Builder) SetValue(path []string, name string, typ uint32, data []byte) error {
	if b.closed {
		return errors.New("builder is closed")
	}

	if len(path) == 0 {
		return errors.New("path cannot be empty")
	}

	// Create operation
	op := merge.Op{
		Type:      merge.OpSetValue,
		KeyPath:   path,
		ValueName: name,
		ValueType: typ,
		Data:      data,
	}

	return b.addOp(op)
}

// SetString sets a REG_SZ value (null-terminated UTF-16LE string).
//
// Example:
//
//	b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
func (b *Builder) SetString(path []string, name string, value string) error {
	data := encodeString(value)
	return b.SetValue(path, name, format.REGSZ, data)
}

// SetExpandString sets a REG_EXPAND_SZ value (string with environment variables).
//
// Example:
//
//	b.SetExpandString([]string{"Environment"}, "Path", "%SystemRoot%\\System32")
func (b *Builder) SetExpandString(path []string, name string, value string) error {
	data := encodeString(value)
	return b.SetValue(path, name, format.REGExpandSZ, data)
}

// SetBinary sets a REG_BINARY value (raw bytes).
//
// Example:
//
//	b.SetBinary([]string{"Security"}, "Key", []byte{0x01, 0x02, 0x03})
func (b *Builder) SetBinary(path []string, name string, data []byte) error {
	return b.SetValue(path, name, format.REGBinary, data)
}

// SetDWORD sets a REG_DWORD value (32-bit little-endian integer).
//
// Example:
//
//	b.SetDWORD([]string{"Software", "MyApp"}, "Timeout", 30)
func (b *Builder) SetDWORD(path []string, name string, value uint32) error {
	data := encodeDWORD(value)
	return b.SetValue(path, name, format.REGDWORD, data)
}

// SetQWORD sets a REG_QWORD value (64-bit little-endian integer).
//
// Example:
//
//	b.SetQWORD([]string{"Stats"}, "Counter", 1234567890123)
func (b *Builder) SetQWORD(path []string, name string, value uint64) error {
	data := encodeQWORD(value)
	return b.SetValue(path, name, format.REGQWORD, data)
}

// SetMultiString sets a REG_MULTI_SZ value (array of null-terminated strings).
//
// Example:
//
//	b.SetMultiString([]string{"Paths"}, "SearchDirs", []string{
//	    "C:\\Program Files",
//	    "C:\\Windows\\System32",
//	})
func (b *Builder) SetMultiString(path []string, name string, values []string) error {
	data := encodeMultiString(values)
	return b.SetValue(path, name, format.REGMultiSZ, data)
}

// SetDWORDBigEndian sets a REG_DWORD_BIG_ENDIAN value (32-bit big-endian integer).
//
// This type is rare and mainly used for compatibility.
//
// Example:
//
//	b.SetDWORDBigEndian([]string{"Network"}, "Magic", 0x12345678)
func (b *Builder) SetDWORDBigEndian(path []string, name string, value uint32) error {
	data := encodeDWORDBigEndian(value)
	return b.SetValue(path, name, format.REGDWORDBigEndian, data)
}

// EnsureKey creates a key at the specified path if it doesn't already exist.
//
// This operation is idempotent - it succeeds if the key already exists.
// All parent keys in the path are created automatically if needed.
//
// Parameters:
//   - path: Full path to key to ensure exists
//
// Returns:
//   - error: If operation fails
//
// Example:
//
//	// Ensure key exists (creates if needed, no-op if exists)
//	b.EnsureKey([]string{"Software", "MyApp", "Settings"})
func (b *Builder) EnsureKey(path []string) error {
	if b.closed {
		return errors.New("builder is closed")
	}

	if len(path) == 0 {
		return errors.New("path cannot be empty")
	}

	// Create operation
	op := merge.Op{
		Type:    merge.OpEnsureKey,
		KeyPath: path,
	}

	return b.addOp(op)
}

// DeleteKey deletes a key and all its subkeys at the specified path.
//
// Note: This operation always deletes recursively (including all subkeys).
//
// Parameters:
//   - path: Full path to key to delete
//
// Returns:
//   - error: If operation fails
//
// Example:
//
//	// Delete key with all subkeys
//	b.DeleteKey([]string{"Software", "OldApp"})
func (b *Builder) DeleteKey(path []string) error {
	if b.closed {
		return errors.New("builder is closed")
	}

	if len(path) == 0 {
		return errors.New("path cannot be empty")
	}

	// Create operation
	op := merge.Op{
		Type:    merge.OpDeleteKey,
		KeyPath: path,
	}

	return b.addOp(op)
}

// DeleteValue deletes a value from the specified key.
//
// This operation is idempotent - it succeeds even if the value doesn't exist.
//
// Parameters:
//   - path: Full path to parent key
//   - name: Value name to delete
//
// Example:
//
//	b.DeleteValue([]string{"Software", "MyApp"}, "Deprecated")
func (b *Builder) DeleteValue(path []string, name string) error {
	if b.closed {
		return errors.New("builder is closed")
	}

	if len(path) == 0 {
		return errors.New("path cannot be empty")
	}

	// Create operation
	op := merge.Op{
		Type:      merge.OpDeleteValue,
		KeyPath:   path,
		ValueName: name,
	}

	return b.addOp(op)
}

// Commit flushes all pending operations and commits changes to disk.
//
// Process:
//  1. Flush any pending operations (final progressive write)
//  2. Update transaction sequences (PrimarySeq = SecondarySeq)
//  3. Update timestamp and checksum
//  4. Sync all dirty pages to disk (msync + fsync)
//  5. Close session and hive
//
// After Commit(), the builder is closed and cannot be reused.
//
// Returns:
//   - error: If flush or sync fails
func (b *Builder) Commit() error {
	if b.closed {
		return errors.New("builder is already closed")
	}

	// Flush any pending operations
	if err := b.flush(); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}

	// CRITICAL: Flush deferred subkeys one more time after the final flush.
	// Even if opCount was 0, there may be deferred parent-child relationships
	// accumulated from the previous flush cycle that need to be written.
	_, flushErr := b.session.FlushDeferredSubkeys()
	if flushErr != nil {
		return fmt.Errorf("final deferred flush: %w", flushErr)
	}

	// Close session (includes final commit)
	if err := b.session.Close(); err != nil {
		return fmt.Errorf("close session: %w", err)
	}

	b.closed = true
	return nil
}

// Rollback closes the builder without committing changes (best effort).
//
// Note: Due to progressive writes, some changes may already be on disk.
// This is not a true rollback - it just closes without final commit.
//
// After Rollback(), the builder is closed and cannot be reused.
//
// Returns:
//   - error: If cleanup fails
func (b *Builder) Rollback() error {
	if b.closed {
		return nil // Already closed
	}

	// Close session without commit
	// Note: Progressive flushes may have already written some data
	if err := b.session.Close(); err != nil {
		return fmt.Errorf("close session: %w", err)
	}

	b.closed = true
	return nil
}

// Close is an alias for Rollback(). It closes the builder without committing.
//
// This is useful for defer cleanup:
//
//	b, err := builder.New("/tmp/test.hive", nil)
//	if err != nil {
//	    return err
//	}
//	defer b.Close()  // Cleanup if we don't reach Commit()
//
//	// ... operations ...
//
//	return b.Commit()  // Normal path
func (b *Builder) Close() error {
	return b.Rollback()
}

// addOp adds an operation to the current plan and triggers progressive flush if threshold reached.
func (b *Builder) addOp(op merge.Op) error {
	// Add to plan
	b.plan.Ops = append(b.plan.Ops, op)
	b.opCount++

	// Check if we should flush
	if b.opts.AutoFlushThreshold > 0 && b.opCount >= b.opts.AutoFlushThreshold {
		if err := b.flush(); err != nil {
			return fmt.Errorf("progressive flush: %w", err)
		}
	}

	return nil
}

// flush applies the current plan and resets for the next batch.
//
// IMPORTANT: This MUST flush deferred subkey lists before committing to ensure
// that parent-child relationships are written to disk. Otherwise, when children
// are added to the same parent across multiple flush cycles, earlier children
// will be lost.
func (b *Builder) flush() error {
	// Nothing to flush
	if b.opCount == 0 {
		return nil
	}

	// CRITICAL: Flush deferred subkey lists before applying the plan.
	// This writes all accumulated parent-child relationships to disk,
	// ensuring they're visible to subsequent flushes.
	_, flushErr := b.session.FlushDeferredSubkeys()
	if flushErr != nil {
		return fmt.Errorf("flush deferred subkeys: %w", flushErr)
	}

	// Apply plan with transaction
	_, err := b.session.ApplyWithTx(b.plan)
	if err != nil {
		return fmt.Errorf("apply plan: %w", err)
	}

	// Reset plan and counter for next batch
	b.plan = merge.NewPlan()
	b.opCount = 0

	return nil
}

// splitPath converts a registry path string to a path array, respecting the
// StripHiveRootPrefixes option.
//
// When opts.StripHiveRootPrefixes is true (default), paths like
// "HKLM\Software\MyApp" become ["Software", "MyApp"].
//
// When false, the full path is preserved as-is: ["HKLM", "Software", "MyApp"].
func (b *Builder) splitPath(path string) []string {
	// Strip hive root prefixes based on option
	path = stripHiveRoot(path, b.opts.StripHiveRootPrefixes)

	// Handle empty path
	if path == "" {
		return []string{}
	}

	// Split on backslashes
	segments := strings.Split(path, "\\")

	// Filter out empty segments
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}

	return result
}
