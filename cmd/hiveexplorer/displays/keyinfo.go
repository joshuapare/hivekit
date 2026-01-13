package displays

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// KeyInfoDisplay shows detailed metadata about the currently selected registry key.
// It subscribes to navigation signals and loads data on-demand with cancellation support.
type KeyInfoDisplay struct {
	reader          hive.Reader
	nodeID          hive.NodeID
	path            string
	name            string
	subkeyN         int
	valueN          int
	lastWrite       time.Time
	className       string
	flags           uint16
	hasSecDesc      bool
	maxValueNameLen uint32
	maxValueDataLen uint32
	loaded          bool
	width           int
	navBus          *keyselection.Bus // Navigation bus for receiving signals
}

// NewKeyInfoDisplay creates a new key info display component.
func NewKeyInfoDisplay(reader hive.Reader, width int) *KeyInfoDisplay {
	return &KeyInfoDisplay{
		reader: reader,
		width:  width,
	}
}

// SetNavigationBus sets the navigation bus for this key info display.
// The display will subscribe to navigation signals and load metadata on-demand.
func (k *KeyInfoDisplay) SetNavigationBus(bus *keyselection.Bus) {
	k.navBus = bus
}

// StartListening begins listening to navigation signals and loading key metadata.
// This should be called after SetNavigationBus is called.
// It returns a Bubble Tea command that will process navigation signals.
func (k *KeyInfoDisplay) StartListening() tea.Cmd {
	if k.navBus == nil {
		return nil
	}

	// Subscribe to navigation signals
	signals := k.navBus.Subscribe()

	// Return a command that listens for signals
	return func() tea.Msg {
		// Wait for next signal
		sig := <-signals

		// Check if cancelled
		select {
		case <-sig.Ctx.Done():
			// Navigation was cancelled, don't load
			return nil
		default:
			// Load key info for this navigation
			return KeyInfoSignalMsg{Signal: sig}
		}
	}
}

// KeyInfoSignalMsg is a message for key info navigation signals
// Exported so main Update can forward it to KeyInfo component
type KeyInfoSignalMsg struct {
	Signal keyselection.Event
}

// loadKeyInfoAsync loads key metadata asynchronously using the navigation signal
func (k *KeyInfoDisplay) loadKeyInfoAsync(sig keyselection.Event) tea.Cmd {
	return func() tea.Msg {
		// Check if cancelled before starting
		select {
		case <-sig.Ctx.Done():
			return nil
		default:
		}

		// Load key info using the reader
		err := k.LoadInfoWithReader(sig.NodeID, sig.Path, k.reader)
		if err != nil {
			// Log error but don't fail - just don't update display
			return nil
		}

		// Return nil - display already updated its internal state
		return nil
	}
}

// Update handles Bubble Tea messages for the key info display
func (k *KeyInfoDisplay) Update(msg tea.Msg) (*KeyInfoDisplay, tea.Cmd) {
	switch msg := msg.(type) {
	case KeyInfoSignalMsg:
		// Navigation signal received - load key info
		// Also restart listening for next signal
		return k, tea.Batch(
			k.loadKeyInfoAsync(msg.Signal),
			k.StartListening(), // Keep listening
		)
	}
	return k, nil
}

// LoadInfo loads metadata for a registry key using the internal reader.
// It respects the context for cancellation - if the user navigates away quickly,
// the load will be cancelled.
func (k *KeyInfoDisplay) LoadInfo(nodeID hive.NodeID, path string) error {
	return k.LoadInfoWithReader(nodeID, path, k.reader)
}

// LoadInfoWithReader loads metadata for a registry key using a specific reader.
// This is used in diff mode to load from the appropriate hive.
func (k *KeyInfoDisplay) LoadInfoWithReader(
	nodeID hive.NodeID,
	path string,
	reader hive.Reader,
) error {
	if reader == nil {
		return fmt.Errorf("reader is nil")
	}

	// Load detailed key metadata using the reader
	detail, err := reader.DetailKey(nodeID)
	if err != nil {
		return fmt.Errorf("failed to load key metadata: %w", err)
	}

	// Update display state with basic metadata
	k.nodeID = nodeID
	k.path = path
	k.name = detail.Name
	k.subkeyN = detail.SubkeyN
	k.valueN = detail.ValueN
	k.lastWrite = detail.LastWrite
	k.hasSecDesc = detail.HasSecDesc

	// Update with detailed NK record fields
	k.className = detail.ClassName
	k.flags = detail.Flags
	k.maxValueNameLen = detail.MaxValueNameLength
	k.maxValueDataLen = detail.MaxValueDataLength
	k.loaded = true

	return nil
}

// SetWidth updates the display width for rendering.
func (k *KeyInfoDisplay) SetWidth(width int) {
	k.width = width
}

// View renders the key info display.
func (k *KeyInfoDisplay) View() string {
	if !k.loaded {
		return k.renderEmpty()
	}

	return k.renderInfo()
}

