# Regtext Parser Optimization - Detailed Lessons Learned

## Overview: 4x Performance Improvement Journey

**Starting Point (Baseline):**
- Throughput: 75-88 MB/s
- Memory: 77-393 MB per file
- Allocations: ~250K per MB

**Final Result (After Phase 7):**
- Throughput: 377-398 MB/s (**+360-400%** - 4.6x faster!)
- Memory: 11-62 MB per file (**-80-85%** reduction from baseline!)
- Allocations: ~17K per MB (**-93%** reduction!)

---

## Phase 1: Streaming Architecture (+11-17% throughput)

### What We Changed
```go
// BEFORE: Full file materialization
text, err := decodeInput(data, opts.InputEncoding)  // Returns entire file as STRING
scanner := bufio.NewScanner(strings.NewReader(text))
line := scanner.Text()  // Allocates new string for every line

// AFTER: Stream from bytes
textBytes, err := decodeInputToBytes(data, opts.InputEncoding)  // Returns []byte (zero-copy for UTF-8!)
scanner := bufio.NewScanner(bytes.NewReader(textBytes))
line := scanner.Bytes()  // Returns slice into scanner's buffer - NO allocation!
```

### Why This Worked

#### 1. **Eliminated Massive String Allocation**
**Problem:** Converting entire 48MB file to string allocates 48MB+ of memory BEFORE parsing even starts.

```go
// Before: string(data) copies entire file
func decodeInput(data []byte, enc string) (string, error) {
    return string(data), nil  // 48MB allocation for 48MB file
}

// After: Return slice of original data
func decodeInputToBytes(data []byte, enc string) ([]byte, error) {
    return data, nil  // Zero-copy for UTF-8 files!
}
```

**Why it matters:**
- String is immutable - can't reuse the []byte backing
- Doubles memory usage (original []byte + new string)
- Triggers GC when allocating large strings

#### 2. **scanner.Bytes() vs scanner.Text()**
**Problem:** scanner.Text() allocates a new string for EVERY line by copying the scanner's internal buffer.

```go
// Before: Allocates 1000s of strings
for scanner.Scan() {
    line := scanner.Text()  // NEW string allocation per line
    // Process line...
}

// After: Reuses scanner's buffer
for scanner.Scan() {
    line := scanner.Bytes()  // Returns slice into scanner's buffer - shared memory!
    // Process line...
}
```

**Key insight:** The scanner already has a buffer. Why copy it to a new string? Just use the buffer directly!

#### 3. **bytes Package is Your Friend**
All `strings.*` functions have `bytes.*` equivalents that work without allocating:

```go
// Before: Multiple string allocations
trim := strings.TrimSpace(line)           // Allocates new string
if strings.HasPrefix(trim, "[") {         // Works on string
    section := strings.TrimPrefix(...)    // Another allocation
}

// After: Work with slices (no allocation)
trim := bytes.TrimSpace(line)             // Returns slice - no allocation!
if bytes.HasPrefix(trim, []byte("[")) {   // Works on []byte
    section := bytes.TrimPrefix(...)      // Returns slice - no allocation!
}
```

**Why slicing doesn't allocate:**
```go
data := []byte{1, 2, 3, 4, 5}
slice := data[1:4]  // No allocation! Just a pointer + length + capacity
```

### Key Lessons

1. **Avoid string conversions early** - Keep data as []byte as long as possible
2. **scanner.Bytes() not scanner.Text()** - Reuse buffers instead of copying
3. **bytes package for zero-copy operations** - Trimming, prefix checking, etc.
4. **Convert to string only at storage time** - When storing in final structs

### Performance Impact
- **+11-17% throughput** - Less time spent copying memory
- **Eliminated 1 huge + 1000s of small allocations**
- **Better cache locality** - Working with same memory regions

---

## Phase 2: UTF-16 Encoding Optimization (+8% throughput, -21% memory)

### What We Changed
```go
// BEFORE: Per-character encoding with massive allocations
func encodeUTF16LE(s string, withBOM bool) []byte {
    var words []uint16
    for _, r := range s {
        words = append(words, utf16.Encode([]rune{r})...)  // 😱 TERRIBLE!
    }
    // More allocations...
}

// AFTER: Single-pass encoding
func encodeUTF16LE(s string, withBOM bool) []byte {
    runes := []rune(s)           // 1 allocation
    words := utf16.Encode(runes)  // 1 allocation, encodes ALL at once
    buf := make([]byte, len(words)*2)  // Pre-allocate exact size
    // Write directly to buffer
}
```

