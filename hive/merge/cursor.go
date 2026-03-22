package merge

// cursorEntry caches parsed NK cell data and subkey list data at one tree level.
// When the DFS traversal ascends from one child and descends to a sibling,
// the parent's cached data avoids redundant hive reads.
type cursorEntry struct {
	nkCellIdx   uint32 // cell index of the NK at this level
	lhListRef   uint32 // cell index of the subkey list (for sibling reuse)
	lhListData  []byte // raw subkey list bytes, cached for sibling lookups
	subkeyCount uint32 // number of subkeys from the NK header
	dirty       bool   // true if ops modified this NK (needs re-resolve)
}

// cursorStack is a LIFO stack of cursorEntry values, one per tree level
// in the DFS walk. It avoids re-parsing parent NK cells when ascending
// from one child and descending to a sibling.
type cursorStack struct {
	entries []cursorEntry
	top     int // number of entries on the stack (0 = empty)
}

// newCursorStack creates a cursor stack with the given initial capacity.
// The stack grows dynamically via append if depth exceeds capacity.
func newCursorStack(initialCap int) *cursorStack {
	return &cursorStack{
		entries: make([]cursorEntry, 0, initialCap),
		top:     0,
	}
}

// push adds an entry to the top of the stack.
func (cs *cursorStack) push(e cursorEntry) {
	if cs.top < len(cs.entries) {
		cs.entries[cs.top] = e
	} else {
		cs.entries = append(cs.entries, e)
	}
	cs.top++
}

// pop removes and returns the top entry. Returns zero value if empty.
func (cs *cursorStack) pop() cursorEntry {
	if cs.top == 0 {
		return cursorEntry{}
	}
	cs.top--
	e := cs.entries[cs.top]
	// Zero out to release lhListData slice for GC
	cs.entries[cs.top] = cursorEntry{}
	return e
}

// peek returns a copy of the top entry without removing it.
// Returns zero value if empty.
func (cs *cursorStack) peek() cursorEntry {
	if cs.top == 0 {
		return cursorEntry{}
	}
	return cs.entries[cs.top-1]
}

// peekPtr returns a pointer to the top entry, allowing in-place modification.
// Returns nil if the stack is empty.
func (cs *cursorStack) peekPtr() *cursorEntry {
	if cs.top == 0 {
		return nil
	}
	return &cs.entries[cs.top-1]
}

// depth returns the number of entries on the stack.
func (cs *cursorStack) depth() int {
	return cs.top
}

// evict removes the top entry entirely, used after key deletion
// to discard stale cached data. Equivalent to pop but discards the result.
func (cs *cursorStack) evict() {
	if cs.top == 0 {
		return
	}
	cs.top--
	// Zero out to release lhListData slice for GC
	cs.entries[cs.top] = cursorEntry{}
}