// renderEmpty renders a placeholder when no key is selected.
func (k *KeyInfoDisplay) renderEmpty() string {
	style := lipgloss.NewStyle().
		Width(k.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("No key selected")

	return style.Render(content)
}

// renderInfo renders the key metadata in a double-column fixed-height layout.
func (k *KeyInfoDisplay) renderInfo() string {
	borderStyle := lipgloss.NewStyle().
		Width(k.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255"))

	// Calculate column widths (split available width, accounting for borders and padding)
	// Border adds 2, padding adds 2 horizontally, plus 2 for column spacing
	availableWidth := k.width - 6
	leftWidth := availableWidth / 2
	rightWidth := availableWidth - leftWidth

	// Build left column (always 6 lines)
	leftLines := make([]string, 6)

	// Line 1: Key name
	nameLabel := labelStyle.Render("Key: ")
	nameValue := k.truncate(k.name, leftWidth-len("Key: "))
	leftLines[0] = nameLabel + valueStyle.Render(nameValue)

	// Line 2: Path
	pathLabel := labelStyle.Render("Path: ")
	pathValue := k.truncate(k.path, leftWidth-len("Path: "))
	leftLines[1] = pathLabel + valueStyle.Render(pathValue)

	// Line 3: Node ID
	nodeLabel := labelStyle.Render("Node: ")
	nodeValue := fmt.Sprintf("0x%x", k.nodeID)
	leftLines[2] = nodeLabel + valueStyle.Render(nodeValue)

	// Line 4: Class name (or placeholder)
	classLabel := labelStyle.Render("Class: ")
	classValue := "—"
	if k.className != "" {
		classValue = k.truncate(k.className, leftWidth-len("Class: "))
	}
	leftLines[3] = classLabel + valueStyle.Render(classValue)

	// Line 5: Subkeys count
	subkeysLabel := labelStyle.Render("Subkeys: ")
	subkeysValue := fmt.Sprintf("%d", k.subkeyN)
	leftLines[4] = subkeysLabel + valueStyle.Render(subkeysValue)

	// Line 6: Values count
	valuesLabel := labelStyle.Render("Values: ")
	valuesValue := fmt.Sprintf("%d", k.valueN)
	leftLines[5] = valuesLabel + valueStyle.Render(valuesValue)

	// Build right column (always 6 lines)
	rightLines := make([]string, 6)

	// Line 1: Modified time (or placeholder)
	modLabel := labelStyle.Render("Modified: ")
	modValue := "—"
	if !k.lastWrite.IsZero() {
		modValue = k.lastWrite.Format("2006-01-02 15:04")
	}
	rightLines[0] = modLabel + valueStyle.Render(modValue)

	// Line 2: Security descriptor
	secLabel := labelStyle.Render("Security: ")
	secValue := "No"
	if k.hasSecDesc {
		secValue = "Yes"
	}
	rightLines[1] = secLabel + valueStyle.Render(secValue)

	// Line 3: Encoding
	encLabel := labelStyle.Render("Encoding: ")
	encValue := "UTF-16LE"
	if k.isCompressedName() {
		encValue = "ANSI"
	}
	rightLines[2] = encLabel + valueStyle.Render(encValue)

	// Line 4: Max name length
	maxNameLabel := labelStyle.Render("Max Name: ")
	maxNameValue := "—"
	if k.maxValueNameLen > 0 {
		maxNameValue = formatBytes(k.maxValueNameLen)
	}
	rightLines[3] = maxNameLabel + valueStyle.Render(maxNameValue)

	// Line 5: Max data length
	maxDataLabel := labelStyle.Render("Max Data: ")
	maxDataValue := "—"
	if k.maxValueDataLen > 0 {
		maxDataValue = formatBytes(k.maxValueDataLen)
	}
	rightLines[4] = maxDataLabel + valueStyle.Render(maxDataValue)

	// Line 6: Empty (for fixed height)
	rightLines[5] = ""

	// Ensure columns are same width for proper alignment
	leftColumn := lipgloss.NewStyle().Width(leftWidth).Render(strings.Join(leftLines, "\n"))
	rightColumn := lipgloss.NewStyle().Width(rightWidth).Render(strings.Join(rightLines, "\n"))

	// Join columns horizontally with spacing
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		lipgloss.NewStyle().Width(2).Render("  "), // Column spacing
		rightColumn,
	)

	rendered := borderStyle.Render(content)

	// DEBUG: Log rendered height to verify it's fixed
	logger.Debug("KeyInfo rendered", "height", lipgloss.Height(rendered), "expected", 8)

	return rendered
}

// truncate truncates a string to fit within maxLen, adding "..." if needed.
func (k *KeyInfoDisplay) truncate(s string, maxLen int) string {
	// Guard against negative or zero maxLen
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Clear clears the display state.
func (k *KeyInfoDisplay) Clear() {
	k.loaded = false
	k.nodeID = 0
	k.path = ""
	k.name = ""
	k.subkeyN = 0
	k.valueN = 0
	k.lastWrite = time.Time{}
	k.className = ""
	k.flags = 0
	k.hasSecDesc = false
	k.maxValueNameLen = 0
	k.maxValueDataLen = 0
}

// isCompressedName returns true if the NK record uses compressed (ANSI) name encoding.
// Flag 0x0020 indicates compressed name according to Windows Registry documentation.
func (k *KeyInfoDisplay) isCompressedName() bool {
	return k.flags&0x0020 != 0
}

// formatBytes formats a byte count as human-readable string.
func formatBytes(bytes uint32) string {
	if bytes == 0 {
		return "0"
	}
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}
