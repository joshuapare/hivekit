package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if os.Args[1] == "--help" || os.Args[1] == "-h" {
		printHelp()
		os.Exit(0)
	}

	if os.Args[1] == "--version" || os.Args[1] == "-v" {
		fmt.Printf("hiveexplorer %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s\n", date)
		os.Exit(0)
	}

	hivePath := os.Args[1]

	// Check if file exists
	if _, err := os.Stat(hivePath); err != nil {
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
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	// Clean up resources
	if model, ok := finalModel.(Model); ok {
		if err := model.Close(); err != nil {
			// Log error but don't fail - cleanup is best effort
			fmt.Fprintf(os.Stderr, "Warning: error closing resources: %v\n", err)
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: hiveexplorer <hive-file>\n")
	fmt.Fprintf(os.Stderr, "Try 'hiveexplorer --help' for more information.\n")
}

func printHelp() {
	fmt.Println("hiveexplorer - Interactive TUI for Windows Registry Hive Files")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  hiveexplorer <hive-file>")
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
	fmt.Println("  -h, --help     Show this help message")
	fmt.Println("  -v, --version  Show version information")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  hiveexplorer system.hive")
	fmt.Println("  hiveexplorer software.hive")
	fmt.Println()
	fmt.Println("For non-interactive operations, use the 'hivectl' command instead.")
}