### Why This Worked

#### 1. **Eliminated Per-Character Allocations**
**Problem:** The old code called `utf16.Encode()` once PER CHARACTER with a temporary slice allocation.

```go
// Before: For "Hello" (5 chars)
for _, r := range "Hello" {
    words = append(words, utf16.Encode([]rune{r})...)
    // Allocates: []rune{r} - 5 allocations
    // Calls utf16.Encode() 5 times
    // append() may reallocate words slice multiple times
}
// Total: ~15+ allocations for 5 characters!

// After: For "Hello"
runes := []rune("Hello")      // 1 allocation (5 runes)
words := utf16.Encode(runes)  // 1 allocation (result slice)
// Total: 2 allocations for 5 characters
```

**For a 1000-character string:**
- Before: **~3000+ allocations**
- After: **2 allocations**
- **1500x fewer allocations!**

#### 2. **Pre-allocation Prevents Growth**
```go
// Before: Unknown size, grows dynamically
var words []uint16  // nil slice
for _, r := range s {
    words = append(words, ...)  // May reallocate: 0→1→2→4→8→16→32...
}

// After: Pre-allocate exact size
runes := []rune(s)
words := utf16.Encode(runes)
buf := make([]byte, len(words)*2)  // Exact size known upfront
```

**Why slice growth is expensive:**
```go
// When slice runs out of capacity, append() must:
// 1. Allocate new, larger backing array (usually 2x size)
// 2. Copy all existing elements
// 3. Discard old backing array (garbage for GC)

// With 1000 elements:
// 0→1 (copy 0), 1→2 (copy 1), 2→4 (copy 2), 4→8 (copy 4), 8→16 (copy 8)...
// Total copies: 0+1+2+4+8+16+32+64+128+256+512 = ~1023 element copies!
```

#### 3. **Batch Processing is Faster**
```go
// Before: Call function 1000 times
for each character {
    utf16.Encode(single_character)  // Function call overhead × 1000
}

// After: Call function once
utf16.Encode(all_characters)  // Function call overhead × 1
```

**Why:** Modern CPUs love predictable, sequential work. Calling `utf16.Encode()` once lets it:
- Use SIMD instructions (process multiple bytes at once)
- Better branch prediction
- Better cache utilization
- Less function call overhead

### Key Lessons

1. **Never allocate inside loops** - Especially per-element allocations
2. **Batch operations when possible** - Call functions with all data, not one element at a time
3. **Pre-allocate with exact size** - If you know the size, use it!
4. **Avoid slice growth** - Growing slices causes repeated copying
5. **Profile allocation patterns** - Use `-benchmem` to see allocs/op

### Performance Impact
- **-19-21% memory** for string-heavy workloads
- **Eliminated 1000s of allocations per string value**
- **Better GC behavior** - Fewer small objects to track

---

## Phase 3: Hex Parsing Rewrite (+241-311% throughput, -76-89% memory) 🚀

### What We Changed
```go
// BEFORE: 5+ allocations and multiple passes per hex value
func parseHexBytes(hexStr string) ([]byte, error) {
    colonPos := strings.Index(hexStr, ":")      // Scan 1
    hexStr = hexStr[colonPos+1:]                 // Substring allocation

    hexStr = removeWhitespace(hexStr)            // Allocation 1: New string
    parts := strings.Split(hexStr, ",")          // Allocation 2: Slice of strings

    buf := make([]byte, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)                 // Allocation 3+ per part
        if len(p) == 1 {
            p = "0" + p                           // Allocation 4+ for padding
        }
        b, err := hex.DecodeString(p)            // Allocation 5+ per byte
        buf = append(buf, b...)
    }
    return buf, nil
}

// AFTER: Single allocation, in-place parsing
func parseHexBytesFromBytes(hexBytes []byte) ([]byte, error) {
    colonPos := bytes.IndexByte(hexBytes, ':')
    hexBytes = hexBytes[colonPos+1:]             // Slice - no allocation!

    result := make([]byte, 0, len(hexBytes)/3+1) // Pre-allocate once

    i := 0
    for i < len(hexBytes) {
        // Skip whitespace/commas in-place - no allocation
        for i < len(hexBytes) && isHexSkipChar(hexBytes[i]) {
            i++
        }

        // Parse hex digits directly
        hi := hexCharToNibble(hexBytes[i])
        i++
        lo := hexCharToNibble(hexBytes[i])
        i++

        result = append(result, (hi<<4)|lo)  // Manual byte construction
    }
    return result, nil
}
```

