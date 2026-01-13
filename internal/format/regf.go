package format

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// Header captures the minimal subset of the REGF header required to traverse a
// types. The diagram below highlights the offsets we care about.
//
//	Offset  Size  Description
//	------  ----  ----------------------------------------------------------
//	 0x000   4    'r' 'e' 'g' 'f'
//	 0x004   4    Primary sequence number
//	 0x008   4    Secondary sequence number
//	 0x00C   8    Last write timestamp (FILETIME)
//	 0x014   4    Major version
//	 0x018   4    Minor version
//	 0x01C   4    Type (0 = primary, 1 = alternate)
//	 0x024   4    Offset (relative to first HBIN) of the root cell (NK)
//	 0x028   4    Total size of HBIN data
//	 0x02C   4    Clustering factor (rarely used)
//
// Windows stores the header in little-endian form.
type Header struct {
	PrimarySequence   uint32
	SecondarySequence uint32
	LastWriteRaw      uint64
	MajorVersion      uint32
	MinorVersion      uint32
	Type              uint32
	RootCellOffset    uint32
	HiveBinsDataSize  uint32
	ClusteringFactor  uint32
}

// ParseHeader validates and extracts key fields from a REGF header.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("regf header: %w", ErrTruncated)
	}
	if !bytes.Equal(b[:REGFSignatureSize], REGFSignature) {
		return Header{}, fmt.Errorf("regf header: %w", ErrSignatureMismatch)
	}
	pseq := buf.U32LE(b[REGFPrimarySeqOffset:])
	sseq := buf.U32LE(b[REGFSecondarySeqOffset:])
	lastWrite := buf.U64LE(b[REGFTimeStampOffset:])
	major := buf.U32LE(b[REGFMajorVersionOffset:])
	minor := buf.U32LE(b[REGFMinorVersionOffset:])
	hType := buf.U32LE(b[REGFTypeOffset:])
	rootOff := buf.U32LE(b[REGFRootCellOffset:])
	hbinsSize := buf.U32LE(b[REGFDataSizeOffset:])
	cluster := buf.U32LE(b[REGFClusterOffset:])
	return Header{
		PrimarySequence:   pseq,
		SecondarySequence: sseq,
		LastWriteRaw:      lastWrite,
		MajorVersion:      major,
		MinorVersion:      minor,
		Type:              hType,
		RootCellOffset:    rootOff,
		HiveBinsDataSize:  hbinsSize,
		ClusteringFactor:  cluster,
	}, nil
}
