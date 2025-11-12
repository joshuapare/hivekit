package hive

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joshuapare/hivekit/internal/reader"
)

// DiffStatus represents the diff state of an item.
type DiffStatus int

const (
	DiffUnchanged DiffStatus = iota // Item exists in both/no diff mode
	DiffAdded                       // Item added (only in new)
	DiffRemoved                     // Item removed (only in old)
	DiffModified                    // Item modified (exists in both but changed)
)

// KeyDiff represents differences for a single key.
type KeyDiff struct {
	Path         string
	Name         string
	Status       DiffStatus
	SubkeyN      int
	ValueN       int
	LastWrite    time.Time
	OldLastWrite time.Time // For modified keys
	ValueDiffs   []ValueDiff
}

// ValueDiff represents differences for a single value.
type ValueDiff struct {
	Name      string
	Status    DiffStatus
	Type      string
	OldType   string // For modified values
	Size      int
	OldSize   int // For modified values
	Data      []byte
	OldData   []byte // For modified values
	StringVal string
}

// HiveDiff contains the complete diff between two hives.
type HiveDiff struct {
	OldPath  string
	NewPath  string
	KeyDiffs map[string]KeyDiff // Map of path -> KeyDiff
}

// DiffHives compares two registry hives and returns their differences.
func DiffHives(oldPath, newPath string) (*HiveDiff, error) {
	fmt.Fprintf(os.Stderr, "[DIFF] Starting diff: old=%s, new=%s\n", oldPath, newPath)

	// Open both hives
	fmt.Fprintf(os.Stderr, "[DIFF] Opening old ..\n")
	oldReader, err := reader.Open(oldPath, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open old hive: %w", err)
	}
	defer oldReader.Close()

	fmt.Fprintf(os.Stderr, "[DIFF] Opening new ..\n")
	newReader, err := reader.Open(newPath, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open new hive: %w", err)
	}
	defer newReader.Close()

	// Load all keys from both hives
	fmt.Fprintf(os.Stderr, "[DIFF] Loading all keys from old ..\n")
	oldKeys, err := loadAllKeys(oldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load old hive keys: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DIFF] Loaded %d keys from old hive\n", len(oldKeys))

	fmt.Fprintf(os.Stderr, "[DIFF] Loading all keys from new ..\n")
	newKeys, err := loadAllKeys(newPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load new hive keys: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DIFF] Loaded %d keys from new hive\n", len(newKeys))

	// Build maps for quick lookup
	fmt.Fprintf(os.Stderr, "[DIFF] Building lookup maps...\n")
	oldKeyMap := make(map[string]KeyInfo)
	for _, k := range oldKeys {
		oldKeyMap[k.Path] = k
	}

	newKeyMap := make(map[string]KeyInfo)
	for _, k := range newKeys {
		newKeyMap[k.Path] = k
	}

	// Compare keys
	fmt.Fprintf(os.Stderr, "[DIFF] Comparing keys...\n")
	diff := &HiveDiff{
		OldPath:  oldPath,
		NewPath:  newPath,
		KeyDiffs: make(map[string]KeyDiff),
	}

	// Find added and modified keys
	for path, newKey := range newKeyMap {
		oldKey, existsInOld := oldKeyMap[path]

		if !existsInOld {
			// Key added
			diff.KeyDiffs[path] = KeyDiff{
				Path:      newKey.Path,
				Name:      newKey.Name,
				Status:    DiffAdded,
				SubkeyN:   newKey.SubkeyN,
				ValueN:    newKey.ValueN,
				LastWrite: newKey.LastWrite,
			}
		} else {
			// Key exists in both - check if modified
			valueDiffs, modified := compareKeyValues(oldPath, newPath, path)

			// Consider a key modified if values changed or timestamp changed
			if modified || !oldKey.LastWrite.Equal(newKey.LastWrite) {
				diff.KeyDiffs[path] = KeyDiff{
					Path:         newKey.Path,
					Name:         newKey.Name,
					Status:       DiffModified,
					SubkeyN:      newKey.SubkeyN,
					ValueN:       newKey.ValueN,
					LastWrite:    newKey.LastWrite,
					OldLastWrite: oldKey.LastWrite,
					ValueDiffs:   valueDiffs,
				}
			} else {
				// Unchanged - still include for context
				diff.KeyDiffs[path] = KeyDiff{
					Path:      newKey.Path,
					Name:      newKey.Name,
					Status:    DiffUnchanged,
					SubkeyN:   newKey.SubkeyN,
					ValueN:    newKey.ValueN,
					LastWrite: newKey.LastWrite,
				}
			}
		}
	}

	// Find removed keys
	for path, oldKey := range oldKeyMap {
		if _, existsInNew := newKeyMap[path]; !existsInNew {
			diff.KeyDiffs[path] = KeyDiff{
				Path:      oldKey.Path,
				Name:      oldKey.Name,
				Status:    DiffRemoved,
				SubkeyN:   oldKey.SubkeyN,
				ValueN:    oldKey.ValueN,
				LastWrite: oldKey.LastWrite,
			}
		}
	}

	fmt.Fprintf(
		os.Stderr,
		"[DIFF] Diff complete: %d total keys (%d added, %d removed, %d modified)\n",
		len(
			diff.KeyDiffs,
		),
		countByStatus(diff, DiffAdded),
		countByStatus(diff, DiffRemoved),
		countByStatus(diff, DiffModified),
	)

	return diff, nil
}

