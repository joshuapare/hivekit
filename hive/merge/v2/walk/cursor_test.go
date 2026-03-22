package walk

import "testing"

func TestCursorStack_PushPop(t *testing.T) {
	s := NewCursorStack(4)

	if s.Depth() != 0 {
		t.Fatalf("expected depth 0, got %d", s.Depth())
	}

	s.Push(CursorEntry{NKCellIdx: 100, SKCellIdx: 10})
	s.Push(CursorEntry{NKCellIdx: 200, SKCellIdx: 20})
	s.Push(CursorEntry{NKCellIdx: 300, SKCellIdx: 30})

	if s.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", s.Depth())
	}

	// Peek should return the top entry without removing it.
	top := s.Peek()
	if top == nil {
		t.Fatal("Peek returned nil")
	}
	if top.NKCellIdx != 300 {
		t.Errorf("Peek: expected NKCellIdx 300, got %d", top.NKCellIdx)
	}
	if s.Depth() != 3 {
		t.Errorf("Peek should not change depth, got %d", s.Depth())
	}

	// Pop in LIFO order.
	e := s.Pop()
	if e.NKCellIdx != 300 || e.SKCellIdx != 30 {
		t.Errorf("Pop 1: expected {300,30}, got {%d,%d}", e.NKCellIdx, e.SKCellIdx)
	}

	e = s.Pop()
	if e.NKCellIdx != 200 || e.SKCellIdx != 20 {
		t.Errorf("Pop 2: expected {200,20}, got {%d,%d}", e.NKCellIdx, e.SKCellIdx)
	}

	e = s.Pop()
	if e.NKCellIdx != 100 || e.SKCellIdx != 10 {
		t.Errorf("Pop 3: expected {100,10}, got {%d,%d}", e.NKCellIdx, e.SKCellIdx)
	}

	if s.Depth() != 0 {
		t.Errorf("expected depth 0 after all pops, got %d", s.Depth())
	}
}

func TestCursorStack_Empty(t *testing.T) {
	s := NewCursorStack(8)

	if s.Depth() != 0 {
		t.Fatalf("new stack should have depth 0, got %d", s.Depth())
	}

	if p := s.Peek(); p != nil {
		t.Errorf("Peek on empty stack should return nil, got %+v", p)
	}
}

func TestCursorStack_GrowsBeyondInitialCapacity(t *testing.T) {
	s := NewCursorStack(2)

	// Push more entries than the initial capacity.
	s.Push(CursorEntry{NKCellIdx: 1})
	s.Push(CursorEntry{NKCellIdx: 2})
	s.Push(CursorEntry{NKCellIdx: 3}) // exceeds initial maxDepth

	if s.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", s.Depth())
	}

	e := s.Pop()
	if e.NKCellIdx != 3 {
		t.Errorf("expected NKCellIdx 3, got %d", e.NKCellIdx)
	}
}
