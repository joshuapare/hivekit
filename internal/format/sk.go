package format

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// DecodeSK returns the absolute offset (relative to the hive buffer) and length
// of the security descriptor stored in an SK cell. Many tools simply copy this
// region verbatim, so we expose it without attempting to parse the ACL.
//
// SK layout (observed empirically):
//
//	Offset  Size  Description
//	0x00    2     's' 'k'
//	0x02    2     Revision
//	0x04    4     Descriptor length in bytes
//	0x08    4     Control flags / ref count (unused here)
//	0x0C    4     Offset to security descriptor relative to cell start
//	0x10    4     Reserved
//	0x14    ...   SECURITY_DESCRIPTOR data
func DecodeSK(b []byte, cellOff int) (start, n int, err error) {
	if len(b) < SKMinSize {
		return 0, 0, fmt.Errorf("sk: %w", ErrTruncated)
	}
	if !bytes.Equal(b[:SignatureSize], SKSignature) {
		return 0, 0, fmt.Errorf("sk: %w", ErrSignatureMismatch)
	}
	length := int(buf.U32LE(b[SKLengthOffset:]))
	offset := int(buf.U32LE(b[SKDescOffsetField:]))
	if length < 0 || offset < 0 {
		return 0, 0, errors.New("sk: negative length/offset")
	}
	startAbs := cellOff + offset
	end := startAbs + length
	if end > cellOff+len(b) {
		return 0, 0, fmt.Errorf("sk: %w", ErrTruncated)
	}
	return startAbs, length, nil
}