// countByStatus counts keys with a specific diff status.
func countByStatus(diff *HiveDiff, status DiffStatus) int {
	count := 0
	for _, kd := range diff.KeyDiffs {
		if kd.Status == status {
			count++
		}
	}
	return count
}

// loadAllKeys recursively loads all keys from a hive.
func loadAllKeys(hivePath string) ([]KeyInfo, error) {
	fmt.Fprintf(os.Stderr, "[DIFF] loadAllKeys: loading from %s\n", hivePath)
	// List all keys recursively with no depth limit
	keys, err := ListKeys(hivePath, "", true, 0)
	fmt.Fprintf(os.Stderr, "[DIFF] loadAllKeys: loaded %d keys\n", len(keys))
	return keys, err
}

// compareKeyValues compares values for a specific key path in both hives.
func compareKeyValues(oldPath, newPath, keyPath string) ([]ValueDiff, bool) {
	oldValues, err := ListValues(oldPath, keyPath)
	if err != nil {
		oldValues = []ValueInfo{} // Treat as no values
	}

	newValues, err := ListValues(newPath, keyPath)
	if err != nil {
		newValues = []ValueInfo{} // Treat as no values
	}

	// Build maps for quick lookup
	oldValueMap := make(map[string]ValueInfo)
	for _, v := range oldValues {
		oldValueMap[v.Name] = v
	}

	newValueMap := make(map[string]ValueInfo)
	for _, v := range newValues {
		newValueMap[v.Name] = v
	}

	var diffs []ValueDiff
	modified := false

	// Find added and modified values
	for name, newVal := range newValueMap {
		oldVal, existsInOld := oldValueMap[name]

		if !existsInOld {
			// Value added
			diffs = append(diffs, ValueDiff{
				Name:      newVal.Name,
				Status:    DiffAdded,
				Type:      newVal.Type,
				Size:      newVal.Size,
				Data:      newVal.Data,
				StringVal: newVal.StringVal,
			})
			modified = true
		} else {
			// Value exists in both - check if modified
			valueModified := !bytesEqual(oldVal.Data, newVal.Data) ||
				oldVal.Type != newVal.Type ||
				oldVal.Size != newVal.Size

			if valueModified {
				diffs = append(diffs, ValueDiff{
					Name:      newVal.Name,
					Status:    DiffModified,
					Type:      newVal.Type,
					OldType:   oldVal.Type,
					Size:      newVal.Size,
					OldSize:   oldVal.Size,
					Data:      newVal.Data,
					OldData:   oldVal.Data,
					StringVal: newVal.StringVal,
				})
				modified = true
			} else {
				// Unchanged
				diffs = append(diffs, ValueDiff{
					Name:      newVal.Name,
					Status:    DiffUnchanged,
					Type:      newVal.Type,
					Size:      newVal.Size,
					Data:      newVal.Data,
					StringVal: newVal.StringVal,
				})
			}
		}
	}

	// Find removed values
	for name, oldVal := range oldValueMap {
		if _, existsInNew := newValueMap[name]; !existsInNew {
			diffs = append(diffs, ValueDiff{
				Name:      oldVal.Name,
				Status:    DiffRemoved,
				Type:      oldVal.Type,
				Size:      oldVal.Size,
				Data:      oldVal.Data,
				StringVal: oldVal.StringVal,
			})
			modified = true
		}
	}

	return diffs, modified
}

// bytesEqual compares two byte slices.
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

// GetParentPaths returns all parent paths for a given path
// e.g., "A\\B\\C" -> ["A", "A\\B"].
func GetParentPaths(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := strings.Split(path, "\\")
	var parents []string

	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "\\")
		parents = append(parents, parent)
	}

	return parents
}

// FilterDiffKeys filters keys based on diff view settings.
func FilterDiffKeys(diff *HiveDiff, showAdded, showRemoved, showModified, showUnchanged bool) []KeyDiff {
	if diff == nil {
		return []KeyDiff{}
	}

	var filtered []KeyDiff

	// Collect all keys that match the filter
	matchingPaths := make(map[string]bool)

	for path, keyDiff := range diff.KeyDiffs {
		include := false

		switch keyDiff.Status {
		case DiffAdded:
			include = showAdded
		case DiffRemoved:
			include = showRemoved
		case DiffModified:
			include = showModified
		case DiffUnchanged:
			include = showUnchanged
		}

		if include {
			matchingPaths[path] = true
		}
	}

	// If not showing unchanged, add parent paths for context
	if !showUnchanged {
		for path := range matchingPaths {
			parents := GetParentPaths(path)
			for _, parent := range parents {
				matchingPaths[parent] = true
			}
		}
	}

	// Build filtered list
	for path := range matchingPaths {
		if keyDiff, exists := diff.KeyDiffs[path]; exists {
			filtered = append(filtered, keyDiff)
		}
	}

	return filtered
}
