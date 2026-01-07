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
