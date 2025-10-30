package hive

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/regtext"
)

// ParseRegFile parses a .reg file and returns the operations without applying them.
// This is useful for testing, validation, and inspecting changes before merging.
//
// Example:
//
//	ops, err := hive.ParseRegFile("changes.reg", hive.RegParseOptions{
//	    AutoPrefix: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, op := range ops {
//	    fmt.Printf("Operation: %T\n", op)
//	}
func ParseRegFile(regPath string, opts RegParseOptions) ([]EditOp, error) {
	// Validate input
	if !fileExists(regPath) {
		return nil, fmt.Errorf(".reg file not found: %s", regPath)
	}

	// Read .reg file
	regData, err := os.ReadFile(regPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .reg file %s: %w", regPath, err)
	}

	return ParseRegBytes(regData, opts)
}

// ParseRegString parses .reg content from a string and returns the operations.
//
// Example:
//
//	regContent := `Windows Registry Editor Version 5.00
//
//	[HKEY_LOCAL_MACHINE\Software\MyApp]
//	"Version"="1.0"
//	`
//	ops, err := hive.ParseRegString(regContent, hive.RegParseOptions{})
func ParseRegString(regContent string, opts RegParseOptions) ([]EditOp, error) {
	return ParseRegBytes([]byte(regContent), opts)
}

// ParseRegBytes parses .reg content from bytes and returns the operations.
// This is the core parsing function used by ParseRegFile and ParseRegString.
//
// Example:
//
//	regData, _ := os.ReadFile("changes.reg")
//	ops, err := hive.ParseRegBytes(regData, hive.RegParseOptions{
//	    Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
//	})
func ParseRegBytes(regData []byte, opts RegParseOptions) ([]EditOp, error) {
	// Use the codec to parse
	codec := regtext.NewCodec()
	ops, err := codec.ParseReg(regData, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .reg data: %w", err)
	}

	return ops, nil
}
