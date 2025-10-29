package valuetable

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable/adapter"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable/display"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// ValueRow represents a value in the table
type ValueRow struct {
	Name       string
	Type       string
	Value      string
	Size       int
	Raw        []byte
	DiffStatus hive.DiffStatus // Diff state for comparison mode
	OldValue   string          // Previous value (for DiffModified)
	OldType    string          // Previous type (for DiffModified)
}

// ValueTableModel manages the value table
type ValueTableModel struct {
	hivePath string
	reader   interface{} // hive.Reader for direct value loading
	items    []ValueRow
	cursor   int
	viewport viewport.Model
	width    int
	height   int
	navBus   *keyselection.Bus // Navigation bus for receiving signals

	// Input handling
	keys Keys
}

// NewValueTableModel creates a new value table model
func NewValueTableModel(hivePath string) ValueTableModel {
	return ValueTableModel{
		hivePath: hivePath,
		items:    make([]ValueRow, 0),
		viewport: viewport.New(0, 0),
	}
}

// Message types for Bubble Tea

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// NavSignalReceivedMsg is sent when a navigation signal is received from the bus
type NavSignalReceivedMsg struct {
	Signal keyselection.Event
}

// ValuesLoadedMsg is exported for use in tui package
type ValuesLoadedMsg struct {
	Path   string
	Values []ValueInfo
}

// Internal alias for compatibility
type valuesLoadedMsg = ValuesLoadedMsg

// ValueInfo is a simplified version of hive.ValueInfo for messages
type ValueInfo struct {
	Name       string
	Type       string
	Size       int
	Data       []byte
	StringVal  string
	StringVals []string
	DWordVal   uint32
	QWordVal   uint64

	// Diff mode fields
	DiffStatus hive.DiffStatus // Diff state for comparison mode
	OldValue   string          // Previous value (for DiffModified)
	OldType    string          // Previous type (for DiffModified)
}

// ConvertValueInfos converts a slice of hive.ValueInfo to valuetable.ValueInfo.
// Exported for use in tui package.
func ConvertValueInfos(values []hive.ValueInfo) []ValueInfo {
	msgValues := make([]ValueInfo, len(values))
	for i, v := range values {
		msgValues[i] = ValueInfo{
			Name:       v.Name,
			Type:       v.Type,
			Size:       v.Size,
			Data:       v.Data,
			StringVal:  v.StringVal,
			StringVals: v.StringVals,
			DWordVal:   v.DWordVal,
			QWordVal:   v.QWordVal,
		}
	}
	return msgValues
}

// Init initializes the value table
func (m ValueTableModel) Init() tea.Cmd {
	return nil
}

// SetReader sets the hive reader for efficient value loading.
// This reader should be kept open for the lifetime of the TUI.
func (m *ValueTableModel) SetReader(reader interface{}) {
	m.reader = reader
}

// SetNavigationBus sets the navigation bus for this value table.
// The table will subscribe to navigation signals and load values on-demand.
func (m *ValueTableModel) SetNavigationBus(bus *keyselection.Bus) {
	m.navBus = bus
}

// SetKeys sets the keyboard shortcuts for this value table
func (m *ValueTableModel) SetKeys(keys Keys) {
	m.keys = keys
}

// StartListening begins listening to navigation signals and loading values.
// This should be called after SetReader and SetNavigationBus are called.
// It returns a Bubble Tea command that will process navigation signals.
func (m *ValueTableModel) StartListening() tea.Cmd {
	if m.navBus == nil {
		return nil
	}

	// Subscribe to navigation signals
	signals := m.navBus.Subscribe()

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
			// Load values for this navigation
			return NavSignalReceivedMsg{Signal: sig}
		}
	}
}

