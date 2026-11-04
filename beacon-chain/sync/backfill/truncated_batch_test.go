package backfill

import (
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const (
	truncatedTestBatchSize = 32
	// truncatedTestMissing is the number of leading blocks a peer leaves out of its response.
	truncatedTestMissing = 11
	// truncatedTestMaxRounds bounds the number of download/import rounds served by honest peers.
	truncatedTestMaxRounds = 10
)

// truncatedTestSetup holds a canonical chain with one block per slot in [floor, origin],
// and a backfill service whose store starts at the origin block.
type truncatedTestSetup struct {
	floor, origin primitives.Slot
	chain         map[primitives.Slot]blocks.ROBlock
	store         *Store
	service       *Service
}

func newTruncatedTestSetup(t *testing.T) *truncatedTestSetup {
	denebSlot, _ := testDenebAndFuluSlots(t)
	floor := denebSlot + truncatedTestBatchSize
	origin := floor + 3*truncatedTestBatchSize

	chain := make(map[primitives.Slot]blocks.ROBlock)
	parent := [32]byte{}
	for slot := floor; slot <= origin; slot++ {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, parent, slot, 0)
		chain[slot] = blk
		parent = blk.Root()
	}

	originBlock := chain[origin]
	originRoot, originParent := originBlock.Root(), originBlock.Block().ParentRoot()
	su := &Store{
		store: &mockBackfillDB{},
		bs: &dbval.BackfillStatus{
			LowSlot:       uint64(origin),
			LowRoot:       originRoot[:],
			LowParentRoot: originParent[:],
		},
	}

	sn, err := das.NewSyncNeeds(func() primitives.Slot { return origin + 1 }, nil, nil, 0)
	require.NoError(t, err)
	s := &Service{
		clock:     startup.NewClock(time.Now(), [32]byte{}),
		store:     su,
		syncNeeds: sn,
	}
	s.batchImporter = s.defaultBatchImporter
	s.batchSeq = newTruncatedTestSequencer(primitives.Slot(su.status().LowSlot), floor)
	return &truncatedTestSetup{floor: floor, origin: origin, chain: chain, store: su, service: s}
}

func newTruncatedTestSequencer(low, floor primitives.Slot) *batchSequencer {
	return newBatchSequencer(2, low, truncatedTestBatchSize, mockCurrentNeedsFunc(floor, math.MaxUint64))
}

// serve fills the batch with the canonical blocks a peer returns for it, starting at slot `from`.
// An honest peer starts at b.begin; a peer missing the beginning of the range starts later.
func (ts *truncatedTestSetup) serve(b batch, from primitives.Slot) batch {
	b.blocks = nil
	for slot := max(b.begin, from); slot < b.end; slot++ {
		if blk, ok := ts.chain[slot]; ok {
			b.blocks = append(b.blocks, blk)
		}
	}
	return b.withState(batchImportable)
}

// round downloads every batch the sequencer asks for, with `from` choosing the first slot
// returned for each batch, then imports the downloaded batches.
func (ts *truncatedTestSetup) round(t *testing.T, from func(b batch) primitives.Slot) {
	todo, err := ts.service.batchSeq.sequence()
	require.NoError(t, err)
	for _, b := range todo {
		if b.state == batchEndSequence {
			continue
		}
		ts.service.batchSeq.update(ts.serve(b, from(b)))
	}
	ts.service.importBatches(t.Context())
}

func honest(b batch) primitives.Slot { return b.begin }

// runHonestRounds lets honest peers serve every batch until backfill ends or the round limit is hit.
func (ts *truncatedTestSetup) runHonestRounds(t *testing.T) {
	for range truncatedTestMaxRounds {
		if ts.service.batchSeq.allEnded() {
			return
		}
		ts.round(t, honest)
	}
}

// truncateFirstBatch makes the peer serving the first batch, the one right below the origin,
// leave out its leading blocks, then imports.
func (ts *truncatedTestSetup) truncateFirstBatch(t *testing.T) {
	first := true
	ts.round(t, func(b batch) primitives.Slot {
		if first {
			first = false
			return b.begin + truncatedTestMissing
		}
		return b.begin
	})
}

func TestImportBatches_HonestPeersReachFloor(t *testing.T) {
	ts := newTruncatedTestSetup(t)
	ts.runHonestRounds(t)
	require.Equal(t, uint64(ts.floor), ts.store.status().LowSlot)
}

// TestImportBatches_TruncatedBatchRecovers checks that backfill keeps making progress
// after a single peer answers a BlocksByRange request without the leading blocks of the range.
//
// Nothing in a batch shows that its leading blocks are missing: they look like empty slots.
// The batch imports, and the store's low parent root becomes the parent of its lowest block,
// which is in the range the peer left out. The next batch ends one slot before that range,
// so its last block does not match the low parent root and ensureParent fails with errChainBroken.
// The missing range belongs to a batch already marked complete, so the failing batch is widened
// up to the store's low slot, and the missing blocks are requested again.
func TestImportBatches_TruncatedBatchRecovers(t *testing.T) {
	ts := newTruncatedTestSetup(t)
	firstBegin := ts.origin - truncatedTestBatchSize

	ts.truncateFirstBatch(t)
	// The truncated batch was imported: its lowest block looks like the lowest backfilled block.
	require.Equal(t, uint64(firstBegin+truncatedTestMissing), ts.store.status().LowSlot)

	// From now on, every peer is honest. Backfill should still reach the floor.
	ts.runHonestRounds(t)
	require.Equal(t, uint64(ts.floor), ts.store.status().LowSlot,
		"backfill is stuck: blocks in [%d, %d) were left out by one peer and are never requested again",
		firstBegin, firstBegin+truncatedTestMissing)
}

// TestImportBatches_TruncatedBatchRecoversOnRestart checks that a restart, which builds a new sequencer
// from the store's low slot, still reaches the floor after a truncated batch.
func TestImportBatches_TruncatedBatchRecoversOnRestart(t *testing.T) {
	ts := newTruncatedTestSetup(t)

	ts.truncateFirstBatch(t)
	ts.runHonestRounds(t)

	ts.service.batchSeq = newTruncatedTestSequencer(primitives.Slot(ts.store.status().LowSlot), ts.floor)
	ts.runHonestRounds(t)
	require.Equal(t, uint64(ts.floor), ts.store.status().LowSlot)
}
