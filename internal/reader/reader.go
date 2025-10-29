// Package reader provides the concrete types.Reader implementation. The
// exported entry points are used by the public wrapper (e.g., the reader
// package or CLI) to obtain a types.Reader without exposing the internal
// parsing machinery directly.
package reader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/mmfile"
	"github.com/joshuapare/hivekit/pkg/types"
	"golang.org/x/text/encoding/charmap"
)

// Open maps the hive at path and returns an implementation of types.Reader.
func Open(path string, opts types.OpenOptions) (types.Reader, error) {
	data, unmap, err := mmfile.Map(path)
	if err != nil {
		return nil, wrapIOErr(fmt.Errorf("open hive: %w", err))
	}
	r, err := newReader(data, unmap, opts)
	if err != nil {
		if unmap != nil {
			_ = unmap()
		}
		return nil, err
	}
	return r, nil
}

// OpenBytes creates a reader backed by the provided buffer.
func OpenBytes(buf []byte, opts types.OpenOptions) (types.Reader, error) {
	return newReader(buf, nil, opts)
}

type reader struct {
	buf            []byte
	unmap          func() error
	opts           types.OpenOptions
	head           format.Header
	closed         bool
	validatedHBINs map[uint32]bool        // HBIN offset -> validated (populated at Open)
	hbinIndex      []hbinIndexEntry       // Fast HBIN lookup index (built at Open)
	diagnostics    *diagnosticCollector   // nil unless CollectDiagnostics=true (zero-cost)
}

// hbinIndexEntry stores HBIN position for fast lookup
type hbinIndexEntry struct {
	offset int // Absolute offset in file (including REGF header)
	size   int // Total HBIN size including header
}

func newReader(buf []byte, unmap func() error, opts types.OpenOptions) (types.Reader, error) {
	head, err := format.ParseHeader(buf)
	if err != nil {
		return nil, wrapFormatErr(err)
	}
	if opts.MaxCellSize <= 0 {
		opts.MaxCellSize = 64 << 20 // default 64 MiB safeguard
	}

	r := &reader{
		buf:   buf,
		unmap: unmap,
		opts:  opts,
		head:  head,
	}

	// Initialize diagnostic collector if requested (zero-cost if not)
	if opts.CollectDiagnostics {
		r.diagnostics = newDiagnosticCollector()
	}

	// Validate all HBIN structures at open time (Option A: structural boundary)
	// This provides clear contract: "Open succeeds = structure is sound"
	if err := r.validateAllHBINs(); err != nil {
		return nil, err
	}

	return r, nil
}

// Close releases resources (unmaps the buffer if necessary).
func (r *reader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.unmap != nil {
		return r.unmap()
	}
	return nil
}

// BaseBuffer returns the underlying buffer for zero-copy operations.
// The returned buffer must not be modified and is only valid until Close() is called.
// This is used by the edit package to build ASTs with zero-copy value references.
func (r *reader) BaseBuffer() []byte {
	return r.buf
}

func (r *reader) ensureOpen() error {
	if r.closed {
		return &types.Error{Kind: types.ErrKindState, Msg: "reader is closed"}
	}
	return nil
}

func (r *reader) Info() types.HiveInfo {
	return types.HiveInfo{
		PrimarySequence:   r.head.PrimarySequence,
		SecondarySequence: r.head.SecondarySequence,
		LastWrite:         format.FiletimeToTime(r.head.LastWriteRaw),
		MajorVersion:      r.head.MajorVersion,
		MinorVersion:      r.head.MinorVersion,
		Type:              r.head.Type,
		RootCellOffset:    r.head.RootCellOffset,
		HiveBinsDataSize:  r.head.HiveBinsDataSize,
		ClusteringFactor:  r.head.ClusteringFactor,
	}
}

func (r *reader) Root() (types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	return types.NodeID(r.head.RootCellOffset), nil
}

func (r *reader) StatKey(id types.NodeID) (types.KeyMeta, error) {
	if err := r.ensureOpen(); err != nil {
		return types.KeyMeta{}, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return types.KeyMeta{}, err
	}
	name, err := DecodeKeyName(nk)
	if err != nil {
		return types.KeyMeta{}, wrapFormatErr(err)
	}
	return types.KeyMeta{
		Name:           name,
		LastWrite:      format.FiletimeToTime(nk.LastWriteRaw),
		SubkeyN:        int(nk.SubkeyCount),
		ValueN:         int(nk.ValueCount),
		HasSecDesc:     nk.SecurityOffset != math.MaxUint32,
		NameCompressed: nk.NameIsCompressed(),
	}, nil
}