### Why This Worked - The Details

#### 1. **Manual Hex Digit Parsing** (Biggest Win!)

**The old way:**
```go
b, err := hex.DecodeString("a5")  // Parses "a5" into []byte{0xa5}
```

**What hex.DecodeString() does internally:**
1. Allocates result []byte
2. Validates entire string
3. Loops through character pairs
4. Looks up each character in a table or does math
5. Combines into bytes

**When you call it 1000 times** (for 1000 bytes):
- 1000 function calls
- 1000 tiny allocations (even though each is just 1 byte!)
- 1000 validations
- Can't be inlined by compiler
- Cache unfriendly (jumps around)

**The optimized way:**
```go
// Lookup table / direct math
func hexCharToNibble(c byte) byte {
    switch {
    case c >= '0' && c <= '9':
        return c - '0'        // 'a' = 97 - 48 = 49... wait, '5' = 53 - 48 = 5 ✓
    case c >= 'a' && c <= 'f':
        return c - 'a' + 10   // 'a' = 97 - 97 + 10 = 10 ✓
    case c >= 'A' && c <= 'F':
        return c - 'A' + 10   // 'A' = 65 - 65 + 10 = 10 ✓
    }
}

// Combine two hex digits into a byte
result := (hi << 4) | lo
// Example: 'a', '5' -> 0xa, 0x5 -> (10 << 4) | 5 = 160 | 5 = 165 = 0xa5 ✓
```

**Why this is faster:**
- **Inlineable** - Compiler can inline this simple function
- **No allocations** - Just arithmetic
- **No function call overhead** - Inlined means direct instructions
- **CPU friendly** - Simple arithmetic is ~1-2 cycles

**Speed difference:**
- `hex.DecodeString("a5")`: ~50-100 nanoseconds (function call + allocation)
- Manual nibble parsing: ~2-5 nanoseconds (pure arithmetic)
- **10-50x faster per byte!**

#### 2. **In-Place Scanning** (No Preprocessing)

**The old way:**
```go
// Step 1: Remove whitespace (full string scan + allocation)
hexStr = removeWhitespace(hexStr)
// Creates new string: "01,02,03,04" (no spaces)

// Step 2: Split by comma (full string scan + allocation)
parts := strings.Split(hexStr, ",")
// Creates slice: ["01", "02", "03", "04"] - 5 allocations!

// Step 3: Parse each part
for _, p := range parts {
    // ...
}
```

**Total passes through data: 3 times (removeWhitespace, Split, parse loop)**

**The optimized way:**
```go
i := 0
for i < len(hexBytes) {
    // Skip whitespace/commas AS WE GO
    for i < len(hexBytes) && isHexSkipChar(hexBytes[i]) {
        i++  // Just increment index - no copying!
    }

    // Parse next byte directly
    hi := hexCharToNibble(hexBytes[i])
    i++
    lo := hexCharToNibble(hexBytes[i])
    i++

    result = append(result, (hi<<4)|lo)
}
```

**Total passes through data: 1 time (single scan)**

**Why this is MUCH faster:**
```
Old way (for "01, 02, 03"):
Pass 1: removeWhitespace -> "01,02,03" (scan all, write all)
Pass 2: Split(",")         -> ["01","02","03"] (scan all, allocate 3)
Pass 3: Parse each         -> [0x01,0x02,0x03] (scan all again)

New way:
Pass 1: Parse directly     -> [0x01,0x02,0x03] (scan once, write once)
```

**Cache efficiency:**
- Old: Data loaded into cache 3 times (may be evicted between passes)
- New: Data loaded once, stays hot in cache

#### 3. **Avoiding String Allocations**

**Key insight:** Strings are immutable and cause allocations

```go
// Every one of these allocates:
s = strings.TrimSpace(s)      // New string
s = s[1:]                      // New string (in some cases)
s = strings.ReplaceAll(s,...) // New string
parts := strings.Split(s,",") // New slice + new strings for each part
```

