package hive

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// bufWriter implements Writer for in-memory buffers.
type bufWriter struct {
	buf *bytes.Buffer
}

func (w *bufWriter) WriteHive(data []byte) error {
	_, err := w.buf.Write(data)
	return err
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return dstFile.Close()
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// applyOperation applies a single edit operation to a transaction.
func applyOperation(tx Tx, op EditOp) error {
	switch op := op.(type) {
	case OpCreateKey:
		return tx.CreateKey(op.Path, CreateKeyOptions{CreateParents: true})

	case OpSetValue:
		return tx.SetValue(op.Path, op.Name, op.Type, op.Data)

	case OpDeleteKey:
		err := tx.DeleteKey(op.Path, DeleteKeyOptions{Recursive: op.Recursive})
		// Windows regedit silently ignores deleting non-existent keys
		if err != nil && strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err

	case OpDeleteValue:
		err := tx.DeleteValue(op.Path, op.Name)
		// Windows regedit silently ignores deleting non-existent values
		if err != nil && strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err

	default:
		return fmt.Errorf("unknown operation type: %T", op)
	}
}
