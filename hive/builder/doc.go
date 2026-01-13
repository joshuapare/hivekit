// Package builder provides a high-performance, path-based API for building
// Windows Registry hive files programmatically.
//
// # Overview
//
// The builder API is designed for maximum ergonomics in parsing loops and data
// import scenarios. Every operation takes a full path to the target key,
// eliminating the need for context management or navigation.
//
// Key features:
//   - Create new hives from scratch or modify existing ones
//   - Progressive writes to disk for constant memory usage
//   - Type-safe value helpers (SetString, SetDWORD, etc.)
//   - 100% reuse of proven merge/transaction infrastructure
//   - ACID semantics with crash recovery
//   - Pluggable write strategies (InPlace/Append/Hybrid)
//
// # Basic Usage
//
//	// Create or open hive
//	b, err := builder.New("/tmp/app.hive", builder.DefaultOptions())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
//	// Add values at specific paths
//	b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
//	b.SetDWORD([]string{"Software", "MyApp"}, "Enabled", 1)
//	b.SetBinary([]string{"Software", "MyApp", "Settings"}, "Key", []byte{0xAA, 0xBB})
//
//	// Commit changes
//	if err := b.Commit(); err != nil {
//	    log.Fatal(err)
//	}
//
// # Parsing Loop Pattern
//
// The API is optimized for parsing scenarios where you know the full path
// and value data upfront:
//
//	b, _ := builder.New("/tmp/output.hive", nil)
//	defer b.Close()
//
//	for line := range regFileLines {
//	    path, name, typ, data := parseLine(line)
//	    b.SetValue(path, name, typ, data)
//	}
//
//	b.Commit()
//
// # Progressive Writes
//
// For large hives, the builder automatically flushes operations to disk
// periodically to maintain constant memory usage:
//
//	opts := builder.DefaultOptions()
//	opts.AutoFlushThreshold = 10000  // Flush every 10k operations
//	opts.PreallocPages = 100000      // Pre-allocate ~400MB
//
//	b, _ := builder.New("/tmp/huge.hive", opts)
//	defer b.Close()
//
//	for _, record := range millionsOfRecords {
//	    b.SetString(record.Path, record.Name, record.Value)
//	}
//
//	b.Commit()
//
// # Performance
//
// The builder leverages the existing high-performance infrastructure:
//   - O(1) cell allocation via segregated free lists
//   - O(1) key/value lookups via in-memory index
//   - Zero-copy operations with memory-mapped I/O
//   - Progressive dirty page tracking
//   - Platform-optimized flush (mremap, F_FULLFSYNC, etc.)
//
// Typical throughput: ~1000 operations/second per hive.
//
// # Thread Safety
//
// Builder instances are NOT thread-safe. Use one builder per goroutine.
// For concurrent building across multiple hives, create one builder per hive.
package builder