func (r *reader) DetailKey(id types.NodeID) (types.KeyDetail, error) {
	if err := r.ensureOpen(); err != nil {
		return types.KeyDetail{}, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return types.KeyDetail{}, err
	}
	name, err := DecodeKeyName(nk)
	if err != nil {
		return types.KeyDetail{}, wrapFormatErr(err)
	}

	// Get class name if present
	className := ""
	if nk.ClassLength > 0 && nk.ClassNameOffset != math.MaxUint32 {
		if classData, err := r.cell(nk.ClassNameOffset); err == nil {
			className = string(classData.Data[:min(len(classData.Data), int(nk.ClassLength))])
		}
	}

	return types.KeyDetail{
		KeyMeta: types.KeyMeta{
			Name:           name,
			LastWrite:      format.FiletimeToTime(nk.LastWriteRaw),
			SubkeyN:        int(nk.SubkeyCount),
			ValueN:         int(nk.ValueCount),
			HasSecDesc:     nk.SecurityOffset != math.MaxUint32,
			NameCompressed: nk.NameIsCompressed(),
		},
		Flags:              nk.Flags,
		ParentOffset:       nk.ParentOffset,
		SubkeyListOffset:   nk.SubkeyListOffset,
		ValueListOffset:    nk.ValueListOffset,
		SecurityOffset:     nk.SecurityOffset,
		ClassNameOffset:    nk.ClassNameOffset,
		MaxNameLength:      nk.MaxNameLength,
		MaxClassLength:     nk.MaxClassLength,
		MaxValueNameLength: nk.MaxValueNameLength,
		MaxValueDataLength: nk.MaxValueDataLength,
		ClassName:          className,
	}, nil
}

// KeyTimestamp returns the LastWrite timestamp for a key without decoding the name.
// This is optimized for single-field access and makes zero allocations.
func (r *reader) KeyTimestamp(id types.NodeID) (time.Time, error) {
	if err := r.ensureOpen(); err != nil {
		return time.Time{}, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return time.Time{}, err
	}
	return format.FiletimeToTime(nk.LastWriteRaw), nil
}

// KeySubkeyCount returns the number of direct child keys without decoding the name.
// This is optimized for single-field access and makes zero allocations.
func (r *reader) KeySubkeyCount(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}
	return int(nk.SubkeyCount), nil
}

// KeyValueCount returns the number of values in a key without decoding the name.
// This is optimized for single-field access and makes zero allocations.
func (r *reader) KeyValueCount(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}
	return int(nk.ValueCount), nil
}

// KeyName returns just the key name without building the full metadata struct.
// This is lighter than StatKey() but still allocates for the string.
func (r *reader) KeyName(id types.NodeID) (string, error) {
	if err := r.ensureOpen(); err != nil {
		return "", err
	}
	nk, err := r.nk(id)
	if err != nil {
		return "", err
	}
	name, err := DecodeKeyName(nk)
	if err != nil {
		return "", wrapFormatErr(err)
	}
	return name, nil
}

func (r *reader) Subkeys(id types.NodeID) ([]types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return nil, err
	}
	if nk.SubkeyCount == 0 || nk.SubkeyListOffset == math.MaxUint32 {
		return nil, nil
	}
	list, err := r.subkeyList(nk.SubkeyListOffset, nk.SubkeyCount)
	if err != nil {
		return nil, err
	}
	out := make([]types.NodeID, len(list))
	for i, off := range list {
		out[i] = types.NodeID(off)
	}
	return out, nil
}

// Lookup finds a direct child key by name (case-insensitive).
// Returns (0, ErrNotFound) if the child doesn't exist.
func (r *reader) Lookup(parent types.NodeID, childName string) (types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	children, err := r.Subkeys(parent)
	if err != nil {
		return 0, err
	}

	// Case-insensitive search
	for _, child := range children {
		name, err := r.KeyName(child)
		if err != nil {
			continue // Skip keys we can't read
		}
		if strings.EqualFold(name, childName) {
			return child, nil
		}
	}

	return 0, &types.Error{
		Kind: types.ErrKindNotFound,
		Msg:  fmt.Sprintf("subkey %q not found", childName),
		Err:  types.ErrNotFound,
	}
}

func (r *reader) Values(id types.NodeID) ([]types.ValueID, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return nil, err
	}
	if nk.ValueCount == 0 || nk.ValueListOffset == math.MaxUint32 {
		return nil, nil
	}
	return r.valueList(nk.ValueListOffset, nk.ValueCount)
}

