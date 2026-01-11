package main

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/displays"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuedetail"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Pane represents which pane is focused
type Pane int

const (
	TreePane Pane = iota
	ValuePane
)

// Layout constants
const (
	HiveInfoPanelHeight  = 6 // Height reserved for hive info display
	HiveInfoPanelSpacing = 2 // Spacing between tree and hive info
	NKInfoPanelHeight    = 8 // Height reserved for NK info display on right
	NKInfoPanelSpacing   = 0 // Spacing between NK info and values (no spacing, direct stack)
)

// InputMode represents different input modes
type InputMode int

const (
	NormalMode InputMode = iota
	SearchMode
	GoToPathMode
	GlobalValueSearchMode
)

// Model is the main application model
type Model struct {
	hivePath    string
	keyTree     keytree.Model
	valueTable  valuetable.ValueTableModel
	valueDetail valuedetail.ValueDetailModel
	hiveInfo    *displays.HiveInfoDisplay
	keyInfo     *displays.KeyInfoDisplay
	keys        KeyMap

	focusedPane Pane
	width       int
	height      int

	// Input modes
	inputMode   InputMode
	inputBuffer string // Buffer for search/path input

	// Search state
	searchQuery    string
	searchMatchIdx int // Current match index
	searchMatches  int // Total matches

	// Global value search state
	globalValueSearchActive     bool                      // Flag indicating we have search results (enables n/N navigation)
	globalValueSearchInProgress bool                      // Flag indicating search is currently running
	globalValueSearchResults    []GlobalValueSearchResult // Cached results from global value search
	globalValueSearchDebounce   *time.Timer               // Debounce timer for search input

	// Bookmarks
	bookmarks map[string]bool // Set of bookmarked key paths

	// Help overlay
	showHelp bool

	// Status message for temporary feedback
	statusMessage string

	// Navigation bus for coordinating component updates
	navBus *keyselection.Bus

	// Persistent reader for efficient value/metadata loading
	mainReader hive.Reader

	err error
}

// NewModel creates a new TUI model
func NewModel(hivePath string) Model {
	// Detail display mode can be configured via GOHIVEX_DETAIL_MODE env var
	// Options: "modal" (popup), "pane" (bottom pane)
	// Default: modal
	detailMode := valuedetail.DetailModeModal
	if mode := os.Getenv("GOHIVEX_DETAIL_MODE"); mode != "" {
		if mode == "pane" {
			detailMode = valuedetail.DetailModePane
		}
	}

	// Load hive header
	var hiveInfo *displays.HiveInfoDisplay
	info, err := hive.GetHiveInfo(hivePath)
	if err == nil {
		hiveInfo = displays.NewHiveInfoDisplay(info)
	}

	// Create navigation bus for coordinating component updates
	navBus := keyselection.NewBus()

	// Create tree and value table models
	keyTree := keytree.NewModel(hivePath)
	valueTable := valuetable.NewValueTableModel(hivePath)

	// Wire navigation bus to tree (tree will emit signals on cursor movement)
	keyTree.SetNavigationBus(navBus)

	// Wire navigation bus to value table (table will listen for signals)
	valueTable.SetNavigationBus(navBus)

	// Open persistent reader for efficient value loading (will be kept open for TUI lifetime)
	// Cleaned up via Close() method
	mainReader, err := reader.Open(hivePath, hive.OpenOptions{})
	if err == nil {
		valueTable.SetReader(mainReader)
	}
	// Note: If reader fails to open, value table will fall back to old method

	// Create KeyInfoDisplay with mainReader for on-demand metadata loading
	var keyInfo *displays.KeyInfoDisplay
	if err == nil && mainReader != nil {
		keyInfo = displays.NewKeyInfoDisplay(mainReader, 40) // Initial width, will be resized in view
		// Wire navigation bus to key info display (it will listen for signals)
		keyInfo.SetNavigationBus(navBus)
	}

	// Create key map
	keys := DefaultKeyMap()

	m := Model{
		hivePath:    hivePath,
		keyTree:     keyTree,
		valueTable:  valueTable,
		valueDetail: valuedetail.NewValueDetailModel(detailMode),
		hiveInfo:    hiveInfo,
		keyInfo:     keyInfo,
		keys:        keys,
		focusedPane: TreePane,
		inputMode:   NormalMode,
		bookmarks:   make(map[string]bool),
		navBus:      navBus,
		mainReader:  mainReader,
	}

	// Configure keytree with keys
	m.keyTree.SetKeys(keytree.Keys{
		Up:              keys.Up,
		Down:            keys.Down,
		Left:            keys.Left,
		Right:           keys.Right,
		PageUp:          keys.PageUp,
		PageDown:        keys.PageDown,
		Home:            keys.Home,
		End:             keys.End,
		Enter:           keys.Enter,
		GoToParent:      keys.GoToParent,
		ExpandAll:       keys.ExpandAll,
		CollapseAll:     keys.CollapseAll,
		ExpandLevel:     keys.ExpandLevel,
		CollapseToLevel: keys.CollapseToLevel,
		Copy:            keys.Copy,
		ToggleBookmark:  keys.ToggleBookmark,
	})
	m.keyTree.SetBookmarks(m.bookmarks)

	// Configure valuetable with keys
	m.valueTable.SetKeys(valuetable.Keys{
		Up:        keys.Up,
		Down:      keys.Down,
		PageUp:    keys.PageUp,
		PageDown:  keys.PageDown,
		Home:      keys.Home,
		End:       keys.End,
		Enter:     keys.Enter,
		CopyValue: keys.CopyValue,
	})

	return m
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// Start value table and keyInfo listening for navigation signals
	cmds := []tea.Cmd{
		m.keyTree.Init(),
		m.valueTable.Init(),
		m.valueTable.StartListening(), // Start listening for navigation signals
	}

	// Start keyInfo listening if available
	if m.keyInfo != nil {
		cmds = append(cmds, m.keyInfo.StartListening())
	}

	return tea.Batch(cmds...)
}

// Close cleans up resources held by the model
// Should be called when the TUI exits to properly close file handles
func (m *Model) Close() error {
	var lastErr error

	// Close main reader
	if m.mainReader != nil {
		if err := m.mainReader.Close(); err != nil {
			lastErr = err
		}
		m.mainReader = nil
	}

	// Close keytree reader
	if err := m.keyTree.Close(); err != nil {
		lastErr = err
	}

	return lastErr
}

// Messages

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// Type aliases to valuetable types
type (
	valuesLoadedMsg = valuetable.ValuesLoadedMsg
	ValueInfo       = valuetable.ValueInfo
)

// convertValueInfos is an alias to valuetable.ConvertValueInfos
var convertValueInfos = valuetable.ConvertValueInfos

type clearStatusMsg struct{}

// GlobalValueSearchResult represents a key that contains matching values
type GlobalValueSearchResult struct {
	KeyPath        string       // Path of the key containing matches
	MatchingValues []ValueMatch // List of values that matched
	MatchCount     int          // Total number of matching values in this key
}

// ValueMatch represents a value that matched the search query
type ValueMatch struct {
	Name      string // Value name
	Type      string // Value type
	Value     string // Value content (may be truncated)
	MatchedIn string // What matched: "name", "type", or "value"
}

// globalValueSearchCompleteMsg is sent when global value search completes
type globalValueSearchCompleteMsg struct {
	results []GlobalValueSearchResult
	query   string
}

// GetKeyTree returns the key tree model (for testing)
func (m *Model) GetKeyTree() *keytree.Model {
	return &m.keyTree
}