// loadValuesWithReader loads values using a persistent reader and NodeID.
// This is much more efficient than opening/closing the file for each load.
// In diff mode, it loads from the appropriate hive based on DiffStatus and compares values.
func (m *ValueTableModel) loadValuesWithReader(sig keyselection.Event) tea.Cmd {
	return func() tea.Msg {
		// Check if cancelled before starting
		select {
		case <-sig.Ctx.Done():
			return nil
		default:
		}

		// In diff mode, load values based on DiffStatus
		if sig.DiffMode {
			return m.loadValuesInDiffMode(sig)
		}

		// Normal mode: use mainReader
		r, ok := m.reader.(hive.Reader)
		if !ok || r == nil {
			// Fallback to old method if reader not set
			return errMsg{fmt.Errorf("hive reader not initialized")}
		}

		// Get value IDs for this node
		valueIDs, err := r.Values(sig.NodeID)
		if err != nil {
			// Check if cancelled during load
			select {
			case <-sig.Ctx.Done():
				return nil
			default:
				return errMsg{fmt.Errorf("failed to get values: %w", err)}
			}
		}

		// Load value metadata and data
		values := make([]ValueInfo, 0, len(valueIDs))
		for _, valID := range valueIDs {
			// Check cancellation periodically
			select {
			case <-sig.Ctx.Done():
				return nil
			default:
			}

			meta, err := r.StatValue(valID)
			if err != nil {
				continue // Skip broken values
			}

			data, err := r.ValueBytes(valID, hive.ReadOptions{CopyData: true})
			if err != nil {
				data = []byte{} // Empty data on error
			}

			// Format value based on type
			valInfo := ValueInfo{
				Name: meta.Name,
				Type: meta.Type.String(),
				Size: len(data),
				Data: data,
			}

			// Parse common types
			switch meta.Type {
			case hive.REG_SZ, hive.REG_EXPAND_SZ:
				valInfo.StringVal = string(data) // Simplified, should handle UTF-16
			case hive.REG_DWORD:
				if len(data) >= 4 {
					valInfo.DWordVal = uint32(
						data[0],
					) | uint32(
						data[1],
					)<<8 | uint32(
						data[2],
					)<<16 | uint32(
						data[3],
					)<<24
				}
			case hive.REG_QWORD:
				if len(data) >= 8 {
					valInfo.QWordVal = uint64(
						data[0],
					) | uint64(
						data[1],
					)<<8 | uint64(
						data[2],
					)<<16 | uint64(
						data[3],
					)<<24 |
						uint64(
							data[4],
						)<<32 | uint64(
						data[5],
					)<<40 | uint64(
						data[6],
					)<<48 | uint64(
						data[7],
					)<<56
				}
			}

			values = append(values, valInfo)
		}

		return valuesLoadedMsg{Path: sig.Path, Values: values}
	}
}

// loadValuesInDiffMode loads values in diff mode from the appropriate hive(s).
// For Added keys: loads from NewReader using NewNodeID
// For Removed keys: loads from OldReader using OldNodeID
// For Modified keys: loads from BOTH readers and compares (using both NodeIDs)
// For Unchanged keys: loads from OldReader using OldNodeID
func (m *ValueTableModel) loadValuesInDiffMode(sig keyselection.Event) tea.Msg {
	// Check cancellation
	select {
	case <-sig.Ctx.Done():
		return nil
	default:
	}

	switch sig.DiffStatus {
	case hive.DiffAdded:
		// Added key: load values from new hive only, using NewNodeID
		return m.loadValuesFromReader(sig, sig.NewReader, sig.NewNodeID, false, nil)

	case hive.DiffRemoved:
		// Removed key: load values from old hive only, using OldNodeID
		return m.loadValuesFromReader(sig, sig.OldReader, sig.OldNodeID, false, nil)

	case hive.DiffModified, hive.DiffUnchanged:
		// Modified/Unchanged: load from old hive first (using OldNodeID), then compare with new (using NewNodeID)
		// This allows us to show value-level diffs
		oldValues := m.loadValuesFromReaderSync(sig, sig.OldReader, sig.OldNodeID)
		newValues := m.loadValuesFromReaderSync(sig, sig.NewReader, sig.NewNodeID)

		// Compare values and build diff result
		return m.compareValues(sig, oldValues, newValues)

	default:
		// Fallback: load from old reader using OldNodeID
		return m.loadValuesFromReader(sig, sig.OldReader, sig.OldNodeID, false, nil)
	}
}