func (r *reader) StatValue(id types.ValueID) (types.ValueMeta, error) {
	if err := r.ensureOpen(); err != nil {
		return types.ValueMeta{}, err
	}
	// Optimized path: decode VK fields inline without creating VKRecord struct
	// This eliminates intermediate allocations (VKRecord struct + NameRaw slice)
	cell, err := r.cell(uint32(id))
	if err != nil {
		return types.ValueMeta{}, err
	}
	if cell.Size > r.opts.MaxCellSize {
		return types.ValueMeta{}, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value cell exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}

	data := cell.Data
	if len(data) < format.VKMinSize {
		return types.ValueMeta{}, wrapFormatErr(errors.New("vk: truncated"))
	}

	// Parse VK fields directly from buffer (no struct allocation)
	nameLen := binary.LittleEndian.Uint16(data[format.VKNameLenOffset:])
	dataLen := binary.LittleEndian.Uint32(data[format.VKDataLenOffset:])
	valType := binary.LittleEndian.Uint32(data[format.VKTypeOffset:])
	flags := binary.LittleEndian.Uint16(data[format.VKFlagsOffset:])

	// Decode name inline (no NameRaw slice allocation)
	if len(data) < format.VKNameOffset+int(nameLen) {
		return types.ValueMeta{}, wrapFormatErr(errors.New("vk name: truncated"))
	}
	nameBytes := data[format.VKNameOffset : format.VKNameOffset+int(nameLen)]

	var name string
	if flags&format.VKFlagASCIIName != 0 {
		// ASCII/Windows-1252 name - use optimized ASCII fast path
		if isASCII(nameBytes) {
			name = string(nameBytes)
		} else {
			// Extended characters need decoder
			decoded, err := charmap.Windows1252.NewDecoder().Bytes(nameBytes)
			if err != nil {
				return types.ValueMeta{}, wrapFormatErr(fmt.Errorf("failed to decode Windows-1252 value name: %w", err))
			}
			name = string(decoded)
		}
	} else {
		// UTF-16LE name
		if len(nameBytes)%2 != 0 {
			return types.ValueMeta{}, wrapFormatErr(errors.New("vk name has odd length"))
		}
		name = decodeUTF16LE(nameBytes)
	}

	// Calculate size
	size := int(dataLen & format.VKDataLengthMask)
	inline := (dataLen & format.VKDataInlineBit) != 0
	if inline && size > format.OffsetFieldSize {
		size = format.OffsetFieldSize // Inline data stored in OffsetFieldSize-byte DataOffset field
	}

	return types.ValueMeta{
		Name:           name,
		Type:           types.RegType(valType),
		Size:           size,
		Inline:         inline,
		NameCompressed: (flags & format.VKFlagASCIIName) != 0,
	}, nil
}

// ValueType returns the registry type of a value without decoding the name.
// This is optimized for single-field access and makes zero allocations.
func (r *reader) ValueType(id types.ValueID) (types.RegType, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, err
	}
	return types.RegType(vk.Type), nil
}

// ValueName returns the name of a value without fetching full metadata.
// This is lighter than StatValue() as it skips struct construction.
func (r *reader) ValueName(id types.ValueID) (string, error) {
	if err := r.ensureOpen(); err != nil {
		return "", err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return "", err
	}
	name, err := DecodeValueName(vk)
	if err != nil {
		return "", wrapFormatErr(err)
	}
	return name, nil
}

func (r *reader) ValueBytes(id types.ValueID, ro types.ReadOptions) ([]byte, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	vk, data, err := r.value(uint32(id))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	if vk.DataInline() || ro.CopyData || !r.opts.ZeroCopy {
		copyData := make([]byte, len(data))
		copy(copyData, data)
		return copyData, nil
	}
	return data, nil
}

func (r *reader) ValueString(id types.ValueID, ro types.ReadOptions) (string, error) {
	if err := r.ensureOpen(); err != nil {
		return "", err
	}
	vk, data, err := r.value(uint32(id))
	if err != nil {
		return "", err
	}
	switch types.RegType(vk.Type) {
	case types.REG_SZ, types.REG_EXPAND_SZ:
		return DecodeUTF16(data)
	default:
		return "", &types.Error{
			Kind: types.ErrKindType,
			Msg:  "registry value has different type",
			Err:  types.ErrTypeMismatch,
		}
	}
}

func (r *reader) ValueStrings(id types.ValueID, ro types.ReadOptions) ([]string, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}
	vk, data, err := r.value(uint32(id))
	if err != nil {
		return nil, err
	}
	if types.RegType(vk.Type) != types.REG_MULTI_SZ {
		return nil, &types.Error{
			Kind: types.ErrKindType,
			Msg:  "registry value has different type",
			Err:  types.ErrTypeMismatch,
		}
	}
	return DecodeMultiString(data)
}

func (r *reader) ValueDWORD(id types.ValueID) (uint32, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}

	// Fast path: check if data is inline (common for DWORD)
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, err
	}

	// Validate type first
	regType := types.RegType(vk.Type)
	if regType != types.REG_DWORD && regType != types.REG_DWORD_BE {
		return 0, &types.Error{
			Kind: types.ErrKindType,
			Msg:  "registry value has different type",
			Err:  types.ErrTypeMismatch,
		}
	}

	// Fast path: inline data (zero allocations)
	if vk.DataInline() {
		// Validate inline length
		length := int(vk.DataLength & format.VKDataLengthMask)
		if length < format.DWORDSize {
			return 0, &types.Error{
				Kind: types.ErrKindCorrupt,
				Msg:  "value too short for DWORD",
				Err:  types.ErrCorrupt,
			}
		}
		if regType == types.REG_DWORD {
			return vk.DataOffset, nil // Already in little-endian
		}
		// REG_DWORD_BE: need to byte-swap
		return binary.BigEndian.Uint32((*[4]byte)(unsafe.Pointer(&vk.DataOffset))[:]), nil
	}

	// Slow path: external data - must read to validate actual length
	_, data, err := r.value(uint32(id))
	if err != nil {
		return 0, err
	}
	// Check ACTUAL data length (not VK metadata which may be incorrect/truncated)
	if len(data) < format.DWORDSize {
		return 0, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value too short for DWORD",
			Err:  types.ErrCorrupt,
		}
	}
	if regType == types.REG_DWORD {
		return binary.LittleEndian.Uint32(data[:format.DWORDSize]), nil
	}
	return binary.BigEndian.Uint32(data[:format.DWORDSize]), nil
}

