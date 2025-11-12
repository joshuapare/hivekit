---
title: "Transaction Protocol"
weight: 10
---

# Transaction Protocol

## Overview

HiveKit implements an ACID transaction protocol based on the Windows Registry hive format specification. The protocol uses sequence numbers and ordered flush operations to ensure atomicity and crash recovery.

## Transaction Lifecycle

```
┌──────────┐     ┌────────────┐     ┌─────────┐     ┌────────┐
│  Begin   │────▶│  Execute   │────▶│  Commit │────▶│  Done  │
│          │     │ Operations │     │         │     │        │
└────┬─────┘     └─────┬──────┘     └────┬────┘     └────────┘
     │                 │                  │
     │                 │                  │
  Seq1++         Mark pages dirty      Seq2=Seq1
  Timestamp      via DirtyTracker      Flush all
  Header dirty                         Update checksum
```

## Implementation

### Begin Phase

```go
func (m *Manager) Begin() error {
    // 1. Read current PrimarySeq
    seq := readU32(data, REGFPrimarySeqOffset)

    // 2. Increment to mark transaction start
    newSeq := seq + 1
    writeU32(data, REGFPrimarySeqOffset, newSeq)

    // 3. Update timestamp (transaction start time)
    nowFiletime := TimeToFiletime(time.Now())
    writeU64(data, REGFTimeStampOffset, nowFiletime)

    // 4. Mark header dirty
    m.dt.Add(0, HeaderSize)

    // 5. Update manager state
    m.seq = newSeq
    m.inTx = true

    return nil
}
```

**State After Begin:**
- `PrimarySeq = N+1` (incremented)
- `SecondarySeq = N` (unchanged)
- `Timestamp = now`
- Transaction is "in progress"

### Execute Phase

During this phase, operations modify the hive:

```go
// Example: Setting a value
strategy.SetValue(path, name, typ, data)
  → KeyEditor.EnsureKeyPath()
  → ValueEditor.SetValue()
  → Allocator.Alloc()  // May call Grow()
  → DirtyTracker.Add(offset, length)
```

**Key Point:** Operations mark pages dirty but **do NOT** modify sequences.

### Commit Phase

```go
func (m *Manager) Commit() error {
    // 1. Flush all dirty DATA pages (NOT header yet)
    m.dt.FlushDataOnly()

    // 2. Set SecondarySeq = PrimarySeq (marks transaction complete)
    writeU32(data, REGFSecondarySeqOffset, m.seq)

    // 3. Update timestamp (commit time)
    nowFiletime := TimeToFiletime(time.Now())
    writeU64(data, REGFTimeStampOffset, nowFiletime)

    // 4. Recalculate header checksum
    checksum := calculateHeaderChecksum(data)
    writeU32(data, REGFCheckSumOffset, checksum)

    // 5. Mark header dirty
    m.dt.Add(0, HeaderSize)

    // 6. Flush header page + optional fdatasync
    m.dt.FlushHeaderAndMeta(m.mode)

    // 7. Clear transaction state
    m.inTx = false
    return nil
}
```

**State After Commit:**
- `PrimarySeq = N+1`
- `SecondarySeq = N+1` (now matches!)
- `Timestamp = commit time`
- All changes flushed to disk
- Transaction is "complete"

## Ordered Flush Protocol

The order of operations in Commit() is **critical** for crash recovery:

```
1. Flush data pages
   ↓
2. Update SecondarySeq (in memory)
   ↓
3. Update timestamp (in memory)
   ↓
4. Update checksum (in memory)
   ↓
5. Mark header dirty
   ↓
6. Flush header page
   ↓
7. Optional fdatasync()
```

### Why This Order Matters

**Crash Scenarios:**

| Crash Point | Result | Recovery |
|-------------|--------|----------|
| Before step 1 | No changes on disk | Clean state, no recovery needed |
| Between 1-6 | Data flushed, header not | Seq1 != Seq2, partial transaction |
| After step 6 | Header flushed | Seq1 == Seq2, complete transaction |

If `PrimarySeq != SecondarySeq`, Windows knows the transaction was interrupted and the hive may need recovery.

