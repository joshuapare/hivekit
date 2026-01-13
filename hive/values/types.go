package values

// List represents a value list containing VK (value key) references.
// The order of VKRefs is preserved from the hive.
type List struct {
	VKRefs []uint32 // HCELL_INDEX references to VK cells
}

// Len returns the number of values in the list.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.VKRefs)
}

// Append adds a VK reference to the end of the list.
func (l *List) Append(vkRef uint32) *List {
	if l == nil {
		return &List{VKRefs: []uint32{vkRef}}
	}

	newRefs := make([]uint32, len(l.VKRefs)+1)
	copy(newRefs, l.VKRefs)
	newRefs[len(l.VKRefs)] = vkRef

	return &List{VKRefs: newRefs}
}

// Remove removes the first occurrence of vkRef from the list.
// Returns a new list with the value removed, or the same list if not found.
func (l *List) Remove(vkRef uint32) *List {
	if l == nil {
		return nil
	}

	// Find the reference
	index := -1
	for i, ref := range l.VKRefs {
		if ref == vkRef {
			index = i
			break
		}
	}

	if index == -1 {
		// Not found - return same list
		return l
	}

	// Create new list without the element
	newRefs := make([]uint32, 0, len(l.VKRefs)-1)
	newRefs = append(newRefs, l.VKRefs[:index]...)
	newRefs = append(newRefs, l.VKRefs[index+1:]...)

	return &List{VKRefs: newRefs}
}

// Find searches for a VK reference in the list.
// Returns the index if found, or -1 if not found.
func (l *List) Find(vkRef uint32) int {
	if l == nil {
		return -1
	}

	for i, ref := range l.VKRefs {
		if ref == vkRef {
			return i
		}
	}

	return -1
}
