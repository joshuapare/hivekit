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
// SK layout (_CM_KEY_SECURITY):
//
//	Offset  Size  Description
//	0x00    2     's' 'k' signature
//	0x02    2     Reserved (unused)
//	0x04    4     Flink - forward link in security descriptor list
//	0x08    4     Blink - backward link in security descriptor list
//	0x0C    4     ReferenceCount - number of keys using this descriptor
//	0x10    4     DescriptorLength - length of descriptor data in bytes
//	0x14    ...   Descriptor - SECURITY_DESCRIPTOR_RELATIVE data (inline)
func DecodeSK(b []byte, cellOff int) (int, int, error) {
	if len(b) < SKMinSize {
		return 0, 0, fmt.Errorf("sk: %w", ErrTruncated)
	}
	if !bytes.Equal(b[:SignatureSize], SKSignature) {
		return 0, 0, fmt.Errorf("sk: %w", ErrSignatureMismatch)
	}
	// Descriptor length is at offset 0x10
	length := int(buf.U32LE(b[SKDescriptorLengthOffset:]))
	if length < 0 {
		return 0, 0, errors.New("sk: negative descriptor length")
	}
	// Descriptor data starts inline at offset 0x14
	startAbs := cellOff + SKDescriptorOffset
	end := startAbs + length
	if end > cellOff+len(b) {
		return 0, 0, fmt.Errorf("sk: %w", ErrTruncated)
	}
	return startAbs, length, nil
}