**With []byte slices:**
```go
b = bytes.TrimSpace(b)   // Returns slice of same backing array - NO allocation
b = b[1:]                 // Returns slice - NO allocation
// No split needed - we scan directly!
```

**String vs []byte for "hex:01,02,03":**
```
As string: "hex:01,02,03" (12 bytes, immutable)
├─ Skip "hex:": "01,02,03" -> allocates new string (8 bytes)
├─ Split by comma -> ["01", "02", "03"] -> 3 new strings + 1 slice (allocates)
└─ Process each -> more allocations

As []byte: []byte("hex:01,02,03")
├─ Skip "hex:": bytes[4:] -> just a pointer + offset, NO allocation
├─ No split needed - scan directly
└─ Parse in-place -> 1 allocation for result only
```

#### 4. **Pre-allocation Strategy**

```go
// Smart estimation
result := make([]byte, 0, len(hexBytes)/3+1)
// "01,02,03" = 8 bytes -> 8/3+1 = 3 bytes output (correct!)
// "ff,ff,ff,ff" = 11 bytes -> 11/3+1 = 4 bytes output (correct!)
```

**Why divide by 3?**
- Most hex bytes formatted as: `XX,` (2 hex digits + comma = 3 characters)
- Slight overestimate is fine - prevents reallocation
- Much better than starting with 0 capacity!

**Reallocation cost:**
```
Starting capacity 0 for 1000 bytes:
0→1→2→4→8→16→32→64→128→256→512→1024
= 11 reallocations, each copies all previous data

Starting capacity 333 (1000/3) for 1000 bytes:
333→666→1332 (if needed)
= 1-2 reallocations worst case
```

#### 5. **Bit Manipulation** (Why it's fast)

```go
result = (hi << 4) | lo
```

**What this does:**
```
hi = 0x0a (10 in decimal)
lo = 0x05 (5 in decimal)

hi << 4 = 0x0a << 4 = 0xa0 (160 in decimal)
  Binary: 00001010 << 4 = 10100000

0xa0 | 0x05:
  10100000
| 00000101
-----------
  10100101 = 0xa5 (165 in decimal)
```

**Why it's fast:**
- CPU has dedicated bit-shift and OR instructions
- Single instruction (1 cycle)
- vs string concatenation: many instructions + allocation

### Dramatic Results Explained

**BinaryHeavy: +311% (4.1x faster)**
- Binary-heavy files are MOSTLY hex data
- Old code: 5+ allocations × 1000s of bytes = 5000+ allocations
- New code: 1 allocation total
- **Result: 4x speedup because we eliminated 99% of the work!**

**Memory: -89% for binary**
- Old: Keep multiple copies of data (original, trimmed, split parts, decoded bytes)
- New: One copy (result buffer)
- **9x less memory!**

**Real-world files: +302-338%**
- Real Windows registries have tons of binary data (security descriptors, cached data, etc.)
- Hex parsing was the bottleneck
- **4x speedup by fixing the bottleneck**

### Key Lessons from Phase 3

1. **Avoid the standard library for hot paths**
   - `hex.DecodeString()` is general-purpose and allocates
   - Manual parsing can be 10-50x faster for simple cases

2. **Single-pass algorithms beat multi-pass**
   - Scan once, parse directly
   - vs. preprocess, split, then parse

3. **String operations are expensive**
   - Every string operation allocates
   - Work with []byte instead

4. **Bit manipulation is your friend**
   - `(hi << 4) | lo` is a single CPU instruction
   - Much faster than string operations

5. **Estimate and pre-allocate**
   - Better to slightly overestimate than grow dynamically
   - Growing slices causes O(n) copies

6. **Inline-friendly code is fast code**
   - Simple functions (like `hexCharToNibble`) can be inlined
   - Inlining eliminates function call overhead

7. **Cache locality matters**
   - Single-pass keeps data hot in CPU cache
   - Multi-pass may evict data between passes

### Why 4x Faster is Possible

