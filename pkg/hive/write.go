package hive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// SetValue sets a registry value at the specified path.
// The value type and data are specified as parameters.
//
// Example:
//
//	err := ops.SetValue("system.hive", "Software\\MyApp", "Version", REG_SZ, []byte("1.0.0"), nil)
func SetValue(hivePath string, keyPath string, valueName string, valueType RegType, data []byte, opts *OperationOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Apply defaults
	if opts == nil {
		opts = &OperationOptions{}
	}
	if opts.Limits == nil {
		limits := DefaultLimits()
		opts.Limits = &limits
	}

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Start transaction
	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(*opts.Limits)

	// Create key if requested
	if opts.CreateKey {
		if err := tx.CreateKey(keyPath, CreateKeyOptions{CreateParents: true}); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create key: %w", err)
		}
	}

	// Set value
	if err := tx.SetValue(keyPath, valueName, valueType, data); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to set value: %w", err)
	}

	// Dry run? Don't commit
	if opts.DryRun {
		return tx.Rollback()
	}

	// Commit to buffer
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: opts.Defragment}
	if err := tx.Commit(&bufWriter{buf}, writeOpts); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Write to file atomically
	tempPath := hivePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, hivePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace hive: %w", err)
	}

	return nil
}

// SetStringValue is a convenience function for setting string values (REG_SZ).
func SetStringValue(hivePath string, keyPath string, valueName string, value string, opts *OperationOptions) error {
	// Convert string to UTF-16LE with null terminator
	data := encodeUTF16LEString(value)
	return SetValue(hivePath, keyPath, valueName, REG_SZ, data, opts)
}

// SetDWORDValue is a convenience function for setting DWORD values.
func SetDWORDValue(hivePath string, keyPath string, valueName string, value uint32, opts *OperationOptions) error {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, value)
	return SetValue(hivePath, keyPath, valueName, REG_DWORD, data, opts)
}

// SetQWORDValue is a convenience function for setting QWORD values.
func SetQWORDValue(hivePath string, keyPath string, valueName string, value uint64, opts *OperationOptions) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value)
	return SetValue(hivePath, keyPath, valueName, REG_QWORD, data, opts)
}

// DeleteKey deletes a registry key from the 
//
// Example:
//
//	err := ops.DeleteKey("system.hive", "Software\\OldApp", true, nil)
func DeleteKey(hivePath string, keyPath string, recursive bool, opts *OperationOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Apply defaults
	if opts == nil {
		opts = &OperationOptions{}
	}
	if opts.Limits == nil {
		limits := DefaultLimits()
		opts.Limits = &limits
	}

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Start transaction
	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(*opts.Limits)

	// Delete key
	if err := tx.DeleteKey(keyPath, DeleteKeyOptions{Recursive: recursive}); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete key: %w", err)
	}

	// Dry run? Don't commit
	if opts.DryRun {
		return tx.Rollback()
	}

	// Commit to buffer
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: opts.Defragment}
	if err := tx.Commit(&bufWriter{buf}, writeOpts); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Write to file atomically
	tempPath := hivePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, hivePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace hive: %w", err)
	}

	return nil
}

// DeleteValue deletes a value from a registry key.
//
// Example:
//
//	err := ops.DeleteValue("system.hive", "Software\\MyApp", "OldSetting", nil)
func DeleteValue(hivePath string, keyPath string, valueName string, opts *OperationOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Apply defaults
	if opts == nil {
		opts = &OperationOptions{}
	}
	if opts.Limits == nil {
		limits := DefaultLimits()
		opts.Limits = &limits
	}

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Start transaction
	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(*opts.Limits)

	// Delete value
	if err := tx.DeleteValue(keyPath, valueName); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete value: %w", err)
	}

	// Dry run? Don't commit
	if opts.DryRun {
		return tx.Rollback()
	}

	// Commit to buffer
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: opts.Defragment}
	if err := tx.Commit(&bufWriter{buf}, writeOpts); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Write to file atomically
	tempPath := hivePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, hivePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace hive: %w", err)
	}

	return nil
}

// encodeUTF16LEString encodes a UTF-8 string to UTF-16LE with null terminator
func encodeUTF16LEString(s string) []byte {
	// Convert to UTF-16
	runes := []rune(s)
	buf := make([]byte, (len(runes)+1)*2) // +1 for null terminator

	for i, r := range runes {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(r))
	}
	// Null terminator
	binary.LittleEndian.PutUint16(buf[len(runes)*2:], 0)

	return buf
}

// ParseValueString parses a value string into the appropriate type and data.
// For use with CLI commands where users provide values as strings.
func ParseValueString(valueStr string, valueType string) (RegType, []byte, error) {
	switch strings.ToUpper(valueType) {
	case "SZ", "REG_SZ":
		return REG_SZ, encodeUTF16LEString(valueStr), nil

	case "EXPAND_SZ", "REG_EXPAND_SZ":
		return REG_EXPAND_SZ, encodeUTF16LEString(valueStr), nil

	case "DWORD", "REG_DWORD":
		val, err := strconv.ParseUint(valueStr, 0, 32)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid DWORD value: %w", err)
		}
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(val))
		return REG_DWORD, data, nil

	case "QWORD", "REG_QWORD":
		val, err := strconv.ParseUint(valueStr, 0, 64)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid QWORD value: %w", err)
		}
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, val)
		return REG_QWORD, data, nil

	case "BINARY", "REG_BINARY":
		// Parse hex string
		data, err := parseHexString(valueStr)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid BINARY value: %w", err)
		}
		return REG_BINARY, data, nil

	default:
		return 0, nil, fmt.Errorf("unsupported value type: %s", valueType)
	}
}

// parseHexString parses a hex string (with or without 0x prefix, with or without spaces)
func parseHexString(s string) ([]byte, error) {
	// Remove common separators and prefix
	s = strings.TrimPrefix(s, "0x")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, ":", "")

	// Parse pairs of hex digits
	if len(s)%2 != 0 {
		return nil, errors.New("hex string must have even number of characters")
	}

	data := make([]byte, len(s)/2)
	for i := 0; i < len(data); i++ {
		val, err := strconv.ParseUint(s[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid hex at position %d: %w", i*2, err)
		}
		data[i] = byte(val)
	}

	return data, nil
}
