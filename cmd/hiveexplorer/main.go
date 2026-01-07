package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
)

func main() {
	// Parse flags first (before positional args)
	args := os.Args[1:]
	debugMode := false

	// Extract --debug/-d flag
	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--debug" || arg == "-d" {
			debugMode = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Initialize logger (must be before any logging calls)
	if err := logger.Init(logger.Options{
		Enabled: debugMode,
		Level:   slog.LevelDebug,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to init logging: %v\n", err)
	}

	if len(filteredArgs) < 1 {
		printUsage()
		os.Exit(1)
	}

	if filteredArgs[0] == "--help" || filteredArgs[0] == "-h" {
		printHelp()
		os.Exit(0)
	}

	if filteredArgs[0] == "--version" || filteredArgs[0] == "-v" {
		fmt.Printf("hiveexplorer %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s\n", date)
		os.Exit(0)
	}

	hivePath := filteredArgs[0]
	logger.Info("starting hiveexplorer", "path", hivePath, "debug", debugMode)

	// Check if file exists
	if _, err := os.Stat(hivePath); err != nil {
		logger.Error("hive file not found", "path", hivePath, "error", err)
		fmt.Fprintf(os.Stderr, "Error: hive file not found: %s\n", hivePath)
		os.Exit(1)
	}

	// Create the TUI model
	m := NewModel(hivePath)

	// Create the Bubbletea program
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run the program
	finalModel, err := p.Run()
	if err != nil {
		logger.Error("TUI error", "error", err)
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	// Clean up resources
	if model, ok := finalModel.(Model); ok {
		if err := model.Close(); err != nil {
			// Log error but don't fail - cleanup is best effort
			logger.Warn("error closing resources", "error", err)
		}
	}

	logger.Info("hiveexplorer exited normally")
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: hiveexplorer [options] <hive-file>\n")
	fmt.Fprintf(os.Stderr, "Try 'hiveexplorer --help' for more information.\n")
}

func printHelp() {
	fmt.Println("hiveexplorer - Interactive TUI for Windows Registry Hive Files")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  hiveexplorer [options] <hive-file>")
	fmt.Println()
	fmt.Println("DESCRIPTION:")
	fmt.Println("  Launches an interactive terminal UI for exploring Windows registry hive files.")
	fmt.Println()
	fmt.Println("  Features:")
	fmt.Println("    - Split-pane layout (tree view + value table)")
	fmt.Println("    - Keyboard navigation (vim-style keys supported)")
	fmt.Println("    - Expand/collapse registry keys")
	fmt.Println("    - View all value types with proper formatting")
	fmt.Println("    - Search keys and values (/, Ctrl+F)")
	fmt.Println("    - Bookmark keys (b)")
	fmt.Println("    - Diff mode (Ctrl+D)")
	fmt.Println("    - Real-time key statistics")
	fmt.Println()
	fmt.Println("  Navigation:")
	fmt.Println("    ↑/k, ↓/j    Navigate up/down")
	fmt.Println("    →/l, Enter  Expand key / Enter key")
	fmt.Println("    ←/h         Collapse key / Go to parent")
	fmt.Println("    Tab         Switch between tree and value panes")
	fmt.Println("    ?           Show help")
	fmt.Println("    q           Quit")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  -d, --debug    Enable debug logging to ~/.hiveexplorer/logs/")
	fmt.Println("  -h, --help     Show this help message")
	fmt.Println("  -v, --version  Show version information")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  hiveexplorer system.hive")
	fmt.Println("  hiveexplorer software.hive")
	fmt.Println()
	fmt.Println("For non-interactive operations, use the 'hivectl' command instead.")
}