**The old code's execution for "hex:01,02,03" (3 bytes):**
```
1. strings.Index(hexStr, ":")           - scan entire string
2. hexStr = hexStr[colonPos+1:]         - allocate substring
3. removeWhitespace(hexStr)             - scan, write to new string
4. strings.Split(hexStr, ",")           - scan, allocate slice + 3 strings
5. For each part (3 iterations):
   a. strings.TrimSpace(p)              - allocate new string
   b. Check length, maybe pad           - maybe allocate
   c. hex.DecodeString(p)               - allocate result + parsing
   d. append to buffer                  - maybe reallocate

Total: ~15+ allocations, 6+ full scans of data
```

**The new code's execution for "hex:01,02,03" (3 bytes):**
```
1. bytes.IndexByte(hexBytes, ':')       - scan to colon
2. hexBytes = hexBytes[colonPos+1:]     - slice (no allocation)
3. result := make([]byte, 0, 2)         - 1 allocation
4. Single loop:
   - Skip 'h','e','x',':'               - just index math
   - Parse '0','1' -> 0x01              - pure arithmetic
   - Skip ','                           - just index math
   - Parse '0','2' -> 0x02              - pure arithmetic
   - Skip ','                           - just index math
   - Parse '0','3' -> 0x03              - pure arithmetic

Total: 1 allocation, 1 scan of data
```

**15 allocations vs 1 allocation = 15x less GC pressure**
**6 scans vs 1 scan = 6x less memory reads**
**Result: 4x faster (15×6 = 90x theoretical, but other overhead limits to 4x)**

---

## Summary: The Optimization Hierarchy

### Level 1: Eliminate Work (Biggest Wins)
- Don't preprocess if you can parse directly
- Don't allocate if you can reuse
- Don't copy if you can slice
- Don't call functions if you can inline

### Level 2: Batch Work
- Process all data at once, not one element at a time
- Single-pass algorithms beat multi-pass
- Pre-allocate if you know the size

### Level 3: Choose Right Data Structures
- []byte for hot paths (zero-copy slicing)
- string only for storage (immutable is safe)
- Pre-sized slices to avoid growth

### Level 4: Use Fast Operations
- Bit manipulation for byte construction
- Direct arithmetic vs string parsing
- Simple inlineable functions

### Level 5: Think About Cache
- Sequential access beats random access
- Keep data hot (single pass)
- Locality matters

---

## Measurement-Driven Optimization

### How We Knew What to Fix

```bash
# Before optimizing, profile!
go test -bench=BenchmarkParseReg_BinaryHeavy -cpuprofile=cpu.out
go tool pprof cpu.out

# Shows hotspots:
# 30% parseHexBytes     <- Phase 3 target!
# 20% encodeUTF16LE     <- Phase 2 target!
# 15% scanner.Text()    <- Phase 1 target!
```

### The Process

1. **Benchmark** - Establish baseline
2. **Profile** - Find bottleneck (CPU + memory)
3. **Optimize** - Fix ONE thing
4. **Benchmark** - Measure improvement
5. **Repeat** - Move to next bottleneck

**Golden Rule:** Measure before and after. If it didn't improve, revert!

---

## Reusable Patterns for Future Optimizations

### Pattern 1: Stream Don't Materialize
```go
// Bad: Load everything into memory first
data := loadEntireFile()
process(data)

// Good: Stream as you go
reader := openFile()
for scanner.Scan() {
    processLine(scanner.Bytes())  // Process incrementally
}
```

### Pattern 2: []byte for Hot Paths
```go
// Bad: String operations allocate
func process(s string) {
    parts := strings.Split(s, ",")  // Allocates
    for _, p := range parts {
        clean := strings.TrimSpace(p)  // Allocates
        // ...
    }
}

// Good: Work with []byte
func process(b []byte) {
    i := 0
    for i < len(b) {
        // Skip delimiters
        for i < len(b) && b[i] == ',' { i++ }
        // Process directly
        // ...
    }
}
```

### Pattern 3: Manual Parsing When Simple
```go
// When parsing is simple (like hex), don't use standard library
// Bad: hex.DecodeString() allocates per call
// Good: Manual nibble parsing with lookup

func parseSimple(c byte) int {
    if c >= '0' && c <= '9' { return int(c - '0') }
    // ... handle other cases
}
```

### Pattern 4: Pre-allocate When Size Known
```go
// Bad: Grow dynamically
var result []byte
for ... {
    result = append(result, ...)  // May reallocate many times
}

// Good: Pre-allocate
result := make([]byte, 0, estimatedSize)
for ... {
    result = append(result, ...)  // Likely no reallocation
}
```

