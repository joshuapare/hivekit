package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuedetail"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// View renders the entire UI
func (m Model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}

	// If help overlay is showing, render it
	if m.showHelp {
		return m.renderHelpOverlay()
	}

	// If detail view is visible, use overlay to render foreground over background
	if m.valueDetail.IsVisible() && m.valueDetail.DisplayMode() == valuedetail.DetailModeModal {
		// Create overlay with current model state
		// We recreate it each render to ensure it has the latest state
		// (since bubbletea's Update returns new models, stored pointers would be stale)
		mainView := NewMainViewModel(&m)
		detailOverlay := overlay.New(
			&m.valueDetail,
			mainView,
			overlay.Center, // horizontal position
			overlay.Center, // vertical position
			0,
			0,
		)
		return detailOverlay.View()
	}

	// Otherwise render normal view (no overlay)
	header := m.renderHeader()
	content := m.renderContent()
	status := m.renderStatus()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		status,
	)
}

// renderHeader renders the header with hive name and current path
func (m Model) renderHeader() string {
	title := "Registry Hive Explorer"
	hiveName := fmt.Sprintf("Hive: %s", m.hivePath)

	currentPath := ""
	if item := m.keyTree.CurrentItem(); item != nil {
		currentPath = fmt.Sprintf("Path: %s", item.Path)
	}

	// Build header line
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		headerStyle.Render(title),
		lipgloss.NewStyle().Render("  "),
		pathStyle.Render(hiveName),
	)

	// Path on second line if we have one
	if currentPath != "" {
		header = lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			pathStyle.Render(currentPath),
		)
	}

	return header
}

