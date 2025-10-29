package main

import (
	"strings"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
)

// SearchResult represents a search match
type SearchResult struct {
	Type  string // "key" or "value"
	Index int    // Index in items array
	Path  string // For keys
	Name  string // For values
}

// SearchKeys searches for keys matching the query
func SearchKeys(items []keytree.Item, query string) []SearchResult {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for i, item := range items {
		// Search in key name and path
		if strings.Contains(strings.ToLower(item.Name), query) ||
			strings.Contains(strings.ToLower(item.Path), query) {
			results = append(results, SearchResult{
				Type:  "key",
				Index: i,
				Path:  item.Path,
				Name:  item.Name,
			})
		}
	}

	return results
}

// SearchValues searches for values matching the query
func SearchValues(items []valuetable.ValueRow, query string) []SearchResult {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for i, item := range items {
		// Search in value name, type, and value content
		if strings.Contains(strings.ToLower(item.Name), query) ||
			strings.Contains(strings.ToLower(item.Type), query) ||
			strings.Contains(strings.ToLower(item.Value), query) {
			results = append(results, SearchResult{
				Type:  "value",
				Index: i,
				Name:  item.Name,
			})
		}
	}

	return results
}

// HighlightMatch highlights a search match in a string
func HighlightMatch(text, query string) string {
	if query == "" {
		return text
	}

	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerText, lowerQuery)

	if idx == -1 {
		return text
	}

	// Return text with match highlighted using ANSI codes
	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]

	// Use searchMatchStyle from styles.go
	return before + searchMatchStyle.Render(match) + after
}