### Pattern 5: Single-Pass Algorithms
```go
// Bad: Multiple passes
data = removeWhitespace(data)  // Pass 1
parts = split(data, ',')        // Pass 2
for each part { parse(part) }   // Pass 3

// Good: Single pass
for i < len(data) {
    skipWhitespace()
    parseNext()
    // Everything in one scan
}
```

---

## Phase 4: String Unescaping Optimization (+1-8% throughput)

### What We Changed
```go
// BEFORE: Always allocate and replace
func unescapeRegString(s string) string {
    s = strings.ReplaceAll(s, EscapedBackslash, Backslash)  // Always allocates
    s = strings.ReplaceAll(s, EscapedQuote, Quote)          // Always allocates
    return s
}

// AFTER: Fast path for no-escape case
func unescapeRegString(s string) string {
    // Single-pass check: look for backslash which precedes all escapes
    if strings.IndexByte(s, '\\') == -1 {
        return s  // Fast path: no backslashes = no escapes (zero allocation)
    }
    // Slow path: backslash found, do replacements
    s = strings.ReplaceAll(s, EscapedBackslash, Backslash)
    s = strings.ReplaceAll(s, EscapedQuote, Quote)
    return s
}
```

### Why This Worked

#### 1. **Fast Path for Common Case**
**Observation:** Most registry key paths and value names DON'T have escape sequences.

```
Typical values in real registry files:
  "Software\\Microsoft\\Windows"     ← HAS backslashes (literal path separators)
  "CurrentVersion"                    ← NO escapes needed
  "DisplayName"                       ← NO escapes needed
  "InstallDate"                       ← NO escapes needed
```

**The insight:** In `.reg` files, backslashes in **quoted strings** are escaped as `\\`. But value names and data rarely need escaping - most are simple alphanumeric identifiers.

#### 2. **Single Check is Cheaper Than Two Replacements**
```go
// Before: Always scan entire string twice
strings.ReplaceAll(s, "\\\\", "\\")   // Scan 1: look for "\\\\"
strings.ReplaceAll(s, "\\\"", "\"")   // Scan 2: look for "\\\""

// After: Single scan, early exit
if strings.IndexByte(s, '\\') == -1 {  // Scan once: look for ANY '\\'
    return s  // Found none? Done! Return original string
}
// Only if backslash found: do the two ReplaceAll calls
```

**Why this matters:**
- `IndexByte` is highly optimized (assembly, SIMD on modern CPUs)
- Early exit avoids two full string scans in common case
- Returning original string = zero allocation

#### 3. **Single-Pass vs Two-Pass Check**
Initial attempt used two Contains checks:
```go
// Slower: Two separate scans
if !strings.Contains(s, "\\\\") && !strings.Contains(s, "\\\"") {
    return s
}
```

But we can do better! Since both escape sequences start with `\`, we only need to check for backslash:
```go
// Faster: One scan for backslash
if strings.IndexByte(s, '\\') == -1 {
    return s  // No backslash = no escapes possible
}
```

**Result:** Single cheap check covers both escape types.

### Performance Impact

**Real-world files (Phase 3 → Phase 4):**
```
2012_Software (43 MB):  350 → 379 MB/s  (+8%)
2003_Software (18 MB):  346 → 357 MB/s  (+3%)
XP_System (9.1 MB):     361 → 368 MB/s  (+2%)
Most others:            +1% improvement
```

**Why modest gains?**
- Unescaping is a small fraction of total parse time (~5-10%)
- Binary parsing (Phase 3) was the dominant bottleneck
- This optimizes the remaining string processing overhead
- Every 1% matters when chasing peak performance!

### Key Lessons

1. **Fast path the common case** - Check if work is needed before doing it
2. **Single-scan checks** - One `IndexByte` beats two `Contains`
3. **Return original when possible** - Zero allocation is best allocation
4. **Small wins add up** - 1-8% improvements compound with other optimizations
5. **Know your data** - Most registry values don't have escape sequences

### Reusable Pattern: Fast-Path Optimization

```go
// Generic pattern for conditional string processing
func processString(s string) string {
    // 1. Check if processing is needed (cheap check)
    if !needsProcessing(s) {
        return s  // Fast path: return original
    }

    // 2. Do expensive processing only when needed
    return expensiveTransform(s)
}
```

**Examples where this applies:**
- URL decoding (check for `%` first)
- HTML unescaping (check for `&` first)
- Path normalization (check for `..` or `.` first)
- Whitespace trimming (check if first/last chars are whitespace)

---

## Phase 5: Buffer Pooling - SKIPPED ❌

### What We Tried
Used `sync.Pool` to reuse buffers for UTF-16 encoding and hex parsing:

```go
var utf16BufferPool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 0, 256)
        return &buf
    },
}

