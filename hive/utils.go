package hive

import (
	"errors"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- Small helpers (no allocations) ---

func u32(b []byte, off int) uint32 {
	return format.ReadU32(b, off)
}

func hasPrefix(b []byte, sig []byte) bool {
	return len(b) >= format.IdxMinHeader &&
		b[format.IdxSignatureOffset] == sig[0] &&
		b[format.IdxSignatureOffset+1] == sig[1]
}

func checkIndexHeader(b []byte) (uint16, error) {
	if len(b) < format.IdxMinHeader {
		return 0, errors.New("subkey index: truncated header")
	}
	return format.ReadU16(b, format.IdxCountOffset), nil
}