## Flush Modes

```go
type FlushMode int

const (
    FlushNone  FlushMode = iota  // No sync (testing only)
    FlushAuto                     // msync() only
    FlushSync                     // msync() + fdatasync()
)
```

### FlushAuto (Recommended)

```go
// Unix/Darwin: msync() with MS_SYNC
msync(addr, length, MS_SYNC)

// Other platforms: No additional sync
// (OS handles flush timing)
```

### FlushSync (Paranoid Mode)

```go
// Additional fdatasync() after header flush
fdatasync(fd)
```

**Trade-off:** Higher durability guarantee vs. ~5-10ms latency per commit

## ACID Properties

### Atomicity

- **Begin** marks transaction start via `Seq1++`
- **Commit** marks transaction complete via `Seq2 = Seq1`
- If `Seq1 != Seq2`, transaction is incomplete

### Consistency

- Checksum validates header integrity
- Ordered flush ensures header is last
- HBIN contiguity enforced by allocator

### Isolation

**Note:** HiveKit transactions are **single-threaded**. Multiple goroutines must coordinate access externally.

### Durability

- `FlushAuto`: Durable after msync() returns
- `FlushSync`: Durable after fdatasync() returns
- Crash recovery possible via sequence number check

## Usage Patterns

### Simple Transaction

```go
session, _ := merge.NewSession(h, opts)

// Begin transaction
session.Begin()

// Execute operations
plan := merge.NewPlan()
plan.AddSetValue(path, name, typ, data)
session.Apply(plan)

// Commit transaction
session.Commit()
```

### Convenience Wrapper

```go
// ApplyWithTx wraps Begin/Apply/Commit
session.ApplyWithTx(plan)
```

### Error Handling

```go
if err := session.Begin(); err != nil {
    return err
}

result, err := session.Apply(plan)
if err != nil {
    session.Rollback()  // Best-effort cleanup
    return err
}

if err := session.Commit(); err != nil {
    return err  // Transaction failed
}
```

## Rollback

```go
func (m *Manager) Rollback() {
    m.inTx = false
    // Note: Cannot undo memory-mapped changes
    // Rollback just prevents Commit from completing
}
```

**Important:** Since HiveKit uses mmap, in-memory changes cannot be rolled back. Rollback primarily prevents the transaction from being marked complete (prevents `Seq2` update).

## Performance Characteristics

| Operation | Typical Time | Notes |
|-----------|-------------|-------|
| Begin() | < 1 μs | Memory write only |
| Apply() | Varies | Depends on operation count |
| Commit() | 5-10 ms | Dominated by I/O |
| Full transaction | ~10 ms | 100KB of dirty data |

### Optimization Tips

1. **Batch operations** - One large transaction is faster than many small ones
2. **Use FlushAuto** - Unless you need absolute durability
3. **Minimize allocations** - Reuse existing cells when possible
4. **Group related operations** - Locality reduces page faults

## Testing Transactions

### Verify Sequences Match

```go
func TestTransactionSequences(t *testing.T) {
    txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

    initialSeq := getSeq1(h)

    txMgr.Begin()
    // ... operations ...
    txMgr.Commit()

    finalSeq1 := getSeq1(h)
    finalSeq2 := getSeq2(h)

    // After commit, sequences should match
    assert.Equal(t, finalSeq1, finalSeq2)

    // Seq1 should have incremented exactly once
    assert.Equal(t, initialSeq+1, finalSeq1)
}
```

### Test Idempotency

```go
func TestBeginIdempotency(t *testing.T) {
    txMgr.Begin()
    txMgr.Begin()  // Should be no-op

    // Seq1 should only increment once
    assert.Equal(t, initialSeq+1, getSeq1(h))
}
```

## References

- [Sequence Number Management]({{< ref "sequence-numbers" >}})
- [Dirty Page Tracking]({{< ref "/components/dirty-tracker" >}})
- [Windows Registry Specification](https://github.com/msuhanov/regf/blob/master/Windows%20registry%20file%20format%20specification.md)
