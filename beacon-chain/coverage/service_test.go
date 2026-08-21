package coverage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbiface "github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// testChain is a mutable canonical/head view.
type testChain struct {
	mu        sync.Mutex
	canonical map[[32]byte]bool
	head      [32]byte
}

func newTestChain() *testChain {
	return &testChain{canonical: make(map[[32]byte]bool)}
}

func (c *testChain) IsCanonical(_ context.Context, root [32]byte) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canonical[root], nil
}

func (c *testChain) HeadRoot(_ context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.head[:], nil
}

func (c *testChain) setHead(root [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.head = root
}

func (c *testChain) setCanonical(root [32]byte, canonical bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.canonical[root] = canonical
}

// fxBlock is one fixture block with the linkage data needed to derive child
// bids and envelopes.
type fxBlock struct {
	slot              primitives.Slot
	root              [32]byte
	parentRoot        [32]byte
	bidHash           [32]byte
	parentPayloadHash [32]byte
}

type fixture struct {
	t       *testing.T
	ctx     context.Context
	db      db.Database
	chain   *testChain
	reqRoot [32]byte
	seed    byte
}

func newFixture(t *testing.T) *fixture {
	beaconDB := testDB.SetupDB(t)
	reqRoot, err := (&enginev1.ExecutionRequestsGloas{}).HashTreeRoot()
	require.NoError(t, err)
	return &fixture{
		t:       t,
		ctx:     context.Background(),
		db:      beaconDB,
		chain:   newTestChain(),
		reqRoot: reqRoot,
	}
}

// addBlock appends a block at the given slot. parentRevealed states whether
// the parent's payload was used by this child (the child's bid commits the
// parent's fullness testimony).
func (f *fixture) addBlock(slot primitives.Slot, parent *fxBlock, parentRevealed, canonical bool) *fxBlock {
	f.t.Helper()
	f.seed++
	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = slot
	var parentRoot, pph [32]byte
	if parent != nil {
		parentRoot = parent.root
		if parentRevealed {
			pph = parent.bidHash
		} else {
			pph = parent.parentPayloadHash
		}
	}
	copy(blk.Block.ParentRoot, parentRoot[:])
	bid := blk.Block.Body.SignedExecutionPayloadBid.Message
	bidHash := bytesutil.ToBytes32([]byte{0xbb, f.seed})
	copy(bid.BlockHash, bidHash[:])
	copy(bid.ParentBlockHash, pph[:])
	copy(bid.ExecutionRequestsRoot, f.reqRoot[:])
	bid.Slot = slot
	wsb := util.SaveBlock(f.t, f.ctx, f.db, blk)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(f.t, err)
	if canonical {
		f.chain.setCanonical(root, true)
	}
	return &fxBlock{
		slot:              slot,
		root:              root,
		parentRoot:        parentRoot,
		bidHash:           bidHash,
		parentPayloadHash: pph,
	}
}

// storeEnvelope persists the envelope for a fixture block.
func (f *fixture) storeEnvelope(b *fxBlock) {
	f.t.Helper()
	env := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    b.parentPayloadHash[:],
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     b.bidHash[:],
				SlotNumber:    b.slot,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       b.root[:],
			ParentBeaconBlockRoot: b.parentRoot[:],
		},
		Signature: make([]byte, 96),
	}
	require.NoError(f.t, f.db.SaveExecutionPayloadEnvelope(f.ctx, env))
}

// newTestService builds a runtime with the clock injected directly so tests
// can drive reconciliation synchronously.
func newTestService(t *testing.T, f *fixture, currentSlot primitives.Slot) *Service {
	t.Helper()
	s, err := New(context.Background(),
		WithDatabase(f.db),
		WithClockWaiter(startup.NewClockSynchronizer()),
		WithScanBudgets(4, 8, 1<<20),
		WithInterPageYield(0),
	)
	require.NoError(t, err)
	s.clock = startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(currentSlot))
	s.SetChainView(f.chain)
	require.NoError(t, s.store.load(context.Background()))
	return s
}

func gloasTestConfig(t *testing.T) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
}