func (r *reader) ValueQWORD(id types.ValueID) (uint64, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	vk, data, err := r.value(uint32(id))
	if err != nil {
		return 0, err
	}
	if len(data) < format.QWORDSize {
		return 0, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value too short for QWORD",
			Err:  types.ErrCorrupt,
		}
	}
	if types.RegType(vk.Type) != types.REG_QWORD {
		return 0, &types.Error{
			Kind: types.ErrKindType,
			Msg:  "registry value has different type",
			Err:  types.ErrTypeMismatch,
		}
	}
	return binary.LittleEndian.Uint64(data[:format.QWORDSize]), nil
}

// Internal helpers ----------------------------------------------------------

func (r *reader) nk(id types.NodeID) (format.NKRecord, error) {
	cell, err := r.cell(uint32(id))
	if err != nil {
		return format.NKRecord{}, err
	}
	nk, err := format.DecodeNK(cell.Data)
	if err != nil {
		return format.NKRecord{}, wrapFormatErr(err)
	}
	return nk, nil
}

func (r *reader) subkeyList(offset uint32, expected uint32) ([]uint32, error) {
	cell, err := r.cell(offset)
	if err != nil {
		return nil, err
	}
	if cell.Size > r.opts.MaxCellSize {
		return nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "subkey list exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}

	// Check if this is an RI (indirect) list
	if format.IsRIList(cell.Data) {
		// Decode RI to get offsets to sub-lists (LF/LH)
		subListOffsets, err := format.DecodeRIList(cell.Data)
		if err != nil {
			return nil, wrapFormatErr(err)
		}

		// Estimate capacity: RI lists typically have ~100 entries per sub-list
		// Pre-allocate to reduce reallocations during append
		estimatedCap := len(subListOffsets) * format.RIListEstimatedCapacity
		if expected > 0 && expected < uint32(estimatedCap) {
			estimatedCap = int(expected)
		}
		result := make([]uint32, 0, estimatedCap)

		// Fetch and decode each sub-list, combining results
		for _, subOffset := range subListOffsets {
			subList, err := r.subkeyList(subOffset, 0)
			if err != nil {
				return nil, err
			}
			result = append(result, subList...)
		}
		return result, nil
	}

	// Not RI - decode as LF/LH/LI
	list, err := format.DecodeSubkeyList(cell.Data, expected)
	if err != nil {
		return nil, wrapFormatErr(err)
	}
	return list, nil
}

func (r *reader) valueList(offset uint32, count uint32) ([]types.ValueID, error) {
	cell, err := r.cell(offset)
	if err != nil {
		return nil, err
	}
	if cell.Size > r.opts.MaxCellSize {
		return nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value list exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}
	list, err := format.DecodeValueList(cell.Data, count)
	if err != nil {
		return nil, wrapFormatErr(err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	// Zero-copy conversion: ValueID is type alias for uint32, same memory layout
	return unsafe.Slice((*types.ValueID)(unsafe.Pointer(&list[0])), len(list)), nil
}

// vkOnly reads just the VK record without fetching the data
// Used by StatValue which only needs metadata
func (r *reader) vkOnly(offset uint32) (format.VKRecord, error) {
	cell, err := r.cell(offset)
	if err != nil {
		return format.VKRecord{}, err
	}
	if cell.Size > r.opts.MaxCellSize {
		return format.VKRecord{}, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value cell exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}
	vk, err := format.DecodeVK(cell.Data)
	if err != nil {
		return format.VKRecord{}, wrapFormatErr(err)
	}
	return vk, nil
}

func (r *reader) value(offset uint32) (format.VKRecord, []byte, error) {
	cell, err := r.cell(offset)
	if err != nil {
		return format.VKRecord{}, nil, err
	}
	if cell.Size > r.opts.MaxCellSize {
		return format.VKRecord{}, nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value cell exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}
	vk, err := format.DecodeVK(cell.Data)
	if err != nil {
		return format.VKRecord{}, nil, wrapFormatErr(err)
	}
	length := int(vk.DataLength & format.VKDataLengthMask)
	if length < 0 {
		return format.VKRecord{}, nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "vk length negative",
			Err:  types.ErrCorrupt,
		}
	}
	if vk.DataInline() {
		var buf [format.OffsetFieldSize]byte
		binary.LittleEndian.PutUint32(buf[:], vk.DataOffset)
		if length > len(buf) {
			return format.VKRecord{}, nil, &types.Error{
				Kind: types.ErrKindCorrupt,
				Msg:  "inline length exceeds field",
				Err:  types.ErrCorrupt,
			}
		}
		data := make([]byte, length)
		copy(data, buf[:length])
		return vk, data, nil
	}
	if length == 0 {
		return vk, nil, nil
	}
	dataCell, err := r.cell(vk.DataOffset)
	if err != nil {
		return format.VKRecord{}, nil, err
	}

	// Check if this is a Big Data (db) record for multi-cell large values
	if format.IsDBRecord(dataCell.Data) {
		return r.valueDB(vk, dataCell.Data, length)
	}

	// Normal single-cell data
	if len(dataCell.Data) < length {
		// Record diagnostic for truncated data
		r.recordDiagnostic(diagData(
			types.SevError,
			uint64(format.HeaderSize)+uint64(dataCell.Offset),
			"VK",
			"Value data truncated - length field exceeds available data",
			length,
			len(dataCell.Data),
			nil, // TODO: add context with key path
			&types.RepairAction{
				Type:        types.RepairTruncate,
				Description: fmt.Sprintf("Update VK data length from %d to %d bytes", length, len(dataCell.Data)),
				Confidence:  1.0,
				Risk:        types.RiskLow,
				AutoApply:   true,
			},
		))

		if r.opts.Tolerant {
			// Tolerant mode: return partial data
			length = len(dataCell.Data)
		} else {
			// Strict mode: fail
			return format.VKRecord{}, nil, &types.Error{Kind: types.ErrKindCorrupt, Msg: "value data truncated", Err: types.ErrCorrupt}
		}
	}
	if length > r.opts.MaxCellSize {
		return format.VKRecord{}, nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "value data exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}
	return vk, dataCell.Data[:length], nil
}

// valueDB reads and assembles data from a Big Data (db) record.
// This handles large registry values (>16KB) that are split across multiple cells.
//
// The db record contains a pointer to a blocklist, which in turn contains
// pointers to the actual data blocks. For efficiency:
//   - Pre-allocates the exact buffer size (expectedLen) once
//   - Directly copies block data into the buffer (no intermediate allocations)
//   - Only reads up to expectedLen bytes (blocks may contain more data)
func (r *reader) valueDB(
	vk format.VKRecord,
	dbData []byte,
	expectedLen int,
) (format.VKRecord, []byte, error) {
	// Parse the db record to get the blocklist location
	db, err := format.DecodeDB(dbData)
	if err != nil {
		return format.VKRecord{}, nil, wrapFormatErr(err)
	}

	// Read the blocklist cell
	blocklistCell, err := r.cell(db.BlocklistOffset)
	if err != nil {
		return format.VKRecord{}, nil, fmt.Errorf("db blocklist: %w", err)
	}

	// Parse block offsets from the blocklist (each offset is 4 bytes)
	blockOffsets := make([]uint32, db.NumBlocks)
	for i := uint16(0); i < db.NumBlocks; i++ {
		offset := int(i) * format.OffsetFieldSize
		if offset+format.OffsetFieldSize > len(blocklistCell.Data) {
			return format.VKRecord{}, nil, &types.Error{
				Kind: types.ErrKindCorrupt,
				Msg: fmt.Sprintf(
					"db blocklist truncated: need %d bytes for %d blocks, have %d",
					int(db.NumBlocks)*format.OffsetFieldSize,
					db.NumBlocks,
					len(blocklistCell.Data),
				),
				Err: types.ErrCorrupt,
			}
		}
		// Read little-endian uint32
		blockOffsets[i] = uint32(blocklistCell.Data[offset]) |
			uint32(blocklistCell.Data[offset+1])<<8 |
			uint32(blocklistCell.Data[offset+2])<<16 |
			uint32(blocklistCell.Data[offset+3])<<24
	}

	// Pre-allocate result buffer (single allocation)
	result := make([]byte, expectedLen)
	bytesRead := 0

	// Read and concatenate each data block
	for i, blockOffset := range blockOffsets {
		// Read the data block cell
		blockCell, err := r.cell(blockOffset)
		if err != nil {
			return format.VKRecord{}, nil, fmt.Errorf("db block %d: %w", i, err)
		}

		// DB blocks have 4 bytes of padding at the end (likely the next cell's header)
		// that should not be included in the data. Trim it off.
		blockData := blockCell.Data
		if len(blockData) > format.DBBlockPadding {
			blockData = blockData[:len(blockData)-format.DBBlockPadding]
		}

		// Copy block data, truncating if necessary to fit expectedLen
		bytesAvailable := expectedLen - bytesRead
		if len(blockData) > bytesAvailable {
			blockData = blockData[:bytesAvailable]
		}

		copy(result[bytesRead:], blockData)
		bytesRead += len(blockData)

		// Early exit if we've read all expected data
		if bytesRead >= expectedLen {
			break
		}
	}

	// Verify we read the expected amount
	if bytesRead != expectedLen {
		if r.opts.Tolerant {
			// Return what we got
			return vk, result[:bytesRead], nil
		}
		return format.VKRecord{}, nil, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg: fmt.Sprintf(
				"db data size mismatch: expected %d bytes, got %d",
				expectedLen,
				bytesRead,
			),
			Err: types.ErrCorrupt,
		}
	}

	return vk, result, nil
}

// validateAllHBINs validates all HBIN structures in the hive at open time.
// This implements Option A: structural boundary validation.
// Returns nil if all HBINs are valid, error if any HBIN is corrupt.
func (r *reader) validateAllHBINs() error {
	// Start after REGF header
	offset := int(format.HeaderSize)
	dataEnd := int(format.HeaderSize) + int(r.head.HiveBinsDataSize)

	// Allocate map to track validated HBINs (typical hive has 1-4 HBINs)
	r.validatedHBINs = make(map[uint32]bool, 4)
	// Allocate index for fast HBIN lookup
	r.hbinIndex = make([]hbinIndexEntry, 0, 4)

	// Iterate through all HBINs
	for offset < dataEnd && offset < len(r.buf) {
		// Validate this HBIN
		hbin, next, err := format.NextHBIN(r.buf, offset)
		if err != nil {
			// Record diagnostic for HBIN corruption
			r.recordDiagnostic(diagStructure(
				types.SevCritical,
				uint64(offset),
				"HBIN",
				fmt.Sprintf("HBIN validation failed: %v", err),
				"valid HBIN structure",
				"corrupted or invalid",
				&types.RepairAction{
					Type:        types.RepairReplace,
					Description: "HBIN header corruption - may require manual repair or data recovery",
					Confidence:  0.5,
					Risk:        types.RiskHigh,
					AutoApply:   false,
				},
			))
			return wrapFormatErr(err)
		}

		// Mark HBIN as validated (using HBIN's FileOffset as key)
		r.validatedHBINs[hbin.FileOffset] = true

		// Add to index for fast lookups
		r.hbinIndex = append(r.hbinIndex, hbinIndexEntry{
			offset: offset,
			size:   int(hbin.Size),
		})

		// Move to next HBIN
		offset = next

		// Safety check: prevent infinite loops on malformed data
		if next <= offset-int(hbin.Size) {
			r.recordDiagnostic(diagStructure(
				types.SevCritical,
				uint64(offset),
				"HBIN",
				"HBIN iteration failed to advance - infinite loop detected",
				"offset increase",
				fmt.Sprintf("offset stuck at %d", offset),
				nil,
			))
			return &types.Error{
				Kind: types.ErrKindCorrupt,
				Msg:  "hbin iteration failed to advance",
				Err:  types.ErrCorrupt,
			}
		}
	}

	return nil
}

func (r *reader) cell(offset uint32) (format.Cell, error) {
	// HBIN validation already done at Open() time (Option A: structural boundary)
	// No need for lazy validation here

	abs := int(format.HeaderSize) + int(offset)
	if abs < format.HeaderSize || abs >= len(r.buf) {
		return format.Cell{}, &types.Error{
			Kind: types.ErrKindFormat,
			Msg:  fmt.Sprintf("cell offset %d out of range", offset),
			Err:  types.ErrCorrupt,
		}
	}

	// Read cell data, handling HBIN boundaries
	cellData, err := r.readCellDataAcrossHBINs(abs)
	if err != nil {
		return format.Cell{}, err
	}

	cell, err := format.ParseCell(cellData)
	if err != nil {
		return format.Cell{}, wrapFormatErr(err)
	}
	if cell.Size > r.opts.MaxCellSize {
		return format.Cell{}, &types.Error{
			Kind: types.ErrKindCorrupt,
			Msg:  "cell exceeds MaxCellSize",
			Err:  types.ErrCorrupt,
		}
	}
	return cell, nil
}

// findHBINForOffset finds the HBIN that contains the given absolute offset.
// Returns the HBIN's starting offset and ending offset, or error if not found.
func (r *reader) findHBINForOffset(absOffset int) (hbinStart, hbinEnd int, err error) {
	// Use the index for fast lookup
	for _, entry := range r.hbinIndex {
		if absOffset >= entry.offset && absOffset < entry.offset+entry.size {
			return entry.offset, entry.offset + entry.size, nil
		}
	}
	return 0, 0, &types.Error{
		Kind: types.ErrKindFormat,
		Msg:  fmt.Sprintf("offset %d not in any HBIN", absOffset),
		Err:  types.ErrCorrupt,
	}
}

// readCellDataAcrossHBINs reads cell data starting at absOffset, handling cells
// that span multiple HBINs by skipping HBIN headers.
func (r *reader) readCellDataAcrossHBINs(absOffset int) ([]byte, error) {
	// First, read the cell size (4 bytes)
	if absOffset+4 > len(r.buf) {
		return nil, &types.Error{
			Kind: types.ErrKindFormat,
			Msg:  "cell size out of bounds",
			Err:  types.ErrCorrupt,
		}
	}

	cellSizeRaw := int32(binary.LittleEndian.Uint32(r.buf[absOffset : absOffset+4]))
	cellSize := int(cellSizeRaw)
	if cellSizeRaw < 0 {
		cellSize = -cellSize // allocated
	}

	if cellSize < 4 {
		return nil, &types.Error{
			Kind: types.ErrKindFormat,
			Msg:  "cell size too small",
			Err:  types.ErrCorrupt,
		}
	}

	// Fast path: find which HBIN this cell starts in using index
	_, hbinEnd, err := r.findHBINForOffset(absOffset)
	if err != nil {
		return nil, err
	}

	// Check if cell fits entirely in this HBIN
	if absOffset+cellSize <= hbinEnd {
		// Cell doesn't cross HBIN boundary - fast path, return direct slice
		return r.buf[absOffset : absOffset+cellSize], nil
	}

	// Slow path: cell crosses HBIN boundaries, need to copy data skipping headers
	result := make([]byte, cellSize)
	copied := 0
	currentOffset := absOffset

	for copied < cellSize {
		// Find current HBIN using index
		_, currentHBINEnd, err := r.findHBINForOffset(currentOffset)
		if err != nil {
			return nil, &types.Error{
				Kind: types.ErrKindFormat,
				Msg:  "invalid HBIN while reading cell",
				Err:  types.ErrCorrupt,
			}
		}

		// Current offset is in this HBIN - copy available data
		availableInHBIN := currentHBINEnd - currentOffset
		needToCopy := cellSize - copied
		toCopy := availableInHBIN
		if toCopy > needToCopy {
			toCopy = needToCopy
		}

		if currentOffset+toCopy > len(r.buf) {
			return nil, &types.Error{
				Kind: types.ErrKindFormat,
				Msg:  "cell data out of bounds",
				Err:  types.ErrCorrupt,
			}
		}

		copy(result[copied:], r.buf[currentOffset:currentOffset+toCopy])
		copied += toCopy
		currentOffset += toCopy

		// If we've copied everything from this HBIN but need more,
		// skip to next HBIN (skip its header)
		if copied < cellSize && currentOffset >= currentHBINEnd {
			// Next HBIN starts at currentHBINEnd, skip its 32-byte header
			currentOffset = currentHBINEnd + format.HBINHeaderSize
		}

		// Safety check to prevent infinite loop
		if toCopy == 0 {
			return nil, &types.Error{
				Kind: types.ErrKindFormat,
				Msg:  "unable to read cell data - no progress made",
				Err:  types.ErrCorrupt,
			}
		}
	}

	return result, nil
}

// Introspection functions (for forensics/debugging) --------------------------

// KeyNameLen returns the byte length of the key name without decoding.
func (r *reader) KeyNameLen(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}
	return int(nk.NameLength), nil
}

// ValueNameLen returns the byte length of the value name without decoding.
func (r *reader) ValueNameLen(id types.ValueID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, err
	}
	return int(vk.NameLength), nil
}

