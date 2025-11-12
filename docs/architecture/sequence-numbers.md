---
title: "Sequence Number Management"
weight: 20
---

# Sequence Number Management

## Overview

HiveKit implements the Windows Registry transaction protocol using sequence numbers to ensure atomicity and crash recovery.

## The Problem (Solved in Phase 6)

**Before Fix:** Multiple `Grow()` calls within a single transaction were incrementing sequence numbers, breaking the transaction protocol:

```go
// Broken behavior (before fix):
Begin():   Seq1=68, Seq2=67  // Start transaction
Grow() #1: Seq1=69, Seq2=68  // Unauthorized increment! ❌
Grow() #2: Seq1=70, Seq2=69  // Unauthorized increment! ❌
Commit():  Seq2=m.seq=68     // Stale value from Begin()
Result:    Seq1=70, Seq2=68  // ❌ INVALID STATE
```

**Root Cause:** Confusion between STRUCTURAL fields (data size) and PROTOCOL fields (sequences). The allocator was calling `TouchNowAndBumpSeq()` which updated both sequences during grow operations.

## The Solution: Architectural Separation

### Field Classification

| Field Type | Example | Owner | Update Timing | Purpose |
|------------|---------|-------|---------------|---------|
| **STRUCTURAL** | Data size (0x28), Checksum | Allocator | Immediately | Tells readers where data ends |
| **PROTOCOL** | Seq1 (0x04), Seq2 (0x08) | tx.Manager | Begin/Commit only | Transaction atomicity |
| **METADATA** | Timestamp (0x0C) | tx.Manager | Commit time | Last modification |

### Correct Behavior

```go
// Correct behavior (after fix):
Begin():   Seq1=68, Seq2=67  // Start transaction
Grow() #1: (no seq change)   // Only updates data size ✅
Grow() #2: (no seq change)   // Only updates data size ✅
Commit():  Seq2=68, Timestamp=now  // Completes transaction ✅
Result:    Seq1=68, Seq2=68  // ✅ VALID STATE
```

## Implementation Details

### tx.Manager (Protocol Owner)

```go
// tx/tx.go

func (m *Manager) Begin() error {
    // Read current PrimarySeq
    m.seq = readU32(data, REGFPrimarySeqOffset)

    // Increment and write back
    newSeq := m.seq + 1
    writeU32(data, REGFPrimarySeqOffset, newSeq)

    // Update timestamp
    nowFiletime := TimeToFiletime(time.Now())
    writeU64(data, REGFTimeStampOffset, nowFiletime)

    // Mark header dirty
    m.dt.Add(0, HeaderSize)

    m.seq = newSeq
    m.inTx = true
    return nil
}

func (m *Manager) Commit() error {
    // 1. Flush data pages
    m.dt.FlushDataOnly()

    // 2. Set SecondarySeq = PrimarySeq (transaction complete)
    writeU32(data, REGFSecondarySeqOffset, m.seq)

    // 3. Update timestamp (commit time, not operation time)
    nowFiletime := TimeToFiletime(time.Now())
    writeU64(data, REGFTimeStampOffset, nowFiletime)

    // 4. Recalculate checksum (AFTER all other updates)
    checksum := calculateHeaderChecksum(data)
    writeU32(data, REGFCheckSumOffset, checksum)

    // 5. Mark header dirty and flush
    m.dt.Add(0, HeaderSize)
    m.dt.FlushHeaderAndMeta(m.mode)

    m.inTx = false
    return nil
}
```

### FastAllocator (Structural Owner)

```go
// alloc/fastalloc.go

func (fa *FastAllocator) Grow(need int) error {
    // ... HBIN allocation logic ...

    // Update STRUCTURAL fields only:
    fa.h.BumpDataSize(uint32(hbinSize))  // Data size field

    // Mark header dirty (tx.Commit will flush it)
    if fa.dt != nil {
        fa.dt.Add(0, format.HeaderSize)
    }

    // Update checksum (structural integrity)
    updateHeaderChecksum(data)

    // NOTE: Does NOT update sequences or timestamp!
    // Those are protocol fields managed by tx.Manager.

    return nil
}
```

## Windows Registry Specification

Per Microsoft's Windows Registry specification:

### Sequence Number Protocol

1. **PrimarySeq (0x04)**: Incremented when transaction begins
2. **SecondarySeq (0x08)**: Updated to match PrimarySeq when transaction completes
3. **Invariant**: `PrimarySeq == SecondarySeq` indicates consistent state

### Crash Recovery

```
If PrimarySeq != SecondarySeq:
    → Transaction was interrupted
    → Hive needs recovery/validation
    → Changes may be partially applied
```

### Why This Matters

Windows and hivexsh validate sequence numbers:

```bash
hivexsh -d test.hiv
# Output:
#   sequence nos             68 68
#   (sequences nos should match if hive was synched at shutdown)
```

If sequences don't match, tools will report the hive as potentially corrupted.

## Testing Strategy

### Unit Tests

**With Transactions** (correct usage):
```go
func TestGrowWithTransaction(t *testing.T) {
    txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

    txMgr.Begin()
    fa.GrowByPages(1)
    txMgr.Commit()

    // Sequences should match
    assert.Equal(t, seq1, seq2)
}
```

**Without Transactions** (sequences won't change):
```go
func TestGrowStructuralOnly(t *testing.T) {
    fa.GrowByPages(1)

    // Data size WILL change (structural)
    assert.Greater(t, newDataSize, oldDataSize)

    // Sequences WON'T change (protocol - needs transaction)
    assert.Equal(t, newSeq1, oldSeq1)
}
```

## Common Pitfalls

### ❌ Calling Grow() Without Transaction

```go
// BAD: Sequences won't be updated
fa.Grow(4096)
// Result: Data size changes, but Seq1 == Seq2 (old values)
```

### ✅ Proper Transaction Wrapping

```go
// GOOD: Complete transaction protocol
txMgr.Begin()   // Increments Seq1, updates timestamp
fa.Grow(4096)   // Updates data size, checksum
txMgr.Commit()  // Sets Seq2 = Seq1, final timestamp, flush
```

## Historical Context

This architectural separation was implemented in **Phase 6** to fix the sequence number conflict bug that was causing transaction failures in `Test_Session_MultipleOperations`.

**Before:** 52/54 tests passing
**After:** 58/58 core tests passing

The fix established clear ownership boundaries:
- Allocator → Structural management
- tx.Manager → Protocol management

This prevents future "flip-flopping" bugs where fixing one test breaks another due to unclear responsibilities.

## References

- [Microsoft Windows Registry Documentation](https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md)
- [Transaction Protocol Design]({{< ref "transaction-protocol" >}})
- [Allocator Component]({{< ref "/components/allocator" >}})