func indexedSlots(t *testing.T, f *fixture) []primitives.Slot {
	t.Helper()
	roots, err := f.db.RevealedEnvelopeRoots(context.Background(), 0, 1<<20, 1<<20)
	require.NoError(t, err)
	out := make([]primitives.Slot, 0, len(roots))
	for _, r := range roots {
		out = append(out, r.Slot)
	}
	return out
}

func TestBootstrapScanAndIndex(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	// Blocks 1..8; slot 6's payload is withheld (its child builds on slot
	// 5's payload); every block including the stored-but-withheld slot 6 has
	// a persisted envelope.
	var blks []*fxBlock
	parent := (*fxBlock)(nil)
	for slot := primitives.Slot(1); slot <= 8; slot++ {
		parentRevealed := parent != nil && parent.slot != 6
		b := f.addBlock(slot, parent, parentRevealed, true)
		f.storeEnvelope(b)
		blks = append(blks, b)
		parent = b
	}
	head := blks[len(blks)-1]
	f.chain.setHead(head.root)

	s := newTestService(t, f, 8)
	s.reconcile(ctx)

	snap := s.Snapshot()
	require.Equal(t, true, snap.Initialized)
	assert.Equal(t, primitives.Slot(1), snap.Low)
	assert.Equal(t, primitives.Slot(8), snap.High)
	assert.Equal(t, head.root, snap.AnchorRoot)
	// The bootstrap and pure extensions never bump the serve epoch.
	assert.Equal(t, uint64(0), s.ServeEpoch())

	// Withheld slot 6 has no index entry although its envelope is stored;
	// the anchor (8) is not classified yet.
	assert.DeepEqual(t, []primitives.Slot{1, 2, 3, 4, 5, 7}, indexedSlots(t, f))
}

func TestMissingEnvelopeStopsLowerExtension(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	var blks []*fxBlock
	parent := (*fxBlock)(nil)
	for slot := primitives.Slot(1); slot <= 8; slot++ {
		b := f.addBlock(slot, parent, parent != nil, true)
		if slot != 3 {
			f.storeEnvelope(b)
		}
		blks = append(blks, b)
		parent = b
	}
	f.chain.setHead(parent.root)

	s := newTestService(t, f, 8)
	s.reconcile(ctx)

	snap := s.Snapshot()
	// Slot 3 is revealed (its child built on it) but its envelope is
	// missing: coverage stops above the gap and refuses to cross it.
	assert.Equal(t, primitives.Slot(4), snap.Low)
	assert.Equal(t, primitives.Slot(8), snap.High)
	assert.DeepEqual(t, []primitives.Slot{4, 5, 6, 7}, indexedSlots(t, f))

	// A second pass is idempotent.
	s.reconcile(ctx)
	assert.Equal(t, primitives.Slot(4), s.Snapshot().Low)

	// Once the envelope arrives through a forward path or repair, the next
	// wake extends coverage down through the previously blocked pair.
	f.storeEnvelope(blks[2])
	s.reconcile(ctx)
	snap = s.Snapshot()
	assert.Equal(t, primitives.Slot(1), snap.Low)
	assert.DeepEqual(t, []primitives.Slot{1, 2, 3, 4, 5, 6, 7}, indexedSlots(t, f))
}

func TestForwardExtension(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	parent := (*fxBlock)(nil)
	for slot := primitives.Slot(1); slot <= 5; slot++ {
		b := f.addBlock(slot, parent, parent != nil, true)
		f.storeEnvelope(b)
		parent = b
	}
	f.chain.setHead(parent.root)

	s := newTestService(t, f, 5)
	s.reconcile(ctx)
	require.Equal(t, primitives.Slot(5), s.Snapshot().High)

	// Forward progress: two new canonical blocks arrive and the head moves.
	b6 := f.addBlock(6, parent, true, true)
	f.storeEnvelope(b6)
	b7 := f.addBlock(7, b6, true, true)
	f.storeEnvelope(b7)
	f.chain.setHead(b7.root)
	s.clock = startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(7)))

	s.reconcile(ctx)
	snap := s.Snapshot()
	assert.Equal(t, primitives.Slot(7), snap.High)
	assert.Equal(t, b7.root, snap.AnchorRoot)
	// The old anchor (5) and slot 6 are now classified; pure upper extension
	// does not bump the serve epoch.
	assert.DeepEqual(t, []primitives.Slot{1, 2, 3, 4, 5, 6}, indexedSlots(t, f))
	assert.Equal(t, uint64(0), s.ServeEpoch())
}

