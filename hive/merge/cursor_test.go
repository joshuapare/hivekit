package merge

import "testing"

func TestCursorStack_PushPeek(t *testing.T) {
	cs := newCursorStack(8)

	cs.push(cursorEntry{nkCellIdx: 100, subkeyCount: 5})
	cs.push(cursorEntry{nkCellIdx: 200, subkeyCount: 3})

	got := cs.peek()
	if got.nkCellIdx != 200 {
		t.Errorf("peek: want nkCellIdx=200, got %d", got.nkCellIdx)
	}
	if got.subkeyCount != 3 {
		t.Errorf("peek: want subkeyCount=3, got %d", got.subkeyCount)
	}
	if cs.depth() != 2 {
		t.Errorf("depth: want 2, got %d", cs.depth())
	}
}

func TestCursorStack_PushPop(t *testing.T) {
	cs := newCursorStack(4)

	cs.push(cursorEntry{nkCellIdx: 10})
	cs.push(cursorEntry{nkCellIdx: 20})
	cs.push(cursorEntry{nkCellIdx: 30})

	got := cs.pop()
	if got.nkCellIdx != 30 {
		t.Errorf("pop: want nkCellIdx=30, got %d", got.nkCellIdx)
	}
	if cs.depth() != 2 {
		t.Errorf("depth after pop: want 2, got %d", cs.depth())
	}

	got = cs.pop()
	if got.nkCellIdx != 20 {
		t.Errorf("pop: want nkCellIdx=20, got %d", got.nkCellIdx)
	}
	if cs.depth() != 1 {
		t.Errorf("depth after second pop: want 1, got %d", cs.depth())
	}
}

func TestCursorStack_Depth(t *testing.T) {
	cs := newCursorStack(4)

	if cs.depth() != 0 {
		t.Errorf("empty stack depth: want 0, got %d", cs.depth())
	}

	cs.push(cursorEntry{nkCellIdx: 1})
	if cs.depth() != 1 {
		t.Errorf("after push: want 1, got %d", cs.depth())
	}

	cs.push(cursorEntry{nkCellIdx: 2})
	cs.push(cursorEntry{nkCellIdx: 3})
	if cs.depth() != 3 {
		t.Errorf("after 3 pushes: want 3, got %d", cs.depth())
	}

	cs.pop()
	if cs.depth() != 2 {
		t.Errorf("after pop: want 2, got %d", cs.depth())
	}
}

func TestCursorStack_Evict(t *testing.T) {
	cs := newCursorStack(4)

	cs.push(cursorEntry{nkCellIdx: 10})
	cs.push(cursorEntry{nkCellIdx: 20})
	cs.push(cursorEntry{nkCellIdx: 30})

	// Evict removes the top entry entirely (used after key deletion)
	cs.evict()
	if cs.depth() != 2 {
		t.Errorf("depth after evict: want 2, got %d", cs.depth())
	}

	got := cs.peek()
	if got.nkCellIdx != 20 {
		t.Errorf("peek after evict: want nkCellIdx=20, got %d", got.nkCellIdx)
	}
}

func TestCursorStack_EmptyOps(t *testing.T) {
	cs := newCursorStack(4)

	// Pop on empty stack returns zero entry
	got := cs.pop()
	if got.nkCellIdx != 0 {
		t.Errorf("pop empty: want zero entry, got nkCellIdx=%d", got.nkCellIdx)
	}

	// Peek on empty stack returns zero entry
	got = cs.peek()
	if got.nkCellIdx != 0 {
		t.Errorf("peek empty: want zero entry, got nkCellIdx=%d", got.nkCellIdx)
	}

	// Evict on empty stack is a no-op
	cs.evict() // should not panic
	if cs.depth() != 0 {
		t.Errorf("depth after evict on empty: want 0, got %d", cs.depth())
	}
}

func TestCursorStack_DirtyFlag(t *testing.T) {
	cs := newCursorStack(4)

	cs.push(cursorEntry{nkCellIdx: 100, dirty: false})
	got := cs.peek()
	if got.dirty {
		t.Error("dirty: want false initially")
	}

	// Modify the top entry through peekPtr
	ptr := cs.peekPtr()
	ptr.dirty = true

	got = cs.peek()
	if !got.dirty {
		t.Error("dirty: want true after setting via peekPtr")
	}
}

func TestCursorStack_LHListData(t *testing.T) {
	cs := newCursorStack(4)

	testData := []byte{0x6c, 0x68, 0x02, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	cs.push(cursorEntry{
		nkCellIdx:   100,
		lhListRef:   0x1000,
		lhListData:  testData,
		subkeyCount: 2,
	})

	got := cs.peek()
	if got.lhListRef != 0x1000 {
		t.Errorf("lhListRef: want 0x1000, got 0x%X", got.lhListRef)
	}
	if len(got.lhListData) != len(testData) {
		t.Errorf("lhListData length: want %d, got %d", len(testData), len(got.lhListData))
	}
	for i, b := range testData {
		if got.lhListData[i] != b {
			t.Errorf("lhListData[%d]: want 0x%02X, got 0x%02X", i, b, got.lhListData[i])
		}
	}
}

func TestCursorStack_GrowsBeyondInitialCapacity(t *testing.T) {
	cs := newCursorStack(2) // small initial capacity

	for i := uint32(0); i < 10; i++ {
		cs.push(cursorEntry{nkCellIdx: i})
	}

	if cs.depth() != 10 {
		t.Errorf("depth after 10 pushes: want 10, got %d", cs.depth())
	}

	// Pop all and verify LIFO order
	for i := uint32(9); ; i-- {
		got := cs.pop()
		if got.nkCellIdx != i {
			t.Errorf("pop order: want nkCellIdx=%d, got %d", i, got.nkCellIdx)
		}
		if i == 0 {
			break
		}
	}

	if cs.depth() != 0 {
		t.Errorf("depth after all pops: want 0, got %d", cs.depth())
	}
}
