package link

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
)

// ConflictStrategy defines how to handle conflicts when linking subtrees.
type ConflictStrategy int

const (
	// ConflictOverwrite replaces existing values with new ones from the child hive.
	ConflictOverwrite ConflictStrategy = iota

	// ConflictSkip keeps existing values and skips new ones from the child hive.
	ConflictSkip

	// ConflictError returns an error when a conflict is detected.
	ConflictError
)

// LinkOptions configures how a child hive is linked under a parent.
type LinkOptions struct {
	// MountPath is the path where the child hive will be mounted in the parent.
	// Example: "SYSTEM" or "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide"
	MountPath string

	// ImportRootValues determines whether values from the child's root key
	// are copied to the mount point key in the parent.
	ImportRootValues bool

	// FlattenDuplicateFirstSegment handles the case where the child hive's
	// first key segment matches the last segment of the mount path.
	// If true and they match, the duplicate segment is skipped.
	// Example: mounting SYSTEM hive under "SYSTEM" mount path with flatten=true
	// will skip the duplicate "SYSTEM" segment.
	FlattenDuplicateFirstSegment bool

	// ConflictStrategy determines how to handle value conflicts.
	// Conflicts occur when a value already exists at the target path.
	ConflictStrategy ConflictStrategy
}

// LinkStats contains statistics about a subtree linking operation.
type LinkStats struct {
	// KeysCreated is the number of new keys created in the parent hive.
	KeysCreated int

	// ValuesSet is the number of values set in the parent hive.
	ValuesSet int

	// FlattenApplied indicates whether the flatten logic was triggered.
	FlattenApplied bool

	// Conflicts is the number of value conflicts encountered.
	Conflicts int
}

// LinkSubtree links a child hive under a parent hive's mount path.
//
// This function copies the entire structure of a child hive into a parent hive
// at a specified mount point. It supports flexible options for handling
// duplicates, root values, and conflicts.
//
// Process:
//  1. Open both parent and child hives
//  2. Optionally flatten duplicate first segment
//  3. Walk the child hive tree
//  4. Rewrite all paths to be under the mount path
//  5. Handle conflicts according to strategy
//  6. Apply all operations in a single transaction
//
// Example:
//
//	stats, err := link.LinkSubtree(
//	    "parent.hive",
//	    "child.hive",
//	    link.LinkOptions{
//	        MountPath: "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide",
//	        ImportRootValues: true,
//	        FlattenDuplicateFirstSegment: false,
//	        ConflictStrategy: link.ConflictOverwrite,
//	    },
//	)
//
// Parameters:
//   - parentHivePath: Path to the parent hive file to modify
//   - childHivePath: Path to the child hive file to read from
//   - opts: Linking options
//
// Returns:
//   - LinkStats: Statistics about the operation
//   - error: If hives cannot be opened or operations fail
func LinkSubtree(parentHivePath, childHivePath string, opts LinkOptions) (LinkStats, error) {
	var stats LinkStats

	// Open parent hive
	parentHive, err := hive.Open(parentHivePath)
	if err != nil {
		return stats, fmt.Errorf("open parent hive: %w", err)
	}
	defer parentHive.Close()

	// Open child hive
	childHive, err := hive.Open(childHivePath)
	if err != nil {
		return stats, fmt.Errorf("open child hive: %w", err)
	}
	defer childHive.Close()

	// Parse mount path
	mountPath := splitAndStripPath(opts.MountPath)

	// Check for flatten logic
	skipFirstSegment := false
	if opts.FlattenDuplicateFirstSegment && len(mountPath) > 0 {
		// Get child hive's root subkeys to find first segment
		childRootKeys, err := childHive.ListSubkeys("")
		if err == nil && len(childRootKeys) > 0 {
			// Check if first child segment matches last mount segment
			firstChildKey := childRootKeys[0].Name
			lastMountSegment := mountPath[len(mountPath)-1]
			if strings.EqualFold(firstChildKey, lastMountSegment) {
				skipFirstSegment = true
				stats.FlattenApplied = true
			}
		}
	}

	// Create merge plan
	plan := merge.NewPlan()

	// Handle root values if requested
	if opts.ImportRootValues {
		err = importRootValues(childHive, mountPath, plan, opts.ConflictStrategy, &stats)
		if err != nil {
			return stats, fmt.Errorf("import root values: %w", err)
		}
	}

	// Walk child hive tree and build operations
	err = walkAndLink(childHive, "", mountPath, skipFirstSegment, parentHive, plan, opts.ConflictStrategy, &stats)
	if err != nil {
		return stats, fmt.Errorf("walk child hive: %w", err)
	}

	// Create merge session and apply operations
	sess, err := merge.NewSession(parentHive, merge.Options{})
	if err != nil {
		return stats, fmt.Errorf("create session: %w", err)
	}
	defer sess.Close()

	// Apply the plan
	applied, err := sess.ApplyWithTx(plan)
	if err != nil {
		return stats, fmt.Errorf("apply operations: %w", err)
	}

	// Update stats from applied results
	stats.KeysCreated = applied.KeysCreated
	stats.ValuesSet = applied.ValuesSet

	return stats, nil
}

// LinkSubtreeComponents is a convenience wrapper for component-style linking.
//
// This is equivalent to calling LinkSubtree with:
//   - ImportRootValues: true
//   - FlattenDuplicateFirstSegment: false
//   - ConflictStrategy: ConflictOverwrite
//
// This is useful for mounting component hives where the child hive is mounted
// raw under the mount path with root values imported directly.
//
// Example:
//
//	stats, err := link.LinkSubtreeComponents(
//	    "parent.hive",
//	    "component.hive",
//	    "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide\\Components",
//	)
func LinkSubtreeComponents(parentHivePath, childHivePath, mountPath string) (LinkStats, error) {
	return LinkSubtree(parentHivePath, childHivePath, LinkOptions{
		MountPath:                    mountPath,
		ImportRootValues:             true,
		FlattenDuplicateFirstSegment: false,
		ConflictStrategy:             ConflictOverwrite,
	})
}

