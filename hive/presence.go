package hive

// PresenceResult contains the results of a key presence check.
type PresenceResult struct {
	// Missing contains the list of key paths that do not exist in the hive.
	Missing []string
}

// CheckKeyPresence checks if multiple key paths exist in a hive.
//
// This is a batch operation that checks the existence of multiple keys
// efficiently. It returns a PresenceResult containing only the paths
// that were not found.
//
// Parameters:
//   - hivePath: Absolute path to the hive file
//   - paths: List of key paths to check (e.g., []string{"Software\\Microsoft", "System"})
//
// Returns:
//   - PresenceResult: Contains list of missing paths (empty if all exist)
//   - error: If hive cannot be opened or other I/O error occurs
//
// Example:
//
//	result, err := hive.CheckKeyPresence("/path/to/hive", []string{
//	    "Software\\Microsoft\\Windows",
//	    "System\\CurrentControlSet",
//	    "NonExistent\\Key",
//	})
//	if err != nil {
//	    return err
//	}
//	// result.Missing would contain: ["NonExistent\\Key"]
func CheckKeyPresence(hivePath string, paths []string) (PresenceResult, error) {
	result := PresenceResult{
		Missing: make([]string, 0),
	}

	// Open the hive
	h, err := Open(hivePath)
	if err != nil {
		return result, err
	}
	defer h.Close()

	// Check each path
	for _, path := range paths {
		_, err := h.Find(path)
		if err != nil {
			// Path not found - add to missing list
			result.Missing = append(result.Missing, path)
		}
	}

	return result, nil
}