// NodeStructSize returns the size of the NK record structure in bytes.
func (r *reader) NodeStructSize(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	cell, err := r.cell(uint32(id))
	if err != nil {
		return 0, err
	}
	return cell.Size, nil
}

// ValueStructSize returns the size of the VK record structure in bytes.
func (r *reader) ValueStructSize(id types.ValueID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	cell, err := r.cell(uint32(id))
	if err != nil {
		return 0, err
	}
	return cell.Size, nil
}

// ValueDataCellOffset returns the file offset and length of the data cell for a value.
// For inline values (stored in the VK record itself), returns (0, length).
// Otherwise returns the absolute file offset (HBIN-relative) and size of the data cell.
func (r *reader) ValueDataCellOffset(id types.ValueID) (uint32, int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, 0, err
	}

	// Get the actual data length (clear inline flag)
	length := int(vk.DataLength & format.VKDataLengthMask)

	// If data is inline (stored in the DataOffset field itself), return 0 offset
	if vk.DataInline() {
		return 0, length, nil
	}

	// Otherwise return the offset to the data cell
	return vk.DataOffset, length, nil
}

// Hivex-compatible introspection methods -------------------------------------
// These methods match hivex behavior exactly for drop-in compatibility.

// KeyNameLenDecoded returns the UTF-8 string length of the decoded key name.
// This matches hivex_node_name_len() behavior, which decodes the name first.
func (r *reader) KeyNameLenDecoded(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}
	// Decode the name to UTF-8 and return the string length
	decodedName, err := DecodeKeyName(nk)
	if err != nil {
		return 0, err
	}
	return len(decodedName), nil
}