// loadValuesFromReader loads values from a specific reader using the provided NodeID (helper for diff mode)
func (m *ValueTableModel) loadValuesFromReader(
	sig keyselection.Event,
	reader hive.Reader,
	nodeID hive.NodeID,
	isOld bool,
	oldValues map[string]ValueInfo,
) tea.Msg {
	if reader == nil {
		return errMsg{fmt.Errorf("reader is nil")}
	}

	// Check cancellation
	select {
	case <-sig.Ctx.Done():
		return nil
	default:
	}

	// Get value IDs for this node using the appropriate NodeID
	valueIDs, err := reader.Values(nodeID)
	if err != nil {
		// Key may not exist in this hive (expected in diff mode)
		return valuesLoadedMsg{Path: sig.Path, Values: []ValueInfo{}}
	}

	// Load value metadata and data
	values := make([]ValueInfo, 0, len(valueIDs))
	for _, valID := range valueIDs {
		select {
		case <-sig.Ctx.Done():
			return nil
		default:
		}

		meta, err := reader.StatValue(valID)
		if err != nil {
			continue
		}

		data, err := reader.ValueBytes(valID, hive.ReadOptions{CopyData: true})
		if err != nil {
			data = []byte{}
		}

		valInfo := ValueInfo{
			Name: meta.Name,
			Type: meta.Type.String(),
			Size: len(data),
			Data: data,
		}

		// Parse common types
		switch meta.Type {
		case hive.REG_SZ, hive.REG_EXPAND_SZ:
			valInfo.StringVal = string(data)
		case hive.REG_DWORD:
			if len(data) >= 4 {
				valInfo.DWordVal = uint32(
					data[0],
				) | uint32(
					data[1],
				)<<8 | uint32(
					data[2],
				)<<16 | uint32(
					data[3],
				)<<24
			}
		case hive.REG_QWORD:
			if len(data) >= 8 {
				valInfo.QWordVal = uint64(
					data[0],
				) | uint64(
					data[1],
				)<<8 | uint64(
					data[2],
				)<<16 | uint64(
					data[3],
				)<<24 |
					uint64(
						data[4],
					)<<32 | uint64(
					data[5],
				)<<40 | uint64(
					data[6],
				)<<48 | uint64(
					data[7],
				)<<56
			}
		}

		values = append(values, valInfo)
	}

	return valuesLoadedMsg{Path: sig.Path, Values: values}
}

// loadValuesFromReaderSync synchronously loads values from a reader using the provided NodeID (for comparison)
func (m *ValueTableModel) loadValuesFromReaderSync(
	sig keyselection.Event,
	reader hive.Reader,
	nodeID hive.NodeID,
) map[string]ValueInfo {
	if reader == nil {
		return make(map[string]ValueInfo)
	}

	select {
	case <-sig.Ctx.Done():
		return make(map[string]ValueInfo)
	default:
	}

	valueIDs, err := reader.Values(nodeID)
	if err != nil {
		return make(map[string]ValueInfo)
	}

	values := make(map[string]ValueInfo)
	for _, valID := range valueIDs {
		select {
		case <-sig.Ctx.Done():
			return values
		default:
		}

		meta, err := reader.StatValue(valID)
		if err != nil {
			continue
		}

		data, err := reader.ValueBytes(valID, hive.ReadOptions{CopyData: true})
		if err != nil {
			data = []byte{}
		}

		valInfo := ValueInfo{
			Name: meta.Name,
			Type: meta.Type.String(),
			Size: len(data),
			Data: data,
		}

		// Parse common types
		switch meta.Type {
		case hive.REG_SZ, hive.REG_EXPAND_SZ:
			valInfo.StringVal = string(data)
		case hive.REG_DWORD:
			if len(data) >= 4 {
				valInfo.DWordVal = uint32(
					data[0],
				) | uint32(
					data[1],
				)<<8 | uint32(
					data[2],
				)<<16 | uint32(
					data[3],
				)<<24
			}
		case hive.REG_QWORD:
			if len(data) >= 8 {
				valInfo.QWordVal = uint64(
					data[0],
				) | uint64(
					data[1],
				)<<8 | uint64(
					data[2],
				)<<16 | uint64(
					data[3],
				)<<24 |
					uint64(
						data[4],
					)<<32 | uint64(
					data[5],
				)<<40 | uint64(
					data[6],
				)<<48 | uint64(
					data[7],
				)<<56
			}
		}

		values[meta.Name] = valInfo
	}

	return values
}

