package walker

import (
	"testing"
)

// TestAcquireBitmap_PoolTypeSafety verifies that acquireBitmap does not panic
// when the bitmap pool contains an unexpected type.
//
// Bug: acquireBitmap used an unchecked type assertion `v.(*Bitmap)` which panicked
// if the pool contained a non-*Bitmap value.
func TestAcquireBitmap_PoolTypeSafety(t *testing.T) {
	// Put an unexpected type into the global pool
	bitmapPool.Put("not a *Bitmap")

	// This should NOT panic — it should fall through to NewBitmap
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("acquireBitmap panicked on unexpected pool type: %v", r)
		}
	}()

	bm := acquireBitmap(4096)
	if bm == nil {
		t.Fatal("acquireBitmap returned nil")
	}
	if bm.size < 4096 {
		t.Errorf("bitmap size %d < requested 4096", bm.size)
	}
}
