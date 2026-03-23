package walk

// CursorEntry holds the hive state for a single level during the annotation walk.
// Each entry captures the resolved NK cell and its subkey list so that child
// lookups can be performed without re-resolving the parent.
type CursorEntry struct {
	NKCellIdx   uint32 // relative cell offset of the NK cell
	NKPayload   []byte // raw NK payload (valid for entire walk; hive is read-only)
	ListRef     uint32 // subkey list cell offset (from NK)
	ListPayload []byte // raw subkey list bytes
	SubkeyCount uint32 // number of subkeys (from NK)
	SKCellIdx   uint32 // security descriptor cell offset (for inheritance)
}

// CursorStack is a pre-allocated stack of CursorEntry used to track the
// current path during a depth-first annotation walk. It avoids heap
// allocations by using a fixed-capacity slice.
type CursorStack struct {
	entries []CursorEntry
	top     int // index of next free slot (== current depth)
}

// NewCursorStack allocates a CursorStack with room for maxDepth levels.
func NewCursorStack(maxDepth int) *CursorStack {
	return &CursorStack{
		entries: make([]CursorEntry, maxDepth),
		top:     0,
	}
}

// Push adds an entry to the top of the stack.
func (s *CursorStack) Push(e CursorEntry) {
	if s.top < len(s.entries) {
		s.entries[s.top] = e
	} else {
		s.entries = append(s.entries, e)
	}
	s.top++
}

// Pop removes and returns the top entry. Caller must ensure the stack is not empty.
func (s *CursorStack) Pop() CursorEntry {
	s.top--
	e := s.entries[s.top]
	// Zero the slot to release any referenced byte slices.
	s.entries[s.top] = CursorEntry{}
	return e
}

// Peek returns a pointer to the top entry without removing it.
// Returns nil if the stack is empty.
func (s *CursorStack) Peek() *CursorEntry {
	if s.top == 0 {
		return nil
	}
	return &s.entries[s.top-1]
}

// Depth returns the current number of entries on the stack.
func (s *CursorStack) Depth() int {
	return s.top
}