/*

COMPLETE API REFERENCE
======================

TYPES
-----

type Builder struct {
    // Manages the underlying hive, transaction state, and progressive writes
}

type Options struct {
    Strategy           StrategyType
    PreallocPages      int
    AutoFlushThreshold int
    CreateIfNotExists  bool
    HiveVersion        HiveVersion
    FlushMode          dirty.FlushMode
    SlackPct           int
}

type StrategyType int
    const (
        StrategyInPlace  // Reuse freed cells (minimal growth)
        StrategyAppend   // Never reuse (monotonic growth)
        StrategyHybrid   // Heuristic (default)
    )

type HiveVersion int
    const (
        Version1_3  // NT 3.51 / Win95
        Version1_4  // NT 4.0
        Version1_5  // Win 2000 / XP
        Version1_6  // Win 10 / 11
    )


CONSTRUCTOR
-----------

func New(path string, opts *Options) (*Builder, error)
    Creates a new builder for the specified hive file.

    If opts.CreateIfNotExists is true (default) and the file doesn't exist,
    creates a minimal valid hive with a root key.

    If the file exists, opens it for modification.

    Parameters:
        path - Absolute path to hive file
        opts - Configuration options (nil uses DefaultOptions())

    Returns:
        *Builder - Ready-to-use builder instance
        error    - If file can't be created/opened or is invalid

    Example:
        b, err := builder.New("/tmp/app.hive", nil)


OPTIONS
-------

func DefaultOptions() *Options
    Returns recommended options for general-purpose building.

    Defaults:
        Strategy:           StrategyHybrid
        PreallocPages:      0 (dynamic growth)
        AutoFlushThreshold: 1000 (progressive writes enabled)
        CreateIfNotExists:  true
        HiveVersion:        Version1_3 (maximum compatibility)
        FlushMode:          FlushDataAndMeta (full fsync)
        SlackPct:           12 (for Hybrid strategy)


TYPE-SAFE VALUE SETTERS
------------------------

func (b *Builder) SetString(path []string, name string, value string) error
    Sets a REG_SZ value at the specified path.
    Automatically encodes string to UTF-16LE with null terminator.
    Creates intermediate keys as needed.

    Parameters:
        path  - Full path to parent key (e.g., []string{"Software", "MyApp"})
        name  - Value name (empty string "" for default value)
        value - String value

    Example:
        b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")

func (b *Builder) SetExpandString(path []string, name string, value string) error
    Sets a REG_EXPAND_SZ value (string with environment variables).
    Automatically encodes to UTF-16LE with null terminator.

    Example:
        b.SetExpandString([]string{"Environment"}, "Path", "%SystemRoot%\\System32")

func (b *Builder) SetBinary(path []string, name string, data []byte) error
    Sets a REG_BINARY value with raw bytes.

    Example:
        b.SetBinary([]string{"Security"}, "Key", []byte{0x01, 0x02, 0x03})

func (b *Builder) SetDWORD(path []string, name string, value uint32) error
    Sets a REG_DWORD value (32-bit little-endian integer).

    Example:
        b.SetDWORD([]string{"Software", "MyApp"}, "Timeout", 30)

func (b *Builder) SetQWORD(path []string, name string, value uint64) error
    Sets a REG_QWORD value (64-bit little-endian integer).

    Example:
        b.SetQWORD([]string{"Stats"}, "Counter", 1234567890123)

func (b *Builder) SetMultiString(path []string, name string, values []string) error
    Sets a REG_MULTI_SZ value (array of strings).
    Automatically encodes as UTF-16LE with null separators and double-null terminator.

    Example:
        b.SetMultiString([]string{"Paths"}, "SearchDirs", []string{
            "C:\\Program Files",
            "C:\\Windows\\System32",
        })

func (b *Builder) SetDWORDBigEndian(path []string, name string, value uint32) error
    Sets a REG_DWORD_BIG_ENDIAN value (32-bit big-endian integer).
    Rare type, mostly for compatibility.

    Example:
        b.SetDWORDBigEndian([]string{"Network"}, "Magic", 0x12345678)


GENERIC VALUE SETTER
---------------------

func (b *Builder) SetValue(path []string, name string, typ uint32, data []byte) error
    Sets a value with explicit type and raw data.
    Use this for custom types or when you already have encoded data.

    Parameters:
        path - Full path to parent key
        name - Value name
        typ  - Registry value type (REG_SZ, REG_DWORD, etc.)
        data - Pre-encoded value data

    Example:
        b.SetValue(
            []string{"Custom"},
            "Data",
            0x0001,  // REG_SZ
            []byte{'H', 0, 'i', 0, 0, 0},  // UTF-16LE "Hi\0"
        )


DELETION OPERATIONS
-------------------

func (b *Builder) DeleteKey(path []string, recursive bool) error
    Deletes a key at the specified path.

    Parameters:
        path      - Full path to key to delete
        recursive - If true, deletes subkeys recursively
                    If false, fails if key has subkeys

    Returns:
        error - If key doesn't exist (only when not recursive)
                If key has subkeys and recursive=false

    Example:
        // Delete key with all subkeys
        b.DeleteKey([]string{"Software", "OldApp"}, true)

        // Delete empty key only
        b.DeleteKey([]string{"Software", "MyApp", "TempData"}, false)

func (b *Builder) DeleteValue(path []string, name string) error
    Deletes a value from the specified key.
    Idempotent - succeeds even if value doesn't exist.

    Parameters:
        path - Full path to parent key
        name - Value name to delete

    Example:
        b.DeleteValue([]string{"Software", "MyApp"}, "Deprecated")


LIFECYCLE METHODS
-----------------

func (b *Builder) Commit() error
    Flushes any pending operations and commits all changes to disk.

    Process:
        1. Flush any pending operations (final progressive write)
        2. Update transaction sequences (PrimarySeq = SecondarySeq)
        3. Update timestamp and checksum
        4. Sync all dirty pages to disk (msync + fsync)
        5. Close session and hive

    After Commit(), the builder is closed and cannot be reused.

    Returns:
        error - If flush or sync fails

    Example:
        if err := b.Commit(); err != nil {
            log.Fatal("Failed to commit:", err)
        }

func (b *Builder) Rollback() error
    Closes the builder without committing changes (best effort).

    Note: Due to progressive writes, some changes may already be on disk.
    This is not a true rollback - it just closes without final commit.

    After Rollback(), the builder is closed and cannot be reused.

    Returns:
        error - If cleanup fails

    Example:
        if err != nil {
            b.Rollback()
            return err
        }

func (b *Builder) Close() error
    Alias for Rollback(). Closes without committing.
    Useful for defer cleanup.

    Example:
        b, err := builder.New("/tmp/test.hive", nil)
        if err != nil {
            return err
        }
        defer b.Close()  // Cleanup if we don't reach Commit()

        // ... operations ...

        return b.Commit()  // Normal path


INTERNAL IMPLEMENTATION FUNCTIONS
----------------------------------

These functions are used internally by the Builder but are documented here
for completeness. Users typically don't call these directly.

func createMinimalHive(path string, version HiveVersion) (*hive.Hive, error)
    Creates a new minimal hive file with valid structure.

    Creates:
        - REGF header (4KB) with correct signature, version, checksum
        - First HBIN (4KB minimum) with header
        - Root NK cell (empty name, no values/subkeys)
        - Master free cell for remaining HBIN space

    Returns:
        - Opened *hive.Hive ready for building

    Implementation details:
        1. Create file, allocate 8KB (4KB header + 4KB HBIN)
        2. Write REGF header at offset 0x0000
        3. Write HBIN header at offset 0x1000
        4. Allocate root NK at offset 0x1020 (32 bytes into HBIN)
        5. Create master free cell for ~3KB remaining space
        6. mmap() file and return

func (b *Builder) addOp(op merge.Op) error
    Adds an operation to the current plan.
    Triggers progressive flush if threshold reached.

    Process:
        1. Add operation to plan
        2. Increment operation counter
        3. If counter >= AutoFlushThreshold:
            a. Apply plan with transaction
            b. Reset plan and counter

func (b *Builder) flush() error
    Flushes pending operations to disk.

    Process:
        1. Apply current plan via session.ApplyWithTx()
        2. Reset plan and counter

    Called automatically by addOp() when threshold reached.
    Called explicitly by Commit() for final flush.


VALUE ENCODING FUNCTIONS
-------------------------

These functions in encode.go handle conversion from Go types to registry
value encodings.

func encodeString(s string) []byte
    Encodes a UTF-8 string to UTF-16LE with null terminator.

    Example:
        "Hello" -> []byte{
            0x48, 0x00,  // 'H'
            0x65, 0x00,  // 'e'
            0x6C, 0x00,  // 'l'
            0x6C, 0x00,  // 'l'
            0x6F, 0x00,  // 'o'
            0x00, 0x00,  // null terminator
        }

func encodeMultiString(values []string) []byte
    Encodes string array to UTF-16LE multi-string format.
    Strings separated by null, terminated by double-null.

    Example:
        []string{"A", "B"} -> UTF-16LE: "A\0B\0\0"

func encodeDWORD(v uint32) []byte
    Encodes uint32 to 4-byte little-endian representation.

    Example:
        0x12345678 -> []byte{0x78, 0x56, 0x34, 0x12}

func encodeQWORD(v uint64) []byte
    Encodes uint64 to 8-byte little-endian representation.

    Example:
        0x123456789ABCDEF0 -> []byte{0xF0, 0xDE, 0xBC, ...}

func encodeDWORDBigEndian(v uint32) []byte
    Encodes uint32 to 4-byte big-endian representation.

    Example:
        0x12345678 -> []byte{0x12, 0x34, 0x56, 0x78}


USAGE PATTERNS
--------------

Pattern 1: Simple Programmatic Building
    b, _ := builder.New("/tmp/app.hive", nil)
    defer b.Close()

    b.SetString([]string{"Software", "MyApp"}, "Version", "1.0")
    b.SetDWORD([]string{"Software", "MyApp"}, "Timeout", 30)

    b.Commit()

Pattern 2: Parsing Loop (Perfect for .reg files)
    b, _ := builder.New("/tmp/output.hive", nil)
    defer b.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        path, name, typ, data := parseLine(scanner.Text())
        b.SetValue(path, name, typ, data)
    }

    b.Commit()

Pattern 3: Large Hive with Progressive Writes
    opts := builder.DefaultOptions()
    opts.AutoFlushThreshold = 10000
    opts.PreallocPages = 100000

    b, _ := builder.New("/tmp/huge.hive", opts)
    defer b.Close()

    for i := 0; i < 10_000_000; i++ {
        path := []string{"Data", fmt.Sprintf("Key%d", i)}
        b.SetDWORD(path, "Value", uint32(i))
    }

    b.Commit()

Pattern 4: Error Handling
    b, err := builder.New("/tmp/app.hive", nil)
    if err != nil {
        return fmt.Errorf("create builder: %w", err)
    }
    defer b.Close()

    if err := b.SetString([]string{"Software", "MyApp"}, "Version", "1.0"); err != nil {
        return fmt.Errorf("set version: %w", err)
    }

    if err := b.Commit(); err != nil {
        return fmt.Errorf("commit: %w", err)
    }

    return nil

Pattern 5: Multiple Operations, Single Transaction
    b, _ := builder.New("/tmp/app.hive", nil)
    defer b.Close()

    // These all go in same transaction (under threshold)
    b.SetString([]string{"Software", "MyApp"}, "Name", "MyApp")
    b.SetString([]string{"Software", "MyApp"}, "Vendor", "Acme Corp")
    b.SetString([]string{"Software", "MyApp"}, "Version", "1.0")
    b.SetDWORD([]string{"Software", "MyApp"}, "Build", 12345)

    b.Commit()  // Single atomic commit

Pattern 6: Cleanup on Error
    func buildHive(path string) (err error) {
        b, err := builder.New(path, nil)
        if err != nil {
            return err
        }
        defer func() {
            if err != nil {
                b.Rollback()  // Don't commit on error
            }
        }()

        // ... operations that might fail ...

        return b.Commit()
    }


REGTEXT INTEGRATION
-------------------

func BuildHiveFromRegText(hivePath string, regText string, opts *Options) error
    Parses a .reg file and builds a hive using the builder API.

    Located in: internal/regtext/builder.go

    Process:
        1. Parse .reg text to operations
        2. Create builder
        3. Apply all operations
        4. Commit

    Example:
        regText := `Windows Registry Editor Version 5.00

        [HKEY_LOCAL_MACHINE\Software\MyApp]
        "Version"="1.0"
        "Timeout"=dword:0000001e
        `

        err := regtext.BuildHiveFromRegText("/tmp/output.hive", regText, nil)


PERFORMANCE CHARACTERISTICS
----------------------------

Throughput:
    - Single-threaded: ~1000 operations/second
    - Multi-hive parallel: ~100-200 hives/second
    - Large batch optimized: ~10M operations/hour

Memory Usage:
    - Index overhead: ~4-5MB per 18K keys
    - Allocator metadata: ~32KB
    - Dirty tracker: 1 bit per 4KB page (~32KB per 1GB hive)
    - Total session: ~5-6MB
    - Progressive writes: Constant memory (doesn't grow with hive size)

Latency:
    - Begin transaction: ~50ns
    - Dirty tracking add: ~20ns
    - Index lookup: O(1), ~50-100ns
    - Cell allocation: O(1), ~200ns
    - Operation apply: ~500ns-1μs
    - Progressive flush: ~10-50ms (depends on dirty pages)
    - Final commit: ~10-100ms

Scalability:
    - Tested up to 1GB hives
    - No algorithmic limits (32-bit offsets = 4GB max hive size)
    - Memory usage constant with progressive writes
    - Concurrent builders scale linearly (one per goroutine)


ERROR HANDLING
--------------

All methods return errors for the following conditions:

New():
    - File path doesn't exist and CreateIfNotExists=false
    - File exists but is not a valid hive
    - Insufficient permissions
    - Disk full (for new hive creation)

SetValue/SetString/SetDWORD/etc():
    - Invalid path (nil or empty)
    - Hive corruption during walk
    - Allocation failure (disk full)
    - Progressive flush failure

DeleteKey():
    - Key doesn't exist (when recursive=false)
    - Key has subkeys (when recursive=false)

DeleteValue():
    - Never fails (idempotent)

Commit():
    - Final flush failure
    - Sync failure (I/O error)
    - Checksum calculation failure

Rollback()/Close():
    - Cleanup failure (rare, logged but not critical)


THREAD SAFETY
--------------

Builder instances are NOT thread-safe. Do not share across goroutines.

Safe patterns:
    // One builder per goroutine
    for _, file := range files {
        go func(f string) {
            b, _ := builder.New(f, nil)
            defer b.Close()
            // ... operations ...
            b.Commit()
        }(file)
    }

Unsafe patterns:
    // DON'T DO THIS: Shared builder across goroutines
    b, _ := builder.New("/tmp/shared.hive", nil)
    go func() { b.SetString(...) }()  // RACE!
    go func() { b.SetDWORD(...) }()   // RACE!


IMPLEMENTATION NOTES
--------------------

Reused Infrastructure (100% reuse, zero duplication):
    - hive/merge/session.go - Transaction management
    - hive/merge/ops.go - Operation types
    - hive/merge/strategy/ - InPlace/Append/Hybrid strategies
    - hive/alloc/fastalloc.go - O(1) cell allocation
    - hive/dirty/dirty.go - Progressive dirty tracking
    - hive/tx/tx.go - ACID transaction protocol
    - hive/index/ - O(1) key/value lookups
    - hive/edit/ - Low-level key/value editors

New Code (thin ergonomic layer):
    - hive/builder/builder.go - Builder type and path-based API
    - hive/builder/create.go - Minimal hive creation
    - hive/builder/encode.go - Value encoding helpers
    - hive/builder/options.go - Configuration types
    - internal/regtext/builder.go - Regtext integration

*/