// ValueNameLenDecoded returns the UTF-8 string length of the decoded value name.
// This matches hivex_value_key_len() behavior, which decodes the name first.
func (r *reader) ValueNameLenDecoded(id types.ValueID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, err
	}
	// Decode the name to UTF-8 and return the string length
	decodedName, err := DecodeValueName(vk)
	if err != nil {
		return 0, err
	}
	return len(decodedName), nil
}

// NodeStructSizeCalculated returns the calculated minimum NK structure size.
// This matches hivex_node_struct_length() behavior.
// Formula: CellHeaderSize + NKFixedHeaderSize + NameLength
func (r *reader) NodeStructSizeCalculated(id types.NodeID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}
	// Hivex calculates: cell_header + NK_fixed_header + name_length
	return format.CellHeaderSize + format.NKFixedHeaderSize + int(nk.NameLength), nil
}

// ValueStructSizeCalculated returns the calculated minimum VK structure size.
// This matches hivex_value_struct_length() behavior exactly.
// Hivex formula: decoded_name_len + VKHivexSizeConstant
// Where VKHivexSizeConstant = 24 (determined by analyzing hivex source)
func (r *reader) ValueStructSizeCalculated(id types.ValueID) (int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, err
	}
	// Get decoded name length (matches hivex_value_key_len)
	decodedName, err := DecodeValueName(vk)
	if err != nil {
		return 0, err
	}
	// Hivex formula: decoded_name_length + VKHivexSizeConstant
	return len(decodedName) + format.VKHivexSizeConstant, nil
}