// renderContent renders the split-pane content
func (m Model) renderContent() string {
	// Calculate pane widths (50-50 split)
	treeWidth := m.width / 2
	valueWidth := m.width - treeWidth

	// Calculate pane height (account for header and status bar)
	// Content boxes get rendered at paneHeight+1 (for title line)
	// So we need to leave room for: header + (paneHeight+1) + status = m.height
	paneHeight := max(m.height-8, 5)

	// Reserve space for hive info panel
	// Note: Since JoinVertical doesn't add spacing, we include the spacing in hive info height
	hiveInfoHeight := HiveInfoPanelHeight + HiveInfoPanelSpacing
	treeViewHeight := paneHeight - hiveInfoHeight

	if treeViewHeight < 5 {
		treeViewHeight = 5
		hiveInfoHeight = paneHeight - treeViewHeight
	}

	// DEBUG
	fmt.Fprintf(
		os.Stderr,
		"[VIEW] m.height=%d paneHeight=%d treeViewHeight=%d hiveInfoHeight=%d treeBox=%d valueBox=%d\n",
		m.height,
		paneHeight,
		treeViewHeight,
		hiveInfoHeight,
		treeViewHeight+1,
		paneHeight+3,
	)

	// Render tree pane
	treeTitle := "Keys"
	items := m.keyTree.GetItems()
	if items != nil {
		treeTitle = fmt.Sprintf("Keys (%d)", len(items))
	}

	// Pass bookmarks to tree for rendering
	m.keyTree.SetBookmarks(m.bookmarks)

	treeContent := m.keyTree.View()
	treePane := lipgloss.NewStyle().
		Width(treeWidth - 2).
		Height(treeViewHeight).
		Render(treeContent)

	var treeBox string
	switch m.focusedPane {
	case TreePane:
		treeBox = activePaneStyle.
			Width(treeWidth - 2).
			Height(treeViewHeight + 1).
			Render(lipgloss.JoinVertical(lipgloss.Left, treeTitle, treePane))
	default:
		treeBox = paneStyle.
			Width(treeWidth - 2).
			Height(treeViewHeight + 1).
			Render(lipgloss.JoinVertical(lipgloss.Left, treeTitle, treePane))
	}

	// Render hive info panel below tree
	var hiveInfoBox string
	if m.hiveInfo != nil {
		m.hiveInfo.SetSize(treeWidth-2, hiveInfoHeight)
		hiveInfoBox = m.hiveInfo.View()
	} else {
		// Empty placeholder if no hive info
		hiveInfoBox = lipgloss.NewStyle().
			Width(treeWidth - 2).
			Height(hiveInfoHeight).
			Render("")
	}

	// Combine tree and hive info vertically
	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		treeBox,
		hiveInfoBox,
	)

	// Measure the actual left column height - this is what the right column should match
	leftColumnHeight := lipgloss.Height(leftColumn)

	// DEBUG: Check actual rendered heights
	fmt.Fprintf(os.Stderr, "[VIEW] Rendered heights: treeBox=%d hiveInfoBox=%d leftColumn=%d\n",
		lipgloss.Height(treeBox), lipgloss.Height(hiveInfoBox), leftColumnHeight)

	// Right column: NK info panel above values panel
	// Render key info panel FIRST to measure its actual height
	var keyInfoBox string
	if m.keyInfo != nil {
		m.keyInfo.SetWidth(valueWidth - 2)
		keyInfoBox = m.keyInfo.View()
	} else {
		// Empty placeholder if keyInfo not available
		nkInfoHeight := NKInfoPanelHeight + NKInfoPanelSpacing
		keyInfoBox = lipgloss.NewStyle().
			Width(valueWidth - 2).
			Height(nkInfoHeight).
			Render("")
	}

	// Measure actual keyInfoBox height
	keyInfoBoxHeight := lipgloss.Height(keyInfoBox)

	// Calculate remaining height for values table to match leftColumn height exactly
	// valueBox = valueViewHeight + 1 (title) + 2 (borders from paneStyle)
	// So: leftColumnHeight = keyInfoBoxHeight + (valueViewHeight + 3)
	// Therefore: valueViewHeight = leftColumnHeight - keyInfoBoxHeight - 3
	valueViewHeight := leftColumnHeight - keyInfoBoxHeight - 3
	if valueViewHeight < 5 {
		valueViewHeight = 5 // Minimum height for values table
	}

	// Render value pane (below NK info)
	valueTitle := "Values"
	if m.valueTable.GetItems() != nil {
		count := m.valueTable.GetItemCount()
		cursor := m.valueTable.GetCursor() + 1
		valueTitle = fmt.Sprintf("Values (%d) [%d/%d]", count, cursor, count)
	}

	valueContent := m.valueTable.View()
	valuePane := lipgloss.NewStyle().
		Width(valueWidth - 2).
		Height(valueViewHeight).
		Render(valueContent)

	var valueBox string
	switch m.focusedPane {
	case ValuePane:
		valueBox = activePaneStyle.
			Width(valueWidth - 2).
			Height(valueViewHeight + 1).
			Render(lipgloss.JoinVertical(lipgloss.Left, valueTitle, valuePane))
	default:
		valueBox = paneStyle.
			Width(valueWidth - 2).
			Height(valueViewHeight + 1).
			Render(lipgloss.JoinVertical(lipgloss.Left, valueTitle, valuePane))
	}

	// Combine key info and value pane vertically (right column)
	rightColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		keyInfoBox,
		valueBox,
	)

	// DEBUG: Check actual rendered heights
	fmt.Fprintf(
		os.Stderr,
		"[VIEW] Right column: leftColumnHeight=%d keyInfoBoxHeight=%d valueViewHeight=%d keyInfoBox=%d valueBox=%d rightColumn=%d (should match leftColumn=%d)\n",
		leftColumnHeight,
		keyInfoBoxHeight,
		valueViewHeight,
		lipgloss.Height(keyInfoBox),
		lipgloss.Height(valueBox),
		lipgloss.Height(rightColumn),
		leftColumnHeight,
	)

	// Join left column (tree + hive info) with right column (key info + values) horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		rightColumn,
	)
}

