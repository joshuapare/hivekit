# Registry Display Components

This directory contains specialized display components for different types of registry structures.

## Overview

Each display component is responsible for rendering a specific type of registry record in a user-friendly format. These components are designed to be compact and informative, providing key metadata at a glance.

## Available Displays

### HiveInfo (`hiveinfo.go`)

Displays registry hive header (REGF) metadata in a compact panel.

**Information Shown:**
- **Version**: Major and minor format version (e.g., 1.3, 1.5)
- **Type**: Primary or Alternate hive
- **Modified**: Last write timestamp
- **Data Size**: Total size of HBIN data (in KB/MB)
- **Seq**: Primary/Secondary sequence numbers (for atomicity checks)
- **Cluster**: Clustering factor (only shown if non-zero, rarely used)

**Usage:**
```go
info, err := hivex.GetHiveInfo(hivePath)
if err != nil {
    // handle error
}

display := displays.NewHiveInfoDisplay(info)
display.SetSize(width, height)
rendered := display.View()
```

**TUI Integration:**
The hive info is displayed in the bottom-left panel, below the registry key tree.

## Future Display Types

Additional display components can be added for other registry structures:

- **NK Record** (Node Key): Detailed key information, timestamps, flags
- **VK Record** (Value Key): Detailed value information, data type specifics
- **SK Record** (Security): Security descriptor information
- **HBIN** (Hive Bin): Binary blob metadata, allocation info

## Design Principles

1. **Compact**: Display should fit in a small panel without scrolling
2. **Informative**: Show the most relevant metadata first
3. **Formatted**: Use appropriate units (KB/MB, formatted timestamps)
4. **Conditional**: Hide rarely-used fields when they're not relevant
5. **Styled**: Use consistent lipgloss styling matching the TUI theme

## Adding New Displays

To add a new display type:

1. Create a new file in this directory (e.g., `nkrecord.go`)
2. Define a struct with the display state
3. Implement `SetSize(width, height int)` for responsive sizing
4. Implement `View() string` to render the display
5. Export a constructor function (e.g., `NewNKRecordDisplay()`)
6. Add necessary public API functions to `pkg/hivex` if needed
7. Update this README with the new display type
