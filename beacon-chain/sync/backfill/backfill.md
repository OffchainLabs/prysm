# Backfill Mechanism in Prysm

## Table of Contents
- [Overview](#overview)
- [Why Backfill?](#why-backfill)
- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
- [Implementation Details](#implementation-details)
- [State Machine](#state-machine)
- [Database Integration](#database-integration)
- [Metrics and Observability](#metrics-and-observability)

## Overview

The backfill mechanism in Prysm is designed to retroactively download historical blocks and blob or data column sidecars for nodes that are initialized via **checkpoint sync**. When a node syncs from a checkpoint (a recent finalized state), it starts from that checkpoint but lacks all historical blocks from genesis up to that checkpoint. The backfill service fills this gap by working backwards from the checkpoint toward genesis (or a specified minimum slot).

**Key characteristics:**
- Runs in the background after initial sync completes
- Downloads blocks in reverse chronological order (newest to oldest)
- Uses concurrent workers to maximize network utilization
- Verifies block signatures and chain continuity
- Handles both blocks and blob sidecars (post-Deneb)

## Why Backfill?

### Checkpoint Sync Gap
When a node uses checkpoint sync:
1. It downloads a recent finalized state (checkpoint state)
2. It syncs forward from that checkpoint to the current head
3. **Gap**: All blocks from genesis to the checkpoint are missing

### Use Cases for Historical Data
Even though these blocks are finalized and don't affect consensus, they're needed for:
- Block history queries via RPC
- Historical state regeneration
- Archive node capabilities
- Data availability requirements
- Compliance with `MIN_EPOCHS_FOR_BLOCK_REQUESTS` spec requirement

### Spec Requirement
The Ethereum specification requires nodes to serve blocks going back `MIN_EPOCHS_FOR_BLOCK_REQUESTS` epochs from the current slot. Backfill ensures this requirement is met.

## Architecture

The backfill system consists of several key components working together:

```
┌─────────────────────────────────────────────────────────────┐
│                    Backfill Service                         │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐        │
│  │   Store     │  │   Verifier   │  │  Blob Store  │        │
│  │  (Status)   │  │ (Signatures) │  │   (Deneb+)   │        │
│  └─────────────┘  └──────────────┘  └──────────────┘        │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │           Batch Sequencer                            │   │
│  │  - Creates batches in reverse chronological order    │   │
│  │  - Manages batch state transitions                   │   │
│  │  - Determines importable batches                     │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │           Worker Pool                                │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │   │
│  │  │ Worker 1 │  │ Worker 2 │  │ Worker N │            │   │
│  │  └──────────┘  └──────────┘  └──────────┘            │   │
│  │       ↕              ↕              ↕                │   │
│  │  ┌──────────────────────────────────────┐            │   │
│  │  │       Batch Router                   │            │   │
│  │  │  - Assigns peers to batches          │            │   │
│  │  │  - Manages worker busy state         │            │   │
│  │  └──────────────────────────────────────┘            │   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕                                  │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              P2P Network                             │   │
│  │  - BeaconBlocksByRange requests                      │   │
│  │  - BlobSidecarsByRange requests                      │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                          ↓
           ┌──────────────────────────────┐
           │    Database (BeaconDB)       │
           │  - Blocks storage            │
           │  - Backfill status tracking  │
           │  - Finalized index updates   │
           |                              |
           |    Blob Storage              |
           |  - Blob data                 |
           |  - Column sidecar data       |
           └──────────────────────────────┘
```

### Core Components

#### 1. Service (`service.go`)
The main orchestrator that:
- Initializes all components
- Runs the main backfill loop
- Coordinates batch scheduling and importing
- Manages lifecycle (start/stop)
- Waits for initial sync before starting

**Location**: `beacon-chain/sync/backfill/service.go`

#### 2. Store (`status.go`)
Tracks backfill progress:
- **BackfillStatus**: Protobuf structure with:
  - `LowSlot`: Lowest slot backfilled so far
  - `LowRoot`: Block root at lowest slot
  - `LowParentRoot`: Parent root of lowest block (next target)
  - `OriginSlot`: Checkpoint sync origin slot
  - `OriginRoot`: Checkpoint sync origin root
- Provides `AvailableBlock()` as an interface method which other services can use to check if a slot is available
- Thread-safe access to status

**Location**: `beacon-chain/sync/backfill/status.go`

#### 3. Batch Sequencer (`batcher.go`)
Manages the sequence of batches:
- Creates batches working backward from `LowSlot` toward minimum slot
- Each batch covers a half-open interval `[begin, end)` of slots
- Maintains batch state (init, sequenced, importable, complete, etc.)
- Determines which batches are importable (ready for DB import)
- Batches must be imported in descending order to maintain chain continuity

**Location**: `beacon-chain/sync/backfill/batcher.go`

#### 4. Worker Pool (`pool.go`)
Concurrent download system:
- Spawns N workers (configurable, default 2)
- **Batch Router**: Assigns available peers to pending batches
- Manages peer busy state to avoid overloading peers
- Routes completed batches back to service

**Location**: `beacon-chain/sync/backfill/pool.go`

#### 5. Workers (`worker.go`)
Individual download workers:
- Download blocks via `BeaconBlocksByRange` RPC
- Download blobs via `BlobSidecarsByRange` RPC (if needed)
- Verify blocks using the verifier
- Handle both block and blob sync phases

**Location**: `beacon-chain/sync/backfill/worker.go`

#### 6. Verifier (`verify.go`)
Block verification:
- Verifies proposer signatures using BLS batch verification
- Checks parent-child chain continuity (parent_root matches)
- Uses signing domains cached by epoch for performance
- Validator public keys loaded from checkpoint state

**Location**: `beacon-chain/sync/backfill/verify.go`

#### 7. Blob Sync (`blobs.go`)
Handles blob sidecars for Deneb+ blocks:
- Identifies which blocks need blobs (post-Deneb with commitments)
- Downloads blob sidecars separately
- Verifies KZG commitments and proofs
- Stores blobs to filesystem storage

**Location**: `beacon-chain/sync/backfill/blobs.go`

## How It Works

### Initialization Phase

1. **Service Creation** (`NewService`)
   - Store initialized with backfill status from DB (or recovered from legacy checkpoint sync)
   - Worker pool created with configured worker count
   - Verifier prepared (waits for genesis data)

2. **Service Start** (`Start`)

- Check if enabled (--enable-backfill)
- Wait for clock (genesis data)
- Check if genesis sync (short-circuit)
- Check if already complete (LowSlot <= minimumBackfillSlot)
- Initialize verifier with origin state
- Wait for initial sync to complete
- Spawn workers and start main loop

3. **Worker Spawn**
   - N workers start listening for batches
   - Batch router starts managing peer assignments

### Main Loop

The service runs a continuous loop:

```go
for {
    // 1. Check for completed batches from workers
    if updateComplete() { // Returns true when backfill done
        markComplete()
        return
    }

    // 2. Import batches that are ready
    importBatches(ctx)

    // 3. Adjust minimum slot based on current time
    moveMinimum(minimumBackfillSlot(currentSlot))

    // 4. Schedule new batches to workers
    scheduleTodos()
}
```

#### Step-by-Step Flow

**1. Batch Creation**
```
Current Status: LowSlot = 1000, LowParentRoot = 0xABC...
Batch Size: 32 slots
Minimum: 100

Sequencer creates:
Batch 1: [968, 1000)  ← First batch ends at current LowSlot
Batch 2: [936, 968)
Batch 3: [904, 936)
...
```

**2. Batch Scheduling**
- Sequencer calls `sequence()` to get batches ready to schedule
- Batches in state `batchInit` or `batchErrRetryable` are sequenced
- Router assigns available peers to batches
- Batches sent to workers

**3. Block Download** (in worker)
```go
// Worker receives batch [936, 968)
request := &BeaconBlocksByRangeRequest{
    StartSlot: 936,
    Count: 32,
    Step: 1,
}
blocks := p2p.BeaconBlocksByRange(peer, request)
```

**4. Verification**
- Verify each block's proposer signature
- Verify chain continuity: `blocks[i-1].root == blocks[i].parent_root`
- Use BLS batch verification for performance

**5. Blob Sync** (if needed)
- Check which blocks need blobs (Deneb+ with commitments)
- If blobs needed, batch state → `batchBlobSync`
- Worker downloads blobs via `BlobSidecarsByRange`
- Verify KZG proofs and commitments
- Hold blobs in memory in `AvailabilityStore` for later saving to filesystem upon a call to `IsDataAvailable` is succesful

**6. Batch Completion**
- Worker returns batch with state `batchImportable`
- Batch router forwards to service

**7. Import Decision**
```go
// Service calls sequencer.importable()
// Returns batches that:
// 1. Are in batchImportable state
// 2. Have no non-importable batches before them (between them and the lowest block that has been backfilled to db)

importable := [batch1, batch2, batch3]
```

**8. Database Import**
```go
for each importable batch (in descending order):
    // Verify last block's root matches expected parent
    ensureParent(status.LowParentRoot)

    // Check data availability (blobs if needed)
    IsDataAvailable(blocks)

    // Save blocks to DB
    db.SaveROBlocks(blocks)

    // Update finalized block index
    db.BackfillFinalizedIndex(blocks)

    // Update backfill status
    status.LowSlot = batch.lowestBlock.Slot
    status.LowRoot = batch.lowestBlock.Root
    status.LowParentRoot = batch.lowestBlock.ParentRoot
    db.SaveBackfillStatus(status)
```

**9. Sequence Update**
- Sequencer updates with completed batch
- Removes completed batches from sequence
- Adds new batches to fill the sequence

### Termination

Backfill completes when:
```go
LowSlot <= minimumBackfillSlot(currentSlot)
```

Where:
```go
minimumBackfillSlot = currentSlot - MIN_EPOCHS_FOR_BLOCK_REQUESTS
```

When complete:
- Service marks completion channel closed
- Workers continue running until context canceled
- `WaitForCompletion()` unblocks

## Configuration

### Flags

Located in `cmd/beacon-chain/sync/backfill/flags/flags.go`:

#### `--enable-backfill`
**Type**: Boolean
**Default**: `false`
**Description**: Enables backfill service. Required for backfill to run.

#### `--backfill-batch-size`
**Type**: Uint64
**Default**: `32`
**Description**: Number of blocks per batch request.

**Trade-offs**:
- Larger: Better network utilization, more memory usage
- Smaller: Less memory, more overhead from multiple requests


#### `--backfill-worker-count`
**Type**: Integer
**Default**: `2`
**Description**: Number of concurrent workers downloading batches.

**Trade-offs**:
- More workers: Better network utilization, higher memory usage
- Fewer workers: Less memory, slower backfill

#### `--backfill-oldest-slot`
**Type**: Uint64
**Default**: None (uses spec minimum)
**Description**: Optionally backfill to an older slot than required by spec.

```bash
# Backfill all the way to genesis
./beacon-chain --enable-backfill --backfill-oldest-slot=0
```

**Note**: If this value > `current - MIN_EPOCHS_FOR_BLOCK_REQUESTS`, it's ignored with a warning.

### Service Options

Programmatic configuration via `ServiceOption` functions:

```go
// beacon-chain/sync/backfill/service.go

WithEnableBackfill(bool)         // Enable/disable
WithWorkerCount(int)             // Number of workers
WithBatchSize(uint64)            // Batch size
WithMinimumSlot(Slot)            // Custom minimum
WithInitSyncWaiter(func() error) // Wait for initial sync
WithVerifierWaiter(...)          // Verification initialization
```

## Implementation Details

### Batch Structure

```go
// beacon-chain/sync/backfill/batch.go

type batch struct {
    firstScheduled time.Time          // When first scheduled
    scheduled      time.Time          // When last scheduled
    seq            int                // Sequence number
    retries        int                // Retry count
    retryAfter     time.Time          // Retry delay
    begin          primitives.Slot    // Start slot (inclusive)
    end            primitives.Slot    // End slot (exclusive)
    results        verifiedROBlocks   // Downloaded blocks
    err            error              // Last error
    state          batchState         // Current state
    busy           peer.ID            // Assigned peer
    blockPid       peer.ID            // Peer that provided blocks
    blobPid        peer.ID            // Peer that provided blobs
    bs             *blobSync          // Blob sync state
}
```

### Backfill Status Protobuf

```protobuf
// proto/dbval/dbval.proto

message BackfillStatus {
    uint64 low_slot = 1;         // Lowest backfilled slot
    bytes low_root = 2;          // Root at lowest slot
    bytes low_parent_root = 3;   // Parent of lowest block
    uint64 origin_slot = 4;      // Checkpoint origin slot
    bytes origin_root = 5;       // Checkpoint origin root
}
```

Stored in the database at key `backfillStatusKey` in the `blocksBucket`.

### Database Operations

#### Saving Backfill Status
```go
// beacon-chain/db/kv/backfill.go

func (s *Store) SaveBackfillStatus(ctx context.Context, bf *BackfillStatus) error
```
Marshals the protobuf and stores in a single key.

#### Reading Backfill Status
```go
func (s *Store) BackfillStatus(ctx context.Context) (*BackfillStatus, error)
```
Retrieves and unmarshals the status. Returns `ErrNotFound` if not present (genesis sync case).

#### Backfill Block Import
```go
// beacon-chain/sync/backfill/status.go

func (s *Store) fillBack(
    ctx context.Context,
    current primitives.Slot,
    blocks []blocks.ROBlock,
    store das.AvailabilityStore,
) (*BackfillStatus, error)
```

Steps:
1. Verify highest block root matches `LowParentRoot`
2. Check data availability for all blocks
3. Save blocks: `db.SaveROBlocks(ctx, blocks, false)`
4. Update finalized index: `db.BackfillFinalizedIndex(ctx, blocks, LowRoot)`
5. Update and save new BackfillStatus

### Verification Process

#### Verifier Initialization
```go
// beacon-chain/sync/backfill/verify.go

func newBackfillVerifier(
    genesisValidatorsRoot []byte,
    validatorKeys [][48]byte,
) (*verifier, error)
```

- Loads validator public keys from checkpoint state
- Pre-computes signing domains for all forks
- Creates domain cache for fast epoch → domain lookup

#### Batch Verification
```go
func (vr verifier) verify(blocks []SignedBeaconBlock) (verifiedROBlocks, error)
```

Process:
1. Convert to ROBlock (Read-Only Block) format
2. Check chain continuity: `block[i-1].root == block[i].parent_root`
3. Create BLS signature batch for all blocks
4. Batch verify all signatures at once (efficient!)
5. Return verified blocks

```go
// Pseudocode for verification
for i, block := range blocks {
    // Check chain
    if i > 0 && blocks[i-1].Root() != block.ParentRoot() {
        return error
    }

    // Add to batch
    proposerIndex := block.ProposerIndex()
    signature := block.Signature()
    publicKey := verifier.keys[proposerIndex]
    domain := verifier.domainForEpoch(block.Epoch())
    sigSet.Add(publicKey, signature, domain, block.Root())
}

// Verify all at once
if !sigSet.Verify() {
    return error
}
```

### Blob Handling

For blocks post-Deneb with blob commitments:

#### Blob Summary
```go
type blobSummary struct {
    blockRoot  [32]byte  // Block root
    index      uint64    // Blob index (0-5)
    commitment [48]byte  // KZG commitment
    signature  [96]byte  // Block signature
}
```

#### Blob Sync Flow
```go
// beacon-chain/sync/backfill/blobs.go

// 1. Identify blobs needed
func (vbs verifiedROBlocks) blobIdents(retentionStart Slot) ([]blobSummary, error)

// 2. Create blob sync state
func newBlobSync(current Slot, vbs verifiedROBlocks, cfg *blobSyncConfig) (*blobSync, error)

// 3. Validate each incoming blob
func (bs *blobSync) validateNext(rb blocks.ROBlob) error {
    // Check block root, index, commitment
    // Verify proposer signature already checked
    // Verify inclusion proof
    // Verify KZG proof
    // Persist to storage
}
```

#### Blob Retention
Only download blobs if the block is within the blob retention period:
```go
blobRetentionStart := current - MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUESTS
```

Blocks older than this don't need blobs (not required by spec).

### Peer Management

#### Peer Assignment
```go
// Assign peers to batches, avoiding busy peers
func (pa PeerAssigner) Assign(busy map[peer.ID]bool, n int) ([]peer.ID, error)
```

Returns up to `n` available peers, excluding busy ones.

#### Peer Scoring
When a batch fails import:
```go
func (s *Service) downscorePeer(peerID peer.ID, reason string) {
    newScore := p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
}
```

This helps avoid unreliable peers.

### Error Handling and Retries

#### Retryable Errors
Batches with errors transition to `batchErrRetryable`:
- Network failures
- Peer disconnections
- Temporary verification failures
- Blob download failures

#### Retry Logic
```go
// beacon-chain/sync/backfill/batch.go

func (b batch) waitUntilReady(ctx context.Context) error {
    if b.retries > 0 {
        untilRetry := time.Until(b.retryAfter)
        if untilRetry > 0 {
            time.Sleep(untilRetry)  // Wait before retry
        }
    }
}

// Retry delay
var retryDelay = time.Second
```

Each retry increments `b.retries` and sets `b.retryAfter = now + retryDelay`.

#### Non-Retryable Errors
- Chain broken (parent_root mismatch after multiple retries)
- Configuration errors
- Context cancellation

## State Machine

### Batch States

```
batchNil            → Zero value, not yet initialized
batchInit           → Initialized, ready to be scheduled
batchSequenced      → Scheduled to workers
batchErrRetryable   → Failed with retryable error
batchBlobSync       → Downloading blobs
batchImportable     → Ready to import to DB
batchImportComplete → Successfully imported
batchEndSequence    → Signals end of backfill
```

### State Transitions

```
┌──────────────┐
│  batchNil    │
└──────┬───────┘
       │ Initialize bounds
       ↓
┌──────────────┐
│  batchInit   │◄──────────────────┐
└──────┬───────┘                   │
       │ sequence()                │ Retry after delay
       ↓                           │
┌──────────────┐                   │
│batchSequenced│                   │
└──────┬───────┘                   │
       │ Worker downloads blocks   │
       ↓                           │
  ┌─────────────────────┐          │
  │ Verification        │          │
  └────┬────────────────┘          │
       │                           │
       ├─Success (no blobs)────────┼──────┐
       │                           │      │
       ├─Success (blobs needed)─┐  │      │
       │                        ↓  │      │
       │                ┌──────────────┐  │
       │                │ batchBlobSync│  │
       │                └──────┬───────┘  │
       │                       │          │
       │                Download blobs    │
       │                       │          │
       │                       ↓          │
       │                   Success?       │
       │                    /    \        │
       │                  Yes    No       │
       │                   │      │       │
       ├───────────────────┘      │       │
       │                          │       │
       │◄─────────────────────────┘       │
       │ (batchErrRetryable)              │
       │                                  │
       ↓                                  │
┌──────────────┐                          │
│batchImportable│◄────────────────────────┘
└──────┬───────┘
       │ Import to DB
       ↓
┌──────────────┐
│batchImport   │
│  Complete    │
└──────────────┘
```

### End Condition

When `batcher.before(upTo)` is called with `upTo <= min`:
```go
return batch{begin: upTo, end: upTo, state: batchEndSequence}
```

When all batches are `batchEndSequence`:
- Worker pool's `complete()` returns `errEndSequence`
- Service marks completion and exits

## Database Integration

### Schema

**Backfill Status**:
- **Bucket**: `blocksBucket`
- **Key**: `backfillStatusKey`
- **Value**: Marshaled `BackfillStatus` protobuf

**Blocks**:
- Saved via `SaveROBlocks(ctx, blocks, cache=false)`
- No caching during backfill: historical blocks are not typically needed for fast lookup, we keep a cache in the db-layer for the recent blocks that are typically needed for faster lookup and avoid backfill blocks polluting this cache.

**Finalized Index**:
- Updated via `BackfillFinalizedIndex(ctx, blocks, finalizedChildRoot)`
- Links backfilled blocks to canonical finalized chain
- Ensures blocks are queryable

### Recovery from Legacy Checkpoint Sync

If `BackfillStatus` doesn't exist in DB:

```go
// beacon-chain/sync/backfill/status.go

func (s *Store) recoverLegacy(ctx context.Context) error {
    // Check for origin checkpoint root
    cpr, err := s.store.OriginCheckpointBlockRoot(ctx)
    if err == ErrNotFoundOriginBlockRoot {
        // No checkpoint, this is genesis sync
        s.genesisSync = true
        return nil
    }

    // Load checkpoint block
    cpb := s.store.Block(ctx, cpr)

    // Create initial BackfillStatus
    bs := &BackfillStatus{
        LowSlot:       cpb.Slot,
        LowRoot:       cpr,
        LowParentRoot: cpb.ParentRoot,
        OriginSlot:    cpb.Slot,
        OriginRoot:    cpr,
    }

    return s.saveStatus(ctx, bs)
}
```

This allows backfill to work with databases from an older checkpoint sync.

## Metrics and Observability

### Prometheus Metrics

Located in `beacon-chain/sync/backfill/metrics.go`:

**Counters**:
- `backfillBatchesImported`: Number of batches successfully imported
- `backfillBlocksApproximateBytes`: Approximate bytes of blocks downloaded
- `backfillBlobsApproximateBytes`: Approximate bytes of blobs downloaded

**Gauges**:
- `backfillRemainingBatches`: Number of batches left to complete
- `batchesWaiting`: Batches in importable state waiting for import
- `oldestBatch`: Slot number of oldest batch currently being processed

**Histograms** (milliseconds):
- `backfillBatchTimeRoundtrip`: Total time from batch scheduled to imported
- `backfillBatchTimeWaiting`: Time batch waited for peer assignment
- `backfillBatchTimeDownloadingBlocks`: Time spent downloading blocks
- `backfillBatchTimeDownloadingBlobs`: Time spent downloading blobs
- `backfillBatchTimeVerifying`: Time spent verifying blocks

### Logging

Key log messages:

**Service start**:
```
INFO "Backfill service not enabled" (if disabled)
INFO "Backfill short-circuit; node synced from genesis"
INFO "Exiting backfill service; minimum block retention slot > lowest backfilled block"
INFO "Backfill service waiting for initial-sync to reach head before starting"
```

**Progress**:
```
INFO "Backfill batches processed"
     imported=2 importable=3 batchesRemaining=47
```

**Completion**:
```
INFO "Backfill is complete" backfillSlot=1234
INFO "Backfill service marked as complete"
```

**Errors**:
```
DEBUG "Batch requesting failed" batchId=1000:1032 error=...
DEBUG "Batch validation failed" batchId=1000:1032 error=...
ERROR "Batch with no results, skipping importer"
DEBUG "Downscore peer" peerID=... reason=backfillBatchImportError newScore=...
```

### Debugging

To debug backfill issues:

1. **Check if enabled**:
   ```bash
   # Look for "Backfill service not enabled"
   grep -i backfill beacon.log
   ```

2. **Monitor progress**:
   ```bash
   # Watch batch import progress
   tail -f beacon.log | grep "Backfill batches processed"
   ```

3. **Check for errors**:
   ```bash
   # Look for batch failures
   grep "Batch.*failed" beacon.log
   ```

4. **Metrics endpoint**:
   ```bash
   curl http://localhost:8080/metrics | grep backfill
   ```

5. **Database status**:
   ```go
   // Check via RPC or debug endpoints
   backfillStatus := db.BackfillStatus(ctx)
   fmt.Printf("Low slot: %d, Origin slot: %d\n",
       backfillStatus.LowSlot, backfillStatus.OriginSlot)
   ```

---

## Summary

The backfill mechanism satisfies:

1. **Fills historical gaps** from checkpoint sync by working backward in time
2. **Uses concurrent workers** to maximize network throughput while managing memory
3. **Verifies all data** (signatures, chain continuity, KZG proofs) before import
4. **Handles both blocks and blobs** appropriately based on the fork
5. **Maintains chain integrity** by importing batches in strict order
6. **Persists progress** to survive restarts
7. **Terminates automatically** when spec requirements are met

The design prioritizes:
- **Correctness**: All blocks verified before import
- **Performance**: Concurrent downloads, batch verification
- **Resource efficiency**: Configurable memory usage
- **Robustness**: Retry logic, peer scoring, error handling
- **Observability**: Comprehensive metrics and logging

This enables Prysm nodes using checkpoint sync to eventually have the same historical data availability as nodes that synced from genesis, without requiring the full sync time upfront.