// compareValues compares old and new values and returns a valuesLoadedMsg with diff information
func (m *ValueTableModel) compareValues(
	sig keyselection.Event,
	oldValues, newValues map[string]ValueInfo,
) tea.Msg {
	result := make([]ValueInfo, 0)

	// Track which new values we've seen
	seenNew := make(map[string]bool)

	// Check old values: removed or modified
	for name, oldVal := range oldValues {
		newVal, exists := newValues[name]
		if !exists {
			// Value was removed
			oldVal.DiffStatus = hive.DiffRemoved
			result = append(result, oldVal)
		} else {
			// Value exists in both - check if modified
			seenNew[name] = true

			// Compare data
			if !bytesEqual(oldVal.Data, newVal.Data) || oldVal.Type != newVal.Type {
				// Modified value - show new with old value stored
				newVal.DiffStatus = hive.DiffModified
				newVal.OldValue = formatValue(oldVal)
				newVal.OldType = oldVal.Type
				result = append(result, newVal)
			} else {
				// Unchanged value
				newVal.DiffStatus = hive.DiffUnchanged
				result = append(result, newVal)
			}
		}
	}

	// Check for added values (in new but not in old)
	for name, newVal := range newValues {
		if !seenNew[name] {
			newVal.DiffStatus = hive.DiffAdded
			result = append(result, newVal)
		}
	}

	return valuesLoadedMsg{Path: sig.Path, Values: result}
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// formatValue formats a ValueInfo as a string (for displaying old values in diffs)
func formatValue(val ValueInfo) string {
	switch val.Type {
	case "REG_SZ", "REG_EXPAND_SZ":
		return val.StringVal
	case "REG_MULTI_SZ":
		return strings.Join(val.StringVals, ", ")
	case "REG_DWORD", "REG_DWORD_BE":
		return fmt.Sprintf("0x%08x (%d)", val.DWordVal, val.DWordVal)
	case "REG_QWORD":
		return fmt.Sprintf("0x%016x (%d)", val.QWordVal, val.QWordVal)
	case "REG_BINARY":
		if len(val.Data) > 16 {
			return hex.EncodeToString(val.Data[:16]) + "..."
		}
		return hex.EncodeToString(val.Data)
	default:
		if len(val.Data) > 16 {
			return hex.EncodeToString(val.Data[:16]) + "..."
		} else if len(val.Data) > 0 {
			return hex.EncodeToString(val.Data)
		}
		return "(empty)"
	}
}

// LoadValues loads values for a specific key path using synchronous file I/O.
//
// DEPRECATED: This method is primarily for testing purposes.
// Production code should use the bus-based navigation signal architecture
// (which uses loadValuesWithReader with a persistent reader for better performance).
//
// This method opens and closes the hive file on each call, which is slower
// than using a persistent reader. Use only when direct control is needed (e.g., tests).
func (m *ValueTableModel) LoadValues(keyPath string) tea.Cmd {
	return m.LoadValuesFromHive(keyPath, m.hivePath)
}

// LoadValuesFromHive loads values from a specific hive file using synchronous file I/O.
//
// DEPRECATED: This method is primarily for testing and diff mode fallback.
// Production code should use the bus-based navigation signal architecture
// (which uses loadValuesWithReader with a persistent reader for better performance).
//
// This method opens and closes the hive file on each call, which is slower
// than using a persistent reader. Use only when:
//   - Testing and you need direct control
//   - Diff mode and cached readers are unavailable (fallback)
func (m *ValueTableModel) LoadValuesFromHive(keyPath string, hivePath string) tea.Cmd {
	return func() tea.Msg {
		values, err := hive.ListValues(hivePath, keyPath)
		if err != nil {
			// Add detailed context to the error
			return errMsg{
				fmt.Errorf(
					"failed to load values for path %q in hive %q: %w",
					keyPath,
					hivePath,
					err,
				),
			}
		}
		// Convert using helper function
		return ValuesLoadedMsg{Path: keyPath, Values: ConvertValueInfos(values)}
	}
}

// Update handles messages
func (m *ValueTableModel) Update(msg tea.Msg) (ValueTableModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case NavSignalReceivedMsg:
		// Navigation signal received - load values for this key
		// Also restart listening for next signal
		return *m, tea.Batch(
			m.loadValuesWithReader(msg.Signal),
			m.StartListening(), // Keep listening
		)

	case valuesLoadedMsg:
		// Convert to value rows
		m.items = make([]ValueRow, 0, len(msg.Values))
		for _, val := range msg.Values {
			row := ValueRow{
				Name:       val.Name,
				Type:       val.Type,
				Size:       val.Size,
				Raw:        val.Data,
				DiffStatus: val.DiffStatus, // Copy diff status
				OldValue:   val.OldValue,   // Copy old value for modified items
				OldType:    val.OldType,    // Copy old type for modified items
			}

			// Format value based on type
			switch val.Type {
			case "REG_SZ", "REG_EXPAND_SZ":
				row.Value = val.StringVal
			case "REG_MULTI_SZ":
				row.Value = strings.Join(val.StringVals, ", ")
			case "REG_DWORD", "REG_DWORD_BE":
				row.Value = fmt.Sprintf("0x%08x (%d)", val.DWordVal, val.DWordVal)
			case "REG_QWORD":
				row.Value = fmt.Sprintf("0x%016x (%d)", val.QWordVal, val.QWordVal)
			case "REG_BINARY":
				if len(val.Data) > 16 {
					row.Value = hex.EncodeToString(val.Data[:16]) + "..."
				} else {
					row.Value = hex.EncodeToString(val.Data)
				}
			default:
				if len(val.Data) > 16 {
					row.Value = hex.EncodeToString(val.Data[:16]) + "..."
				} else if len(val.Data) > 0 {
					row.Value = hex.EncodeToString(val.Data)
				} else {
					row.Value = "(empty)"
				}
			}

			m.items = append(m.items, row)
		}
		m.cursor = 0
		m.viewport.YOffset = 0 // Reset scroll position when loading new values
		m.updateViewport()

	case tea.KeyMsg:
		// Handle keyboard input for value table navigation
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
		m.updateViewport()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return *m, cmd
}

// handleKeyMsg handles keyboard input for value table navigation and operations
func (m *ValueTableModel) handleKeyMsg(msg tea.KeyMsg) (ValueTableModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.MoveUp()

	case key.Matches(msg, m.keys.Down):
		m.MoveDown()

	case key.Matches(msg, m.keys.Enter):
		// Show detail view for selected value - emit message for main Model
		if value := m.CurrentItem(); value != nil {
			return *m, func() tea.Msg {
				return ValueSelectedMsg{
					Value: value,
				}
			}
		}

	case key.Matches(msg, m.keys.Home):
		// Move to top
		for m.cursor > 0 {
			m.MoveUp()
		}

	case key.Matches(msg, m.keys.End):
		// Move to bottom
		for m.cursor < len(m.items)-1 {
			m.MoveDown()
		}

	case key.Matches(msg, m.keys.CopyValue):
		// Copy current value - emit message for main Model to show status
		err := m.CopyCurrentValue()
		success := err == nil
		value := ""
		if item := m.CurrentItem(); item != nil {
			value = item.Value
		}
		return *m, func() tea.Msg {
			return CopyValueRequestedMsg{
				Value:   value,
				Success: success,
				Err:     err,
			}
		}
	}

	return *m, nil
}

