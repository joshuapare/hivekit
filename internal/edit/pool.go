package edit

import "sync"

// bufferPool provides reusable byte buffers to reduce allocations during commit operations.
// Using a pool reduces per-operation memory overhead from ~80KB to ~1-2KB while maintaining
// or slightly improving performance due to reduced GC pressure.
var bufferPool = sync.Pool{
	New: func() interface{} {
		// Allocate 128 KB buffers (generous for most hives)
		// Small hives: ~8 KB, so 128 KB provides plenty of headroom
		// Medium hives: ~8-100 KB
		// Large hives: May need to grow beyond this, but most operations are on small/medium
		buf := make([]byte, 128*1024)
		return &buf
	},
}

// getBuffer retrieves a buffer from the pool.
// The buffer is at least 128 KB but may be larger if previously grown.
func getBuffer() *[]byte {
	buf, ok := bufferPool.Get().(*[]byte)
	if !ok {
		panic("bufferPool returned unexpected type")
	}
	return buf
}

// putBuffer returns a buffer to the pool for reuse.
// Buffers larger than 1 MB are not returned to avoid keeping excessive memory.
func putBuffer(buf *[]byte) {
	if buf == nil {
		return
	}

	// Don't pool excessively large buffers to avoid memory bloat
	// If a large hive grows the buffer to >1MB, let GC reclaim it
	if cap(*buf) > 1024*1024 {
		return
	}

	// Reset length to capacity to allow reuse of full buffer
	*buf = (*buf)[:cap(*buf)]

	bufferPool.Put(buf)
}

// ensureCapacity ensures the buffer has at least the specified capacity.
// If the buffer is too small, it is grown and the pointer is updated.
func ensureCapacity(buf *[]byte, minCapacity int) {
	if cap(*buf) >= minCapacity {
		return
	}

	// Grow by at least 50% to amortize growth cost
	newCap := cap(*buf) * 3 / 2
	if newCap < minCapacity {
		newCap = minCapacity
	}

	newBuf := make([]byte, newCap)
	copy(newBuf, *buf)
	*buf = newBuf
}
