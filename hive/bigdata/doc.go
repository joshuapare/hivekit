// Package bigdata handles storage and retrieval of large registry values using the DB format.
//
// # Overview
//
// Windows Registry values larger than 16KB (MaxExternalValueBytes) require special handling
// using the DB (big-data) format. This package provides reading and writing of such values
// by chunking them into multiple cells with a DB header cell that references the chunks.
//
// # DB Format Structure
//
// Large values are stored as:
//
//	[DB Header Cell] → [Chunk 0] [Chunk 1] ... [Chunk N]
//
// The DB header contains:
//   - Signature: "db" (0x6462)
//   - Number of chunks (uint16)
//   - Total data size (uint32)
//   - Array of chunk references (uint32 each)
//
// Each chunk is stored in a separate cell, with a maximum size of ~16KB per chunk.
//
// # Usage
//
// Writing large data:
//
//	writer := bigdata.NewWriter(hive, allocator, dirtyTracker)
//	dbRef, err := writer.Store(largeData)
//	if err != nil {
//	    return err
//	}
//	// dbRef is the DB header cell reference to store in the VK
//
// Reading large data:
//
//	reader := bigdata.NewReader(hive)
//	data, err := reader.Read(dbRef)
//	if err != nil {
//	    return err
//	}
//
// # Chunking Strategy
//
// Data is split into chunks of approximately 16KB each:
//   - Chunk size chosen to fit within a single allocation (avoid fragmentation)
//   - Alignment requirements met automatically by allocator
//   - Final chunk may be smaller than 16KB
//
// # Freeing Big-Data
//
// When deleting a value that uses big-data, all chunks and the DB header must be freed:
//
//	err := FreeBigData(allocator, hive, dbRef)
//
// This frees:
//   - The DB header cell
//   - All referenced chunk cells
//
// # Format Specification
//
// The DB header structure (little-endian):
//
//	Offset  Size  Field
//	------  ----  -----
//	0x00    2     Signature ("db" = 0x6462)
//	0x02    2     Number of chunks (n)
//	0x04    4     Total data size
//	0x08    4*n   Chunk cell references (uint32 array)
//
// # Limitations
//
//   - Maximum number of chunks: 65535 (uint16 limit)
//   - Maximum total value size: ~4GB (limited by uint32)
//   - Chunk references use 32-bit cell offsets
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation for chunks
//   - github.com/joshuapare/hivekit/hive/dirty: Dirty page tracking
//   - github.com/joshuapare/hivekit/hive/edit: High-level value editing
package bigdata