// View renders the value table
func (m ValueTableModel) View() string {
	if len(m.items) == 0 {
		return "No values"
	}
	return m.viewport.View()
}

// updateViewport updates the viewport content
func (m *ValueTableModel) updateViewport() {
	fmt.Fprintf(os.Stderr, "[DEBUG] updateViewport: rendering %d items\n", len(m.items))
	var b strings.Builder

	// Use the actual viewport width (which accounts for pane borders/padding)
	// instead of m.width for accurate column calculations
	contentWidth := m.viewport.Width
	if contentWidth <= 0 {
		contentWidth = m.width // Fallback during initialization
	}
	fmt.Fprintf(
		os.Stderr,
		"[DEBUG] updateViewport: contentWidth=%d, viewport.Width=%d, m.width=%d\n",
		contentWidth,
		m.viewport.Width,
		m.width,
	)

	// Calculate column widths based on available width
	nameWidth := 20
	typeWidth := 15

	// Calculate space available for value column
	// diff prefix (1) + space (1) + name (20) + spaces (2) + type (15) + spaces (2) + value
	totalFixedWidth := 1 + 1 + nameWidth + 2 + typeWidth + 2
	valueWidth := contentWidth - totalFixedWidth

	// Ensure minimum width for value column even on very narrow terminals
	if valueWidth < 20 {
		valueWidth = 20
	}

	// Header - use pure display function
	b.WriteString(display.RenderHeader(nameWidth, typeWidth, valueWidth, contentWidth))
	b.WriteString("\n")

	// Separator - use pure display function
	b.WriteString(display.RenderSeparator(contentWidth))
	b.WriteString("\n")

	// Rows - use adapter → display pipeline
	for i, row := range m.items {
		fmt.Fprintf(
			os.Stderr,
			"[DEBUG]   rendering row %d: name=%q, type=%s\n",
			i,
			row.Name,
			row.Type,
		)

		// Convert domain ValueRow to ValueRowSource
		source := adapter.ValueRowSource{
			Name:       row.Name,
			Type:       row.Type,
			Value:      row.Value,
			DiffStatus: row.DiffStatus,
			OldValue:   row.OldValue,
			OldType:    row.OldType,
		}

		// Adapter converts domain data to display properties (ALL business logic)
		displayProps := adapter.RowToDisplayProps(
			source,
			i,
			i == m.cursor,
			nameWidth,
			typeWidth,
			valueWidth,
		)

		// Pure display function renders the row (ZERO business logic)
		line := display.RenderValueRow(displayProps, contentWidth)

		b.WriteString(line)
		if i < len(m.items)-1 {
			b.WriteString("\n")
		}
	}

	m.viewport.SetContent(b.String())
}