func encodeUTF16LE(s string, withBOM bool) []byte {
    bufPtr := utf16BufferPool.Get().(*[]byte)
    // ... use buffer ...
    result := make([]byte, len(buf))
    copy(result, buf)  // Copy to return
    utf16BufferPool.Put(bufPtr)
    return result
}
```

### Why It DIDN'T Work

**Performance degraded by 1-6% across benchmarks.**

#### 1. **sync.Pool Overhead Exceeds Benefits**
```
Sequential code pattern:
  Parse line → Get buffer → Use → Copy → Put buffer → Parse next line

This creates overhead without benefit because:
- No concurrent goroutines competing for buffers
- Single-threaded code doesn't benefit from pooling
- Get/Put calls add function call overhead
```

#### 2. **Required Copy Defeats the Purpose**
```go
// Have to copy because caller needs to own the returned data
result := make([]byte, len(buf))
copy(result, buf)  // This allocation is unavoidable!
```

The buffer pool would save the initial allocation, but we **still need to allocate for the return value**. So we're adding overhead (Get/Put + copy) without eliminating the allocation.

#### 3. **Small Buffer Sizes**
Most registry values are small (10-100 bytes). Allocating small buffers is very fast in Go. The pooling overhead exceeds the allocation cost for small objects.

### When Buffer Pooling DOES Work

```go
// Good use cases:
1. High concurrency (many goroutines)
2. Large allocations (MB range)
3. Reusable without copying (can return slice of pooled buffer)
4. Hot path called millions of times per second
```

### Key Lessons

1. **Measure don't assume** - Pooling sounds good but may hurt performance
2. **Sequential code rarely benefits from pooling** - Pooling helps with contention
3. **Copy overhead matters** - If you have to copy, you're not saving allocations
4. **Small allocations are cheap** - Go's allocator is very fast for small objects
5. **Profile-driven optimization** - Only optimize where profiler shows problems

### Reusable Pattern: When to Skip Pooling

```go
// Skip pooling if ANY of these are true:
- Sequential, single-threaded code
- Return value requires copying pooled data
- Objects are small (<1KB)
- No profiler evidence of allocation pressure
- Benchmarks show neutral or negative impact
```

---

## Phase 7: Pre-allocation (+3-9% throughput, -20-27% memory)

### What We Changed
```go
// BEFORE: Let slice grow dynamically
var ops []types.EditOp  // Starts as nil, grows with each append
for scanner.Scan() {
    ops = append(ops, op)  // May trigger reallocation and copy
}

// AFTER: Estimate size and pre-allocate
estimatedOps := len(textBytes) / 50  // ~20 ops per KB based on real data
ops := make([]types.EditOp, 0, estimatedOps)  // Pre-allocate capacity
for scanner.Scan() {
    ops = append(ops, op)  // No reallocation unless estimate is way off
}
```

### Why This Worked

#### 1. **Slice Growth is O(n) Operation**
**Problem:** When slice capacity is exceeded, Go allocates new array 2x larger and copies all elements.

```go
// Growing from nil:
append(nil, op)        // Allocate capacity 1
append(cap=1, op)      // Allocate capacity 2, copy 1 element
append(cap=2, op)      // Allocate capacity 4, copy 2 elements
append(cap=4, op)      // Allocate capacity 8, copy 4 elements
// ... continues doubling ...

// For 100K operations:
// - ~17 reallocations
// - Copies totaling ~200K elements
// - Wasted memory from over-allocation
```

#### 2. **Pre-allocation Eliminates Copying**
```go
ops := make([]types.EditOp, 0, estimatedOps)