// splitAndStripPath splits a path and strips hive root prefixes.
func splitAndStripPath(path string) []string {
	if path == "" {
		return []string{}
	}

	// Strip common hive root prefixes
	path = strings.TrimPrefix(path, "HKEY_LOCAL_MACHINE\\")
	path = strings.TrimPrefix(path, "HKEY_CURRENT_USER\\")
	path = strings.TrimPrefix(path, "HKEY_CLASSES_ROOT\\")
	path = strings.TrimPrefix(path, "HKEY_USERS\\")
	path = strings.TrimPrefix(path, "HKEY_CURRENT_CONFIG\\")
	path = strings.TrimPrefix(path, "HKLM\\")
	path = strings.TrimPrefix(path, "HKCU\\")
	path = strings.TrimPrefix(path, "HKCR\\")
	path = strings.TrimPrefix(path, "HKU\\")
	path = strings.TrimPrefix(path, "HKCC\\")

	if path == "" {
		return []string{}
	}

	// Split by backslash and filter empty segments
	segments := strings.Split(path, "\\")
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}

	return result
}

// walkAndLink recursively walks the child hive and builds link operations.
func walkAndLink(childHive *hive.Hive, childPath string, mountPath []string, skipFirstSegment bool, parentHive *hive.Hive, plan *merge.Plan, strategy ConflictStrategy, stats *LinkStats) error {
	// List subkeys at current path
	subkeys, err := childHive.ListSubkeys(childPath)
	if err != nil {
		return fmt.Errorf("list subkeys at %s: %w", childPath, err)
	}

	for _, subkey := range subkeys {
		// Build full child path
		var fullChildPath string
		if childPath == "" {
			fullChildPath = subkey.Name
		} else {
			fullChildPath = childPath + "\\" + subkey.Name
		}

		// Split child path into segments
		childPathSegments := splitAndStripPath(fullChildPath)

		// Apply flatten logic if needed
		targetPath := make([]string, len(mountPath))
		copy(targetPath, mountPath)

		if skipFirstSegment && len(childPathSegments) > 0 {
			// Skip first segment of child path
			targetPath = append(targetPath, childPathSegments[1:]...)
		} else {
			targetPath = append(targetPath, childPathSegments...)
		}

		// Ensure the key exists
		plan.Ops = append(plan.Ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: targetPath,
		})
		stats.KeysCreated++

		// Copy all values from this key
		err := copyKeyValues(childHive, fullChildPath, targetPath, parentHive, plan, strategy, stats)
		if err != nil {
			return fmt.Errorf("copy values from %s: %w", fullChildPath, err)
		}

		// Recurse into subkeys
		err = walkAndLink(childHive, fullChildPath, mountPath, skipFirstSegment, parentHive, plan, strategy, stats)
		if err != nil {
			return err
		}
	}

	return nil
}

// importRootValues imports values from the child's root key to the mount path.
func importRootValues(childHive *hive.Hive, mountPath []string, plan *merge.Plan, strategy ConflictStrategy, stats *LinkStats) error {
	// List all values at root
	values, err := childHive.ListValues("")
	if err != nil {
		return fmt.Errorf("list root values: %w", err)
	}

	// Copy each value to mount path
	for _, valueMeta := range values {
		_, data, err := childHive.GetValue("", valueMeta.Name)
		if err != nil {
			return fmt.Errorf("get root value %s: %w", valueMeta.Name, err)
		}

		// Add operation to set value at mount path
		plan.Ops = append(plan.Ops, merge.Op{
			Type:      merge.OpSetValue,
			KeyPath:   mountPath,
			ValueName: valueMeta.Name,
			ValueType: uint32(valueMeta.Type),
			Data:      data,
		})
		stats.ValuesSet++
	}

	return nil
}

// copyKeyValues copies all values from a child key to a target path in the parent.
func copyKeyValues(childHive *hive.Hive, childPath string, targetPath []string, parentHive *hive.Hive, plan *merge.Plan, strategy ConflictStrategy, stats *LinkStats) error {
	// List all values at this key
	values, err := childHive.ListValues(childPath)
	if err != nil {
		return fmt.Errorf("list values: %w", err)
	}

	// Copy each value
	for _, valueMeta := range values {
		_, data, err := childHive.GetValue(childPath, valueMeta.Name)
		if err != nil {
			return fmt.Errorf("get value %s: %w", valueMeta.Name, err)
		}

		// Check for conflicts if strategy requires it
		if strategy != ConflictOverwrite {
			// Check if value already exists in parent
			targetPathStr := strings.Join(targetPath, "\\")
			_, _, existsErr := parentHive.GetValue(targetPathStr, valueMeta.Name)
			if existsErr == nil {
				// Value exists - handle conflict
				stats.Conflicts++
				if strategy == ConflictError {
					return fmt.Errorf("conflict: value %s already exists at %s", valueMeta.Name, targetPathStr)
				} else if strategy == ConflictSkip {
					continue // Skip this value
				}
			}
		}

		// Add operation to set value
		plan.Ops = append(plan.Ops, merge.Op{
			Type:      merge.OpSetValue,
			KeyPath:   targetPath,
			ValueName: valueMeta.Name,
			ValueType: uint32(valueMeta.Type),
			Data:      data,
		})
		stats.ValuesSet++
	}

	return nil
}