// renderStatus renders the status bar with help text
func (m Model) renderStatus() string {
	// Show input prompt if in input mode
	switch m.inputMode {
	case SearchMode:
		prompt := searchPromptStyle.Render("Search: ") + m.inputBuffer + "█"
		return statusStyle.Width(m.width).Render(prompt)
	case GlobalValueSearchMode:
		prompt := searchPromptStyle.Render("Global Value Search: ") + m.inputBuffer + "█"
		if m.globalValueSearchInProgress {
			prompt += " (searching...)"
		}
		return statusStyle.Width(m.width).Render(prompt)
	case GoToPathMode:
		prompt := searchPromptStyle.Render("Go to path: ") + m.inputBuffer + "█"
		return statusStyle.Width(m.width).Render(prompt)
	case DiffPathMode:
		prompt := searchPromptStyle.Render("Compare with hive: ") + m.inputBuffer + "█"
		return statusStyle.Width(m.width).Render(prompt)
	}

	// Show status message if set (takes priority over normal help)
	if m.statusMessage != "" {
		return statusStyle.Width(m.width).Render(
			searchPromptStyle.Render(m.statusMessage),
		)
	}

	// Build help text based on context
	var help strings.Builder

	// Determine help context
	switch {
	case m.diffMode && m.focusedPane == TreePane:
		help.WriteString(helpStyle.Render("a/r/m/u: Toggle"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("v: Diff-only"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("d: Exit diff"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("q: Quit"))
	case m.valueDetail.IsVisible():
		help.WriteString(helpStyle.Render("ESC: Close Detail"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("↑/↓: Scroll"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("q: Quit"))
	case m.focusedPane == TreePane:
		help.WriteString(helpStyle.Render("↑/↓: Navigate"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("→/Enter: Expand"))
		help.WriteString(" │ ")
		if m.searchQuery != "" || m.globalValueSearchActive {
			if m.searchMatches > 0 || len(m.globalValueSearchResults) > 0 {
				help.WriteString(helpStyle.Render("n/N: Next/Prev"))
				help.WriteString(" │ ")
			}
			help.WriteString(helpStyle.Render("Esc: Clear"))
			help.WriteString(" │ ")
		}
		help.WriteString(helpStyle.Render("/: Search"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("^F: Values"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("d: Diff"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("?: Help"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("q: Quit"))
	default: // ValuePane
		help.WriteString(helpStyle.Render("↑/↓: Navigate"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("Enter: Details"))
		help.WriteString(" │ ")
		if m.searchQuery != "" || m.globalValueSearchActive {
			if m.searchMatches > 0 || len(m.globalValueSearchResults) > 0 {
				help.WriteString(helpStyle.Render("n/N: Next/Prev"))
				help.WriteString(" │ ")
			}
			help.WriteString(helpStyle.Render("Esc: Clear"))
			help.WriteString(" │ ")
		}
		help.WriteString(helpStyle.Render("/: Search"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("^F: Values"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("c: Copy"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("?: Help"))
		help.WriteString(" │ ")
		help.WriteString(helpStyle.Render("q: Quit"))
	}

	// Status line with counts and info
	keyCount := len(m.keyTree.GetItems())
	valueCount := m.valueTable.GetItemCount()

	var statsBuilder strings.Builder

	// Show diff mode indicator if active
	if m.diffMode {
		statsBuilder.WriteString(diffModifiedStyle.Render("DIFF"))
		statsBuilder.WriteString(" │ ")
	}

	statsBuilder.WriteString(statusCountStyle.Render(fmt.Sprintf("%d", keyCount)))
	statsBuilder.WriteString(" keys │ ")
	statsBuilder.WriteString(statusCountStyle.Render(fmt.Sprintf("%d", valueCount)))
	statsBuilder.WriteString(" values")

	// Add current path if available
	if item := m.keyTree.CurrentItem(); item != nil {
		pathPreview := item.Path
		// Truncate long paths
		maxPathLen := 40
		if len(pathPreview) > maxPathLen {
			pathPreview = "..." + pathPreview[len(pathPreview)-maxPathLen+3:]
		}
		statsBuilder.WriteString(" │ ")
		statsBuilder.WriteString(pathStyle.Render(pathPreview))
	}

	// Add search match info if searching
	if m.searchQuery != "" {
		statsBuilder.WriteString(" │ ")
		// Show the search query
		statsBuilder.WriteString(searchPromptStyle.Render("Search: "))
		statsBuilder.WriteString(pathStyle.Render(fmt.Sprintf("'%s'", m.searchQuery)))
		// Show match counter if there are matches
		if m.searchMatches > 0 {
			statsBuilder.WriteString(" ")
			statsBuilder.WriteString(
				statusCountStyle.Render(
					fmt.Sprintf("(%d/%d)", m.searchMatchIdx+1, m.searchMatches),
				),
			)
		}
	}

	// Add global value search results info
	if m.globalValueSearchActive && len(m.globalValueSearchResults) > 0 {
		statsBuilder.WriteString(" │ ")
		statsBuilder.WriteString(searchPromptStyle.Render("Value Search: "))
		// Count total matching values across all keys
		totalMatches := 0
		for _, result := range m.globalValueSearchResults {
			totalMatches += result.MatchCount
		}
		statsBuilder.WriteString(
			statusCountStyle.Render(
				fmt.Sprintf("(%d keys, %d values)", len(m.globalValueSearchResults), totalMatches),
			),
		)
	}

	// Join help and stats
	statusLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		help.String(),
		lipgloss.NewStyle().Width(10).Render(""), // Spacer
		statsBuilder.String(),
	)

	return statusStyle.
		Width(m.width).
		Render(statusLine)
}

// renderHelpOverlay renders the help overlay
func (m Model) renderHelpOverlay() string {
	// Create help content
	var helpContent strings.Builder

	// Title
	title := helpTitleStyle.Render("Keyboard Shortcuts")
	helpContent.WriteString(title)
	helpContent.WriteString("\n\n")

	// Key column width for alignment
	const keyWidth = 14

	// Navigation section
	helpContent.WriteString(modalTitleStyle.Render("Navigation"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("↑/↓ or k/j"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Move cursor up/down"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("←/→ or h/l"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Collapse/Expand keys"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Home or g"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Go to top"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("End or G"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Go to bottom"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Tab"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Switch between tree and values"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("p"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Go to parent key"))
	helpContent.WriteString("\n\n")

	// Tree Navigation section
	helpContent.WriteString(modalTitleStyle.Render("Tree Navigation"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("E"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Expand all children recursively"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("C"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Collapse all to root level"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+E"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Expand all siblings at current level"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+L"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Collapse to current level"))
	helpContent.WriteString("\n\n")

	// Actions section
	helpContent.WriteString(modalTitleStyle.Render("Actions"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Enter"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Expand/collapse or show details"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Esc"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Close detail or cancel input"))
	helpContent.WriteString("\n\n")

	// Search section
	helpContent.WriteString(modalTitleStyle.Render("Search & Navigation"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("/"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Search for keys or values (live filter)"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+F"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Global value search (expand to matches)"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Esc"))
	helpContent.WriteString("  ")
	if m.searchQuery != "" {
		helpContent.WriteString(helpDescStyle.Render("Clear active search filter"))
	} else {
		helpContent.WriteString(helpDescStyle.Render("Clear search filter"))
	}
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("n"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Next search match"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("N"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Previous search match"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+Home"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("First search match"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+End"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Last search match"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("Ctrl+G"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Go to specific path"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("c"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Copy current path"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("y"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Copy current value (in value pane)"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("F5"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Refresh current values"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("b"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Toggle bookmark"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("B"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Jump to next bookmark"))
	helpContent.WriteString("\n\n")

	// Diff Mode section
	helpContent.WriteString(modalTitleStyle.Render("Diff Mode"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("d"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Enter/exit diff mode"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("a/r/m/u"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Toggle added/removed/modified/unchanged"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("v"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Toggle diff-only view"))
	helpContent.WriteString("\n\n")

	// Other section
	helpContent.WriteString(modalTitleStyle.Render("Other"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("?"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Show this help"))
	helpContent.WriteString("\n")
	helpContent.WriteString(helpKeyStyle.Width(keyWidth).Render("q or Ctrl+C"))
	helpContent.WriteString("  ")
	helpContent.WriteString(helpDescStyle.Render("Quit"))
	helpContent.WriteString("\n\n")

	helpContent.WriteString(helpStyle.Render("Press Esc, ?, or q to close this help"))

	// Create bordered help box
	helpBox := modalStyle.
		Width(60).
		Render(helpContent.String())

	// Calculate centering
	helpHeight := lipgloss.Height(helpBox)
	helpWidth := lipgloss.Width(helpBox)

	verticalPadding := (m.height - helpHeight) / 2
	horizontalPadding := (m.width - helpWidth) / 2

	if verticalPadding < 0 {
		verticalPadding = 0
	}
	if horizontalPadding < 0 {
		horizontalPadding = 0
	}

	// Position the help box
	positioned := lipgloss.NewStyle().
		MarginTop(verticalPadding).
		MarginLeft(horizontalPadding).
		Render(helpBox)

	return positioned
}