// ValueDataCellOffsetHivex returns data cell info matching hivex behavior.
// For inline values, returns (offset, 0) as a flag instead of (offset, actualSize).
// This matches hivex_value_data_cell_offset() behavior.
func (r *reader) ValueDataCellOffsetHivex(id types.ValueID) (uint32, int, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, 0, err
	}
	vk, err := r.vkOnly(uint32(id))
	if err != nil {
		return 0, 0, err
	}

	// Get the actual data length (clear inline flag)
	length := int(vk.DataLength & format.VKDataLengthMask)

	// If data is inline, hivex returns (0, 0) as a flag
	if vk.DataInline() {
		return 0, 0, nil
	}

	// Otherwise return the offset to the data cell and its length
	return vk.DataOffset, length, nil
}

// Error helpers --------------------------------------------------------------

func wrapIOErr(err error) error {
	return &types.Error{Kind: types.ErrKindState, Msg: err.Error(), Err: err}
}

func wrapFormatErr(err error) error {
	switch {
	case errors.Is(err, format.ErrSignatureMismatch):
		return types.ErrNotHive
	case errors.Is(err, format.ErrTruncated):
		return &types.Error{Kind: types.ErrKindFormat, Msg: "hive truncated", Err: err}
	case errors.Is(err, format.ErrFreeCell):
		return &types.Error{Kind: types.ErrKindCorrupt, Msg: "cell marked free", Err: err}
	default:
		return &types.Error{Kind: types.ErrKindCorrupt, Msg: err.Error(), Err: err}
	}
}

