package index

// ReadOnlyIndex is the read-only interface for fast NK/VK lookups.
// Use this when you only need to query the index without modifying it.
//
// Design: Lookups use (parentOffset, name) tuples instead of full paths.
// This eliminates []string allocations and mirrors the actual hive structure.
//
// For absolute path lookups (e.g., "HKLM\System\Services\Foo"), use the
// WalkPath helper which steps through the tree one component at a time.
type ReadOnlyIndex interface {
	// GetNK returns the NK offset given its parent's offset and name.
	// For the root NK, use parentOff = 0.
	GetNK(parentOff uint32, name string) (nkOff uint32, ok bool)

	// GetVK returns the VK offset given the parent NK's offset and value name.
	// Empty name ("") is valid for the (Default) value.
	GetVK(parentOff uint32, valueName string) (vkOff uint32, ok bool)

	// Stats returns index statistics (size, allocations, impl type).
	Stats() Stats
}

// Index is the full mutable interface for hive indexing.
// It embeds ReadOnlyIndex and adds mutation operations (Add/Remove).
//
// Use this when you need to build or modify the index.
// Both StringIndex and UniqueIndex implement this interface.
//
// Typical usage:
//   - Build phase: AddNK/AddVK as you scan the hive
//   - Edit phase: AddNK/AddVK when creating, RemoveNK/RemoveVK when deleting
//   - Read phase: GetNK/GetVK for lookups (can use ReadOnlyIndex for type safety)
type Index interface {
	ReadOnlyIndex

	// AddNK registers an NK cell.
	// parentOff is the parent NK's offset (0 for root).
	// name is the NK's name (already decoded from NK.Name()).
	// offset is this NK's cell offset.
	AddNK(parentOff uint32, name string, offset uint32)

	// AddVK registers a VK cell.
	// parentOff is the parent NK's offset.
	// valueName is the VK's name (empty "" for (Default)).
	// offset is this VK's cell offset.
	AddVK(parentOff uint32, valueName string, offset uint32)

	// AddNKLower registers an NK cell with a pre-lowercased name.
	// Use this when the name is already lowercased (e.g., from subkeys.Read())
	// to avoid redundant strings.ToLower() calls in the hot path.
	// parentOff is the parent NK's offset (0 for root).
	// nameLower is the NK's name, already lowercased.
	// offset is this NK's cell offset.
	AddNKLower(parentOff uint32, nameLower string, offset uint32)

	// AddVKLower registers a VK cell with a pre-lowercased name.
	// Use this when the value name is already lowercased to avoid
	// redundant strings.ToLower() calls in the hot path.
	// parentOff is the parent NK's offset.
	// valueNameLower is the VK's name, already lowercased.
	// offset is this VK's cell offset.
	AddVKLower(parentOff uint32, valueNameLower string, offset uint32)

	// RemoveNK removes an NK entry from the index.
	// parentOff is the parent NK's offset (0 for root).
	// name is the NK's name (case-insensitive for StringIndex).
	// Safe to call even if the entry doesn't exist.
	RemoveNK(parentOff uint32, name string)

	// RemoveVK removes a VK entry from the index.
	// parentOff is the parent NK's offset.
	// valueName is the VK's name (empty "" for (Default)).
	// Safe to call even if the entry doesn't exist.
	RemoveVK(parentOff uint32, valueName string)
}

// Builder is an alias for Index, maintained for backward compatibility.
// Prefer using Index directly in new code.
//
// Deprecated: Use Index instead. This alias will be removed in a future version.
type Builder = Index

// ReadWriteIndex is an alias for Index, maintained for backward compatibility.
// Prefer using Index directly in new code.
//
// Deprecated: Use Index instead. This alias will be removed in a future version.
type ReadWriteIndex = Index

// Stats reports index metrics.
type Stats struct {
	NKCount     int    // Number of NK entries
	VKCount     int    // Number of VK entries
	BytesApprox int    // Approximate memory usage (best effort)
	Impl        string // Implementation name
}
