package hive

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// -----------------------------------------------------------------------------
// test helpers
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// 1) Happy path: 2 cells → EOF
// -----------------------------------------------------------------------------.
func TestCellIter_HappyPathTwoCellsThenEOF(t *testing.T) {
	hb := buildHBINFromSpec(t, []CellSpec{
		{
			Allocated: true,
			Size:      0x30,
			Payload:   []byte("nk"), // first 2 bytes → "nk"
		},
		{
			Allocated: false,
			Size:      0x18,
		},
	})

	h := &HBIN{
		Data:   hb,
		Offset: 0x1000,
		Size:   uint32(len(hb)),
	}

	it := h.Cells()

	// cell #1
	c1, err := it.Next()
	require.NoError(t, err)
	require.True(t, c1.IsAllocated())
	require.Equal(t, 0x30, c1.SizeAbs())
	require.Equal(t, "nk", string(c1.Signature2()))

	// cell #2
	c2, err := it.Next()
	require.NoError(t, err)
	require.False(t, c2.IsAllocated())
	require.Equal(t, 0x18, c2.SizeAbs())

	// end → EOF
	_, err = it.Next()
	require.ErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 2) Truncated in the middle of second cell → first OK, second = EOF
// -----------------------------------------------------------------------------.
func TestCellIter_TruncatedInSecondCell_ShouldEOF(t *testing.T) {
	// start from a good HBIN
	hb := buildHBINFromSpec(t, []CellSpec{
		{Allocated: true, Size: 0x30, Payload: []byte("nk")},
		{Allocated: false, Size: 0x18},
	})

	// now chop in the middle of where 2nd cell would be
	// header (0x20) + first cell (0x30) = 0x50
	// let's chop at 0x58 → not enough for next header
	hb = hb[:0x58]

	h := &HBIN{
		Data:   hb,
		Offset: 0x1000,
		Size:   uint32(len(hb)),
	}

	it := h.Cells()

	// first cell still OK
	_, err := it.Next()
	require.NoError(t, err)

	// second → EOF (not random error)
	_, err = it.Next()
	require.ErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 3) HBIN has ONLY header → first Next = EOF
// -----------------------------------------------------------------------------.
func TestCellIter_HeaderOnly_ShouldEOFImmediately(t *testing.T) {
	hb := buildHBINFromSpec(t, nil)

	// keep just the header
	hb = hb[:format.HBINHeaderSize]

	h := &HBIN{
		Data:   hb,
		Offset: 0x1000,
		Size:   uint32(len(hb)),
	}
	it := h.Cells()

	_, err := it.Next()
	require.ErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 4) Corrupt: first cell size = 0 → error (NOT EOF)
// -----------------------------------------------------------------------------.
func TestCellIter_CorruptCellSizeZero_ShouldError(t *testing.T) {
	hb := buildHBINFromSpec(t, []CellSpec{
		{Allocated: true, Size: 0x30, Payload: []byte("nk")},
	})

	// overwrite the size at the first cell to 0
	format.PutI32(hb, int(format.HBINHeaderSize), 0)

	h := &HBIN{
		Data:   hb,
		Offset: 0x1000,
		Size:   uint32(len(hb)),
	}
	it := h.Cells()

	_, err := it.Next()
	require.Error(t, err)
	require.NotErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 5) Corrupt: first cell claims to be bigger than the HBIN → error (NOT EOF)
// -----------------------------------------------------------------------------.
func TestCellIter_CorruptCellSizeBeyondHBIN_ShouldError(t *testing.T) {
	hb := buildHBINFromSpec(t, []CellSpec{
		{Allocated: true, Size: 0x30, Payload: []byte("nk")},
	})

	// make that first cell absurdly large
	format.PutI32(hb, int(format.HBINHeaderSize), int32(0x7000)) // > 4096

	h := &HBIN{
		Data:   hb,
		Offset: 0x1000,
		Size:   uint32(len(hb)),
	}
	it := h.Cells()

	_, err := it.Next()
	require.Error(t, err)
	require.NotErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 6) Misaligned cell that forces iterator to notice end and EOF
// -----------------------------------------------------------------------------.
func TestCellIter_MisalignedNextCell_ShouldEOF(t *testing.T) {
	// we'll make a first cell that is a weird (unaligned) size ON PURPOSE
	// so the iterator aligns forward and hits the end.
	hb := buildHBINFromSpec(t, []CellSpec{
		{
			Allocated: true,
			Size:      0x23,            // intentionally weird
			Payload:   []byte("nk..."), // whatever
		},
	})

	h := &HBIN{Data: hb, Offset: 0x1000, Size: uint32(len(hb))}
	it := h.Cells()

	// first cell should still be returned
	_, err := it.Next()
	require.NoError(t, err)

	// next should EOF (we advanced + aligned past HBIN payload)
	_, err = it.Next()
	require.ErrorIs(t, err, io.EOF)
}

// -----------------------------------------------------------------------------
// 7) Exactly ends on a cell boundary → final Next = EOF
// -----------------------------------------------------------------------------.
func TestCellIter_ExactlyEndsOnCellBoundary(t *testing.T) {
	// make 2 cells that clearly fit, like in happy path
	hb := buildHBINFromSpec(t, []CellSpec{
		{Allocated: true, Size: 0x30, Payload: []byte("nk")},
		{Allocated: false, Size: 0x18},
	})

	// now figure out where that second cell ends and slice there
	tmpHB := &HBIN{Data: hb, Offset: 0x1000, Size: uint32(len(hb))}
	tmpIter := tmpHB.Cells()

	c1, err := tmpIter.Next()
	require.NoError(t, err)
	_ = c1

	c2, err := tmpIter.Next()
	require.NoError(t, err)

	end := c2.Off + c2.SizeAbs()
	if rem := end % format.CellAlignment; rem != 0 {
		end += format.CellAlignment - rem
	}

	hb = hb[:end]

	h := &HBIN{Data: hb, Offset: 0x1000, Size: uint32(len(hb))}
	it := h.Cells()

	_, err = it.Next()
	require.NoError(t, err)
	_, err = it.Next()
	require.NoError(t, err)
	_, err = it.Next()
	require.ErrorIs(t, err, io.EOF)
}