// MoveUp moves the cursor up
func (m *ValueTableModel) MoveUp() {
	if m.cursor > 0 {
		m.cursor--
		m.ensureCursorVisible()
	}
}

// MoveDown moves the cursor down
func (m *ValueTableModel) MoveDown() {
	if m.cursor < len(m.items)-1 {
		m.cursor++
		m.ensureCursorVisible()
	}
}

// ensureCursorVisible scrolls viewport to make cursor visible
func (m *ValueTableModel) ensureCursorVisible() {
	// Account for header (2 lines: header + separator)
	headerHeight := 2
	visibleHeight := m.viewport.Height - headerHeight

	if visibleHeight <= 0 {
		return
	}

	if m.cursor < m.viewport.YOffset {
		m.viewport.YOffset = m.cursor
	} else if m.cursor >= m.viewport.YOffset+visibleHeight {
		m.viewport.YOffset = m.cursor - visibleHeight + 1
	}
	m.updateViewport()
}

// CurrentItem returns the currently selected item
func (m *ValueTableModel) CurrentItem() *ValueRow {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return &m.items[m.cursor]
	}
	return nil
}

// GetItemCount returns the number of values
func (m *ValueTableModel) GetItemCount() int {
	return len(m.items)
}

// Clear clears all values
func (m *ValueTableModel) Clear() {
	m.items = make([]ValueRow, 0)
	m.cursor = 0
	m.updateViewport()
}

// CopyCurrentValue copies the current value to clipboard
func (m *ValueTableModel) CopyCurrentValue() error {
	if m.cursor >= len(m.items) {
		return fmt.Errorf("no value selected")
	}

	row := m.items[m.cursor]
	// Copy the value field which contains the formatted value
	return clipboard.WriteAll(row.Value)
}

// GetItems returns the current items (for search functionality)
func (m *ValueTableModel) GetItems() []ValueRow {
	return m.items
}

// GetCursor returns the current cursor position
func (m *ValueTableModel) GetCursor() int {
	return m.cursor
}

// SetCursor sets the cursor position (for search functionality)
func (m *ValueTableModel) SetCursor(cursor int) {
	m.cursor = cursor
}

// EnsureCursorVisible ensures the cursor is visible in the viewport (exported for search)
func (m *ValueTableModel) EnsureCursorVisible() {
	m.ensureCursorVisible()
}

// GetViewportYOffset returns the viewport Y offset (for debugging)
func (m *ValueTableModel) GetViewportYOffset() int {
	return m.viewport.YOffset
}
