package index

// WalkPath performs an absolute path lookup by stepping through each component.
// Returns the final NK offset and true if the entire path exists, or 0 and false.
//
// Example:
//
//	offset, ok := WalkPath(idx, rootOff, "System", "CurrentControlSet", "Services")
//
// This is a convenience wrapper. For performance-critical code, manually step
// through the tree using GetNK() to avoid slice allocations.
//
// Path components are case-sensitive. For case-insensitive lookups, pre-normalize
// the components (e.g., strings.ToLower) before calling this function.
func WalkPath(idx Index, rootOff uint32, components ...string) (uint32, bool) {
	current := rootOff
	for _, name := range components {
		next, ok := idx.GetNK(current, name)
		if !ok {
			return 0, false
		}
		current = next
	}
	return current, true
}