// Now all appends just increment length:
ops = append(ops, op)  // len++, no reallocation!
```

**Result:** Zero reallocations for operations list (unless estimate is very wrong).

#### 3. **Estimation Based on Real Data**
Analyzed real-world registry files to find pattern:

```
XP_System (9.1 MB)      → 175K ops  = 19 ops/KB
2003_Software (18 MB)   → 352K ops  = 20 ops/KB
Win8_Software (30 MB)   → 563K ops  = 19 ops/KB
Average: ~20 operations per KB
```

Used conservative estimate: **1 operation per 50 bytes = 20 ops/KB**

#### 4. **Map Pre-allocation Also Helps**
```go
seenKeys := make(map[string]bool, estimatedKeys)
```

Pre-allocating maps prevents rehashing as it grows. Maps double in size when they exceed load factor (~80% full), which requires rehashing all entries.

### Performance Impact

**Real-world files (Phase 4 → Phase 7, stable runs):**
```
Win8_Software (30 MB):  374 → 398 MB/s  (+6%, -27% memory!)
XP_System (9.1 MB):     368 → 389 MB/s  (+5%, -21% memory)
DWORDHeavy:             149 → 164 MB/s  (+9%, -11% memory)
2003_Software (18 MB):  358 → 376 MB/s  (+5%, -26% memory)
2012_Software (43 MB):  379 → 386 MB/s  (+1%, -20% memory)
```

**Memory savings are huge:** 20-27% reduction on large files!

**Why some benchmarks didn't improve:**
- Synthetic benchmarks with unusual op/byte ratios
- Estimate might under/over-allocate for generated workloads
- Real-world files show consistent gains (what matters most)

### Key Lessons

1. **Slice growth is expensive** - Doubling + copying adds up
2. **Measure real-world patterns** - Analyze production data for estimates
3. **Conservative estimates win** - Slight over-allocation beats reallocation
4. **Maps benefit too** - Pre-allocate maps to avoid rehashing
5. **Memory matters** - Less allocation = less GC pressure

### Reusable Pattern: Pre-allocation

```go
// Generic pre-allocation pattern:
func parseData(data []byte) []Result {
    // 1. Estimate based on input size and known ratios
    estimated := len(data) / averageBytesPerResult
    if estimated < minReasonable {
        estimated = minReasonable
    }

    // 2. Pre-allocate with capacity
    results := make([]Result, 0, estimated)

    // 3. Append without reallocation
    for ... {
        results = append(results, result)
    }

    return results
}
```

**When to pre-allocate:**
- You can estimate final size from input
- Building large slices (>1000 elements)
- Appending in loops
- Real-world pattern analysis shows consistent ratios

---

## Performance Validation

### Before (Baseline)
```
BenchmarkParseReg_XP_System-10      116ms   82 MB/s    77MB   2.5M allocs
BenchmarkParseReg_BinaryHeavy-10     15ms   67 MB/s    10MB   335K allocs
```

### After (Phase 3)
```
BenchmarkParseReg_XP_System-10       26ms  361 MB/s    15MB   175K allocs
BenchmarkParseReg_BinaryHeavy-10      3ms  309 MB/s     1MB     8K allocs
```

### Improvements
- **4.4x faster** (116ms → 26ms)
- **5.1x memory reduction** (77MB → 15MB)
- **14x fewer allocations** (2.5M → 175K)

**Binary workload even better:**
- **5x faster** (15ms → 3ms)
- **10x memory reduction** (10MB → 1MB)
- **42x fewer allocations** (335K → 8K)

---

## Key Takeaways

1. **Measure first** - Profile shows where time is spent
2. **Eliminate allocations** - They're almost always the slowest thing
3. **Work with []byte** - Zero-copy slicing is free
4. **Parse directly** - Skip preprocessing when possible
5. **Single-pass wins** - Cache is king
6. **Manual parsing** - Standard library is general-purpose, not always optimal
7. **Pre-allocate** - Growing slices is expensive
8. **Batch operations** - One function call > many function calls
9. **Validate improvements** - If benchmarks don't show it, it didn't happen
10. **Document why** - Future you (and others) will thank you!

---

**Written after achieving 4.6x performance improvement**
**From: 82-87 MB/s baseline**
**To: 377-398 MB/s optimized (Phase 7)**
**Previous milestones:**
- **Phase 3:** 346-372 MB/s (hex parsing rewrite)
- **Phase 4:** 357-379 MB/s (string unescaping)
- **Phase 7:** 377-398 MB/s (pre-allocation)
**Date: 2025-11-04**