func TestReorgShrinkFlipsCommonAncestorTestimony(t *testing.T) {
	gloasTestConfig(t)

	t.Run("old child revealed, new child withholds C", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()

		b1 := f.addBlock(1, nil, false, true)
		f.storeEnvelope(b1)
		b2 := f.addBlock(2, b1, true, true)
		f.storeEnvelope(b2)
		b3 := f.addBlock(3, b2, true, true)
		f.storeEnvelope(b3)
		b4 := f.addBlock(4, b3, true, true) // old child reveals 3
		f.storeEnvelope(b4)
		b5 := f.addBlock(5, b4, true, true)
		f.storeEnvelope(b5)
		f.chain.setHead(b5.root)

		s := newTestService(t, f, 5)
		s.reconcile(ctx)
		require.Equal(t, primitives.Slot(5), s.Snapshot().High)
		require.DeepEqual(t, []primitives.Slot{1, 2, 3, 4}, indexedSlots(t, f))
		epochBefore := s.ServeEpoch()

		// Reorg to a branch whose new child at slot 4 withholds slot 3: the
		// common ancestor C = 3 stays canonical but its testimony flips.
		f.chain.setCanonical(b4.root, false)
		f.chain.setCanonical(b5.root, false)
		n4 := f.addBlock(4, b3, false /* new child withholds 3 */, true)
		f.storeEnvelope(n4)
		f.chain.setHead(n4.root)

		s.reconcile(ctx)
		snap := s.Snapshot()
		assert.Equal(t, primitives.Slot(4), snap.High)
		assert.Equal(t, n4.root, snap.AnchorRoot)
		// [C, oldHigh) was deleted including C's still-canonical key, and the
		// rescan of the new branch leaves 3 unindexed (withheld).
		assert.DeepEqual(t, []primitives.Slot{1, 2}, indexedSlots(t, f))
		assert.Equal(t, true, s.ServeEpoch() > epochBefore)
	})

	t.Run("old child withholds, new child reveals C", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()

		b1 := f.addBlock(1, nil, false, true)
		f.storeEnvelope(b1)
		b2 := f.addBlock(2, b1, true, true)
		f.storeEnvelope(b2)
		b3 := f.addBlock(3, b2, true, true)
		f.storeEnvelope(b3)
		b4 := f.addBlock(4, b3, false /* old child withholds 3 */, true)
		f.storeEnvelope(b4)
		f.chain.setHead(b4.root)

		s := newTestService(t, f, 5)
		s.reconcile(ctx)
		require.Equal(t, primitives.Slot(4), s.Snapshot().High)
		require.DeepEqual(t, []primitives.Slot{1, 2}, indexedSlots(t, f))

		f.chain.setCanonical(b4.root, false)
		n4 := f.addBlock(4, b3, true /* new child reveals 3 */, true)
		f.storeEnvelope(n4)
		f.chain.setHead(n4.root)

		s.reconcile(ctx)
		snap := s.Snapshot()
		assert.Equal(t, primitives.Slot(4), snap.High)
		assert.Equal(t, n4.root, snap.AnchorRoot)
		// The restored key at C reflects the new child's testimony.
		assert.DeepEqual(t, []primitives.Slot{1, 2, 3}, indexedSlots(t, f))
	})
}