// Navigation helpers (hivex compatibility) ---------------------------------

func (r *reader) Parent(id types.NodeID) (types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}

	// Check if this is the root node by comparing with root offset
	rootID := types.NodeID(r.head.RootCellOffset)
	if id == rootID {
		return 0, types.ErrNotFound
	}

	nk, err := r.nk(id)
	if err != nil {
		return 0, err
	}

	// Return parent NodeID (which is just the offset)
	return types.NodeID(nk.ParentOffset), nil
}

func (r *reader) GetChild(parent types.NodeID, name string) (types.NodeID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}

	// Get all children
	children, err := r.Subkeys(parent)
	if err != nil {
		return 0, err
	}

	// Convert search name to lowercase for case-insensitive comparison
	searchName := strings.ToLower(name)

	// Iterate through children and compare names
	for _, childID := range children {
		meta, err := r.StatKey(childID)
		if err != nil {
			continue // Skip children we can't read
		}

		// Case-insensitive comparison
		if strings.ToLower(meta.Name) == searchName {
			return childID, nil
		}
	}

	return 0, types.ErrNotFound
}

func (r *reader) GetValue(node types.NodeID, name string) (types.ValueID, error) {
	if err := r.ensureOpen(); err != nil {
		return 0, err
	}

	// Get all values at this node
	values, err := r.Values(node)
	if err != nil {
		return 0, err
	}

	// Iterate through values and compare names
	for _, valueID := range values {
		valueName, err := r.ValueName(valueID)
		if err != nil {
			continue // Skip values we can't read
		}

		// Case-insensitive comparison
		if strings.EqualFold(valueName, name) {
			return valueID, nil
		}
	}

	return 0, types.ErrNotFound
}

// Diagnostics & Forensics -------------------------------------------------------

// recordDiagnostic records a diagnostic issue (zero-cost if collector is nil)
func (r *reader) recordDiagnostic(d types.Diagnostic) {
	if r.diagnostics != nil {
		r.diagnostics.record(d)
	}
}

// GetDiagnostics returns passively collected diagnostics (if enabled)
func (r *reader) GetDiagnostics() *types.DiagnosticReport {
	if r.diagnostics == nil {
		return nil
	}
	return r.diagnostics.getReport()
}

// Diagnose performs exhaustive hive validation
func (r *reader) Diagnose() (*types.DiagnosticReport, error) {
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}

	// Create diagnostic scanner and run full scan
	scanner := newDiagnosticScanner(r)
	return scanner.scan()
}

// Ensure reader implements the desired interfaces.
var (
	_ types.Reader  = (*reader)(nil)
	_ types.Scanner = (*reader)(nil)
)
