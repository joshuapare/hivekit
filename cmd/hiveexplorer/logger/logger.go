package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// L is the global logger instance. It's initialized to discard all output by default.
// Call Init() to enable logging to a file.
var L *slog.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

const (
	logPrefix     = "hiveexplorer-"
	logSuffix     = ".log"
	retentionDays = 30
)

// Options configures the logger initialization.
type Options struct {
	Enabled bool       // If false, all logging is discarded
	LogDir  string     // Directory for log files. Default: ~/.hiveexplorer/logs
	Level   slog.Level // Minimum log level. Default: LevelInfo when enabled
}

// Init configures logging. Call from main() before any log calls.
// If opts.Enabled is false, all log output is discarded.
func Init(opts Options) error {
	if !opts.Enabled {
		L = slog.New(slog.NewTextHandler(io.Discard, nil))
		return nil
	}

	logDir := opts.LogDir
	if logDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		logDir = filepath.Join(home, ".hiveexplorer", "logs")
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// Clean up old logs (best-effort, ignore errors)
	cleanOldLogs(logDir)

	filename := filepath.Join(logDir, logPrefix+time.Now().Format("2006-01-02")+logSuffix)

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	level := opts.Level
	if level == 0 {
		level = slog.LevelInfo
	}

	L = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level}))
	return nil
}

// cleanOldLogs removes log files older than retentionDays.
func cleanOldLogs(logDir string) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, logPrefix) || !strings.HasSuffix(name, logSuffix) {
			continue
		}

		// Parse date from filename: hiveexplorer-2024-01-05.log
		dateStr := strings.TrimPrefix(strings.TrimSuffix(name, logSuffix), logPrefix)
		logDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		if logDate.Before(cutoff) {
			os.Remove(filepath.Join(logDir, name))
		}
	}
}

// Debug logs a debug message with optional key-value pairs.
func Debug(msg string, args ...any) { L.Debug(msg, args...) }

// Info logs an info message with optional key-value pairs.
func Info(msg string, args ...any) { L.Info(msg, args...) }

// Warn logs a warning message with optional key-value pairs.
func Warn(msg string, args ...any) { L.Warn(msg, args...) }

// Error logs an error message with optional key-value pairs.
func Error(msg string, args ...any) { L.Error(msg, args...) }