func TestDeepReorgDiscardsInterval(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	b1 := f.addBlock(1, nil, false, true)
	f.storeEnvelope(b1)
	b2 := f.addBlock(2, b1, true, true)
	f.storeEnvelope(b2)
	b3 := f.addBlock(3, b2, true, true)
	f.storeEnvelope(b3)
	b4 := f.addBlock(4, b3, true, true)
	f.storeEnvelope(b4)
	b5 := f.addBlock(5, b4, true, true)
	f.storeEnvelope(b5)
	f.chain.setHead(b5.root)

	// Manufacture a covered interval whose lower bound sits above the coming
	// common ancestor, as after a small head seed.
	require.NoError(t, f.db.SaveEnvelopeCoverage(ctx, &dbval.EnvelopeCoverage{
		FormatVersion:  1,
		LowSlot:        4,
		HighSlot:       5,
		HighAnchorRoot: b5.root[:],
	}))
	s := newTestService(t, f, 5)
	require.NoError(t, s.store.load(ctx))
	require.Equal(t, primitives.Slot(4), s.Snapshot().Low)

	// Reorg with common ancestor at slot 2, below low: there is no valid
	// shrink, so the interval is discarded and re-anchored empty at the new
	// canonical head before reseeding.
	f.chain.setCanonical(b3.root, false)
	f.chain.setCanonical(b4.root, false)
	f.chain.setCanonical(b5.root, false)
	n3 := f.addBlock(3, b2, true, true)
	f.storeEnvelope(n3)
	f.chain.setHead(n3.root)

	epochBefore := s.ServeEpoch()
	s.reconcile(ctx)
	snap := s.Snapshot()
	assert.Equal(t, n3.root, snap.AnchorRoot)
	assert.Equal(t, primitives.Slot(3), snap.High)
	// The reseed after the destructive re-anchor extends back down to the
	// retention floor on the surviving branch.
	assert.Equal(t, primitives.Slot(1), snap.Low)
	assert.Equal(t, true, s.ServeEpoch() > epochBefore)
	assert.DeepEqual(t, []primitives.Slot{1, 2}, indexedSlots(t, f))
}

func TestFloorRaiseIsDestructiveAndPrunes(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	// Two old blocks provide stale index keys below the future floor.
	b2 := f.addBlock(2, nil, false, true)
	f.storeEnvelope(b2)
	b3 := f.addBlock(3, b2, true, true)
	f.storeEnvelope(b3)

	// A canonical chain near the retention frontier.
	parent := b3
	var last *fxBlock
	for slot := primitives.Slot(97); slot <= 100; slot++ {
		last = f.addBlock(slot, parent, true, true)
		f.storeEnvelope(last)
		parent = last
	}
	f.chain.setHead(last.root)

	// Manufacture a record spanning both regions with index entries below
	// the future floor.
	_, fp2, err := f.db.ExecutionPayloadEnvelopeWithFingerprint(ctx, b2.root)
	require.NoError(t, err)
	require.NoError(t, f.db.CommitEnvelopeCoverage(ctx, &dbval.EnvelopeCoverage{
		FormatVersion:  1,
		LowSlot:        2,
		HighSlot:       100,
		HighAnchorRoot: last.root[:],
	}, []dbiface.EnvelopeIndexReplacement{
		{Start: 2, End: 3, Entries: []dbiface.RevealedEnvelopeIndexEntry{{Slot: 2, Root: b2.root, PrimaryFingerprint: fp2}}},
	}))

	// Current slot far enough that the epoch-floored retention offset is 96.
	retention := uint64(params.BeaconConfig().MinEpochsForBlockRequests)
	spe := uint64(params.BeaconConfig().SlotsPerEpoch)
	current := primitives.Slot((retention + 3) * spe)
	s := newTestService(t, f, current)

	epochBefore := s.ServeEpoch()
	s.reconcile(ctx)
	snap := s.Snapshot()
	assert.Equal(t, primitives.Slot(3*spe), snap.Low)
	assert.Equal(t, primitives.Slot(100), snap.High)
	// Raising the floor invalidates published contents and prunes stale
	// index keys below the new bound in bounded pages.
	assert.Equal(t, true, s.ServeEpoch() > epochBefore)
	roots, err := f.db.RevealedEnvelopeRoots(ctx, 0, snap.Low, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, len(roots))
}

func TestUninitializedUntilGloasHead(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	s := newTestService(t, f, 3)
	// No head at all: nothing happens.
	s.reconcile(ctx)
	assert.Equal(t, false, s.Snapshot().Initialized)

	// A durable canonical Gloas head bootstraps an empty interval.
	b1 := f.addBlock(1, nil, false, true)
	f.storeEnvelope(b1)
	f.chain.setHead(b1.root)
	s.reconcile(ctx)
	snap := s.Snapshot()
	require.Equal(t, true, snap.Initialized)
	assert.Equal(t, primitives.Slot(1), snap.Low)
	assert.Equal(t, primitives.Slot(1), snap.High)
	assert.Equal(t, b1.root, snap.AnchorRoot)
}

func TestCoherentServeRead(t *testing.T) {
	gloasTestConfig(t)
	f := newFixture(t)
	ctx := context.Background()

	parent := (*fxBlock)(nil)
	var head *fxBlock
	for slot := primitives.Slot(1); slot <= 8; slot++ {
		head = f.addBlock(slot, parent, parent != nil, true)
		f.storeEnvelope(head)
		parent = head
	}
	f.chain.setHead(head.root)
	s := newTestService(t, f, 8)
	s.reconcile(ctx)
	require.Equal(t, primitives.Slot(8), s.Snapshot().High)

	// wEnd below high: the proven region is [begin, wEnd+1) — the guarded
	// increment never reaches past high.
	read, err := s.CoherentServeRead(ctx, 2, 5, 10)
	require.NoError(t, err)
	require.Equal(t, 4, len(read.Roots))
	assert.Equal(t, primitives.Slot(2), read.Roots[0].Slot)
	assert.Equal(t, primitives.Slot(5), read.Roots[3].Slot)
	assert.Equal(t, head.root, read.HeadRoot)
	assert.Equal(t, s.ServeEpoch(), read.Epoch)

	// wEnd at/above high: the proven region ends at high, and quota caps the
	// copied roots.
	read, err = s.CoherentServeRead(ctx, 2, 100, 2)
	require.NoError(t, err)
	require.Equal(t, 2, len(read.Roots))
	assert.Equal(t, primitives.Slot(2), read.Roots[0].Slot)
	assert.Equal(t, primitives.Slot(3), read.Roots[1].Slot)

	// A begin below the covered lower bound copies no roots; the handler
	// refuses on the snapshot bounds.
	read, err = s.CoherentServeRead(ctx, 0, 5, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, len(read.Roots))
}

func TestLifecycle(t *testing.T) {
	t.Run("stop before clock returns promptly", func(t *testing.T) {
		gloasTestConfig(t)
		f := newFixture(t)
		s, err := New(context.Background(), WithDatabase(f.db), WithClockWaiter(startup.NewClockSynchronizer()))
		require.NoError(t, err)
		s.Start()
		done := make(chan struct{})
		go func() {
			require.NoError(t, s.Stop())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Stop did not return while waiting for the clock")
		}
		// Late notifications into the never-closed notifier are no-ops.
		s.Notifier().Notify()
		s.Notifier().NotifyHead()
		require.NoError(t, s.Status())
	})

	t.Run("unscheduled gloas is a no-op", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MainnetConfig().Copy())
		f := newFixture(t)
		cs := startup.NewClockSynchronizer()
		s, err := New(context.Background(), WithDatabase(f.db), WithClockWaiter(cs))
		require.NoError(t, err)
		s.Start()
		require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), [32]byte{})))
		// The runtime observes the unscheduled fork and exits cleanly.
		require.NoError(t, s.Stop())
		require.NoError(t, s.Status())
		assert.Equal(t, false, s.Snapshot().Initialized)
	})

	t.Run("start-driven scan settles from notifications", func(t *testing.T) {
		gloasTestConfig(t)
		f := newFixture(t)
		parent := (*fxBlock)(nil)
		var head *fxBlock
		for slot := primitives.Slot(1); slot <= 5; slot++ {
			head = f.addBlock(slot, parent, parent != nil, true)
			f.storeEnvelope(head)
			parent = head
		}
		f.chain.setHead(head.root)

		cs := startup.NewClockSynchronizer()
		s, err := New(context.Background(),
			WithDatabase(f.db),
			WithClockWaiter(cs),
			WithScanBudgets(4, 8, 1<<20),
			WithInterPageYield(0),
			WithTickInterval(50*time.Millisecond),
		)
		require.NoError(t, err)
		s.SetChainView(f.chain)
		s.Start()
		defer func() {
			require.NoError(t, s.Stop())
		}()
		require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(5)))))

		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			snap := s.Snapshot()
			if snap.Initialized && snap.High == 5 && snap.Low == 1 {
				break
			}
			s.Notifier().Notify()
			time.Sleep(10 * time.Millisecond)
		}
		snap := s.Snapshot()
		require.Equal(t, true, snap.Initialized)
		assert.Equal(t, primitives.Slot(5), snap.High)
		assert.Equal(t, primitives.Slot(1), snap.Low)
	})
}
