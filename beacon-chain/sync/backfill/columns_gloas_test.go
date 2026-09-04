package backfill

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// setupGloasFullnessConfig ensures the fulu and gloas fork epochs are set and returns the first
// gloas slot, so fixtures live at actual gloas slots covered by the column retention window.
func setupGloasFullnessConfig(t *testing.T) primitives.Slot {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	if cfg.FuluForkEpoch == cfg.FarFutureEpoch {
		cfg.FuluForkEpoch = cfg.DenebForkEpoch + 4096*2
	}
	if cfg.GloasForkEpoch == cfg.FarFutureEpoch {
		cfg.GloasForkEpoch = cfg.FuluForkEpoch + 8
	}
	gloasSlot, err := slots.EpochStart(cfg.GloasForkEpoch)
	require.NoError(t, err)
	return gloasSlot
}

// gloasBlockWithBid builds a gloas ROBlock whose bid carries nCmts KZG commitments plus the
// given payload hashes, which is everything payload fullness classification reads.
func gloasBlockWithBid(t *testing.T, slot primitives.Slot, parentRoot [32]byte, blockHash, parentBlockHash byte, nCmts int) blocks.ROBlock {
	pb := util.NewBeaconBlockGloas()
	pb.Block.Slot = slot
	pb.Block.ParentRoot = parentRoot[:]
	bid := pb.Block.Body.SignedExecutionPayloadBid.Message
	bid.BlockHash = make([]byte, 32)
	bid.BlockHash[0] = blockHash
	bid.ParentBlockHash = make([]byte, 32)
	bid.ParentBlockHash[0] = parentBlockHash
	bid.BlobKzgCommitments = make([][]byte, nCmts)
	for i := range bid.BlobKzgCommitments {
		bid.BlobKzgCommitments[i] = make([]byte, 48)
	}
	sb, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	rob, err := blocks.NewROBlock(sb)
	require.NoError(t, err)
	return rob
}

func testCustodyP2P(t *testing.T) (*p2ptest.TestP2P, peerdas.ColumnIndices) {
	p := p2ptest.NewTestP2P(t)
	_, _, err := p.UpdateCustodyInfo(0, 4)
	require.NoError(t, err)
	custody, err := currentCustodiedColumns(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, true, custody.Count() > 0)
	return p, custody
}

// TestBuildColumnBatchGloasFullness covers how gloas payload fullness drives column requirements:
// an interior revealed block requires custody columns, an interior withheld block requires none,
// and the batch tail without a child stays unknown without holding the batch in column sync.
func TestBuildColumnBatchGloasFullness(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)

	// Three parent-linked gloas blocks with skipped slots between them:
	// a's payload (hash 0xaa) is proven revealed by b's bid building on it.
	// b's payload (hash 0xbb) is proven withheld: c's bid builds on 0xaa, not 0xbb.
	// c is the batch tail, so no in-batch child can testify either way.
	a := gloasBlockWithBid(t, gloasSlot, [32]byte{0x99}, 0xaa, 0x99, 1)
	b := gloasBlockWithBid(t, gloasSlot+3, a.Root(), 0xbb, 0xaa, 1)
	c := gloasBlockWithBid(t, gloasSlot+7, b.Root(), 0xcc, 0xaa, 1)
	blks := verifiedROBlocks{a, b, c}

	ctx := t.Context()
	p, custody := testCustodyP2P(t)
	store := filesystem.NewEphemeralDataColumnStorage(t)
	bt := batch{begin: gloasSlot, end: gloasSlot + 10}
	cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), nil)
	require.NoError(t, err)
	require.NotNil(t, cb)
	require.Equal(t, 3, len(cb.toDownload))

	// a: payload revealed, so its custody columns are required.
	require.Equal(t, fullnessRevealed, cb.toDownload[a.Root()].fullness)
	require.Equal(t, custody.Count(), cb.toDownload[a.Root()].remaining.Count())
	// b: payload withheld, so no columns exist and none are required.
	require.Equal(t, fullnessWithheld, cb.toDownload[b.Root()].fullness)
	require.Equal(t, 0, cb.toDownload[b.Root()].remaining.Count())
	// c: batch tail with no child, fullness unknown; it must not require columns.
	require.Equal(t, fullnessUnknown, cb.toDownload[c.Root()].fullness)
	require.Equal(t, 0, cb.toDownload[c.Root()].remaining.Count())

	// Only a's columns are needed; once they are downloaded the unknown tail must not keep
	// the batch in the column-fetch state.
	require.Equal(t, custody.Count(), cb.needed().Count())
	cb.toDownload[a.Root()].remaining = peerdas.NewColumnIndices()
	require.Equal(t, 0, cb.needed().Count())
	bt.blocks = blks
	bt.columns = &columnSync{columnBatch: cb}
	require.Equal(t, batchImportable, bt.transitionToNext().state)
}

// TestBuildColumnBatchGloasPreGloasUnchanged verifies pre-gloas blocks are always treated as
// revealed and gloas blocks without commitments are skipped entirely.
func TestBuildColumnBatchGloasPreGloasUnchanged(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)
	fuluSlot, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)

	ctx := t.Context()
	p, custody := testCustodyP2P(t)
	store := filesystem.NewEphemeralDataColumnStorage(t)

	t.Run("pre-gloas blocks are revealed", func(t *testing.T) {
		blks, _ := testBlobGen(t, fuluSlot, 2)
		cb, err := buildColumnBatch(ctx, batch{begin: fuluSlot, end: fuluSlot + 10}, blks, p, store, mockCurrentSpecNeeds(), nil)
		require.NoError(t, err)
		require.NotNil(t, cb)
		require.Equal(t, 2, len(cb.toDownload))
		for _, td := range cb.toDownload {
			// The batch tail rule only applies to gloas blocks; pre-gloas payloads are always revealed.
			require.Equal(t, fullnessRevealed, td.fullness)
			require.Equal(t, custody.Count(), td.remaining.Count())
		}
	})

	t.Run("gloas blocks without commitments are skipped", func(t *testing.T) {
		a := gloasBlockWithBid(t, gloasSlot, [32]byte{0x99}, 0xaa, 0x99, 1)
		b := gloasBlockWithBid(t, gloasSlot+1, a.Root(), 0xbb, 0xaa, 0)
		cb, err := buildColumnBatch(ctx, batch{begin: gloasSlot, end: gloasSlot + 10}, verifiedROBlocks{a, b}, p, store, mockCurrentSpecNeeds(), nil)
		require.NoError(t, err)
		require.NotNil(t, cb)
		require.Equal(t, 1, len(cb.toDownload))
		require.NotNil(t, cb.toDownload[a.Root()])
	})
}

// TestBuildColumnBatchBoundaryChild verifies that a direct boundary child can prove the batch
// tail revealed or withheld at build time, and that a missing or failing lookup never
// defaults the tail to revealed.
func TestBuildColumnBatchBoundaryChild(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)

	a := gloasBlockWithBid(t, gloasSlot, [32]byte{0x99}, 0xaa, 0x99, 1)
	tail := gloasBlockWithBid(t, gloasSlot+1, a.Root(), 0xcc, 0xaa, 1)
	blks := verifiedROBlocks{a, tail}

	ctx := t.Context()
	p, custody := testCustodyP2P(t)
	store := filesystem.NewEphemeralDataColumnStorage(t)
	bt := batch{begin: gloasSlot, end: gloasSlot + 10}

	childFn := func(child blocks.ROBlock) boundaryChildFn {
		return func(_ context.Context, parentRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			require.Equal(t, tail.Root(), parentRoot)
			return child, nil
		}
	}

	t.Run("child proves tail revealed", func(t *testing.T) {
		child := gloasBlockWithBid(t, gloasSlot+2, tail.Root(), 0xdd, 0xcc, 0)
		cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), childFn(child))
		require.NoError(t, err)
		require.Equal(t, fullnessRevealed, cb.toDownload[tail.Root()].fullness)
		require.Equal(t, custody.Count(), cb.toDownload[tail.Root()].remaining.Count())
	})

	t.Run("child proves tail withheld", func(t *testing.T) {
		child := gloasBlockWithBid(t, gloasSlot+2, tail.Root(), 0xdd, 0xaa, 0)
		cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), childFn(child))
		require.NoError(t, err)
		require.Equal(t, fullnessWithheld, cb.toDownload[tail.Root()].fullness)
		require.Equal(t, 0, cb.toDownload[tail.Root()].remaining.Count())
	})

	t.Run("no child leaves tail unknown", func(t *testing.T) {
		nilFn := func(context.Context, [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			return nil, nil
		}
		cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), nilFn)
		require.NoError(t, err)
		require.Equal(t, fullnessUnknown, cb.toDownload[tail.Root()].fullness)
		require.Equal(t, 0, cb.toDownload[tail.Root()].remaining.Count())
	})

	t.Run("child lookup failure is best-effort and leaves tail unknown", func(t *testing.T) {
		errFn := func(context.Context, [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			return nil, errors.New("db unavailable")
		}
		cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), errFn)
		require.NoError(t, err)
		require.Equal(t, fullnessUnknown, cb.toDownload[tail.Root()].fullness)
		require.Equal(t, 0, cb.toDownload[tail.Root()].remaining.Count())
	})
}

// TestStoreBoundaryChild verifies boundaryChild only returns the block at BackfillStatus.LowRoot
// when it is the direct child of the requested root.
func TestStoreBoundaryChild(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)
	ctx := t.Context()
	tail := gloasBlockWithBid(t, gloasSlot, [32]byte{0x01}, 0xcc, 0x99, 1)
	tailRoot := tail.Root()
	child := gloasBlockWithBid(t, gloasSlot+1, tailRoot, 0xdd, 0xcc, 0)

	status := func() *dbval.BackfillStatus {
		return &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 1), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}
	}

	t.Run("direct child returned", func(t *testing.T) {
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
		su := &Store{store: mdb, bs: status()}
		got, err := su.boundaryChild(ctx, tailRoot)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, tailRoot, got.Block().ParentRoot())
	})

	t.Run("not the direct child returns nil", func(t *testing.T) {
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
		su := &Store{store: mdb, bs: status()}
		got, err := su.boundaryChild(ctx, [32]byte{0xff})
		require.NoError(t, err)
		require.IsNil(t, got)
	})

	t.Run("missing child block errors", func(t *testing.T) {
		su := &Store{store: &mockBackfillDB{}, bs: status()}
		_, err := su.boundaryChild(ctx, tailRoot)
		require.NotNil(t, err)
	})

	t.Run("child with mismatched parent root errors", func(t *testing.T) {
		other := gloasBlockWithBid(t, gloasSlot+1, [32]byte{0xab}, 0xdd, 0xcc, 0)
		bs := status()
		bs.LowRoot = other.RootSlice()
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{other.Root(): other}}
		su := &Store{store: mdb, bs: bs}
		_, err := su.boundaryChild(ctx, tailRoot)
		require.ErrorIs(t, err, errBatchDisconnected)
	})
}

// TestCheckMultiplexerGloasFullness verifies that import-time DA checking receives revealed gloas
// blocks, skips withheld ones, and refuses to guess for unresolved fullness.
func TestCheckMultiplexerGloasFullness(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)

	a := gloasBlockWithBid(t, gloasSlot, [32]byte{0x99}, 0xaa, 0x99, 1)
	b := gloasBlockWithBid(t, gloasSlot+3, a.Root(), 0xbb, 0xaa, 1)
	c := gloasBlockWithBid(t, gloasSlot+7, b.Root(), 0xcc, 0xaa, 1)
	blks := verifiedROBlocks{a, b, c}

	ctx := t.Context()
	p, _ := testCustodyP2P(t)
	store := filesystem.NewEphemeralDataColumnStorage(t)
	bt := batch{begin: gloasSlot, end: gloasSlot + 10, blocks: blks}
	cb, err := buildColumnBatch(ctx, bt, blks, p, store, mockCurrentSpecNeeds(), nil)
	require.NoError(t, err)
	bt.columns = &columnSync{columnBatch: cb}

	mux := newCheckMultiplexer(mockCurrentSpecNeeds(), bt)
	tracker := NewTrackingAvailabilityChecker(&das.MockAvailabilityStore{})
	mux.colCheck = tracker

	// The unresolved tail must fail the check retryably instead of being imported on a guess.
	err = mux.IsDataAvailable(ctx, gloasSlot+10, blks...)
	require.ErrorIs(t, err, errUnresolvedPayloadFullness)
	require.Equal(t, true, isRetryable(err))
	require.Equal(t, 0, tracker.GetCallCount())

	// Tail proven withheld: only the revealed block reaches the column checker.
	cb.toDownload[c.Root()].fullness = fullnessWithheld
	require.NoError(t, mux.IsDataAvailable(ctx, gloasSlot+10, blks...))
	require.Equal(t, 1, tracker.GetCallCount())
	seen := tracker.GetBlocksInCall(0)
	require.Equal(t, 1, len(seen))
	require.Equal(t, a.Root(), seen[0].Root())

	// Tail proven revealed: it is checked alongside the other revealed block.
	cb.toDownload[c.Root()].fullness = fullnessRevealed
	require.NoError(t, mux.IsDataAvailable(ctx, gloasSlot+10, blks...))
	require.Equal(t, 2, tracker.GetCallCount())
	seen = tracker.GetBlocksInCall(1)
	require.Equal(t, 2, len(seen))
	require.Equal(t, a.Root(), seen[0].Root())
	require.Equal(t, c.Root(), seen[1].Root())
}

// TestDefaultBatchImporterGloasTail verifies import-time resolution of an unknown gloas tail
// against the already-imported canonical child at BackfillStatus.LowRoot.
func TestDefaultBatchImporterGloasTail(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)
	current := gloasSlot + 100
	ctx := t.Context()

	newImporterService := func(t *testing.T, currentSlot primitives.Slot) *Service {
		sn, err := das.NewSyncNeeds(func() primitives.Slot { return currentSlot }, nil, 0)
		require.NoError(t, err)
		return &Service{syncNeeds: sn, dcStore: filesystem.NewEphemeralDataColumnStorage(t)}
	}

	// newTailBatch builds a single-block batch whose gloas tail has unknown fullness.
	newTailBatch := func(t *testing.T, p *p2ptest.TestP2P, tail blocks.ROBlock) batch {
		store := filesystem.NewEphemeralDataColumnStorage(t)
		bt := batch{begin: gloasSlot, end: gloasSlot + 10, blocks: verifiedROBlocks{tail}}
		cb, err := buildColumnBatch(ctx, bt, bt.blocks, p, store, mockCurrentSpecNeeds(), nil)
		require.NoError(t, err)
		require.Equal(t, fullnessUnknown, cb.toDownload[tail.Root()].fullness)
		bt.columns = &columnSync{columnBatch: cb}
		return bt
	}

	tail := gloasBlockWithBid(t, gloasSlot+2, [32]byte{0x99}, 0xcc, 0x99, 1)
	tailRoot := tail.Root()

	t.Run("tail resolved withheld imports without columns", func(t *testing.T) {
		p, _ := testCustodyP2P(t)
		bt := newTailBatch(t, p, tail)
		// The child's bid builds on 0x99, proving the tail's payload (0xcc) was withheld.
		child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0x99, 0)
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
		su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
		svc := newImporterService(t, current)

		st, err := svc.defaultBatchImporter(ctx, current, bt, su)
		require.NoError(t, err)
		require.Equal(t, fullnessWithheld, bt.columns.fullness(tailRoot))
		require.Equal(t, uint64(gloasSlot+2), st.LowSlot)
		_, saved := mdb.blocks[tailRoot]
		require.Equal(t, true, saved)
	})

	t.Run("tail resolved revealed is promoted and cannot import prematurely", func(t *testing.T) {
		p, custody := testCustodyP2P(t)
		bt := newTailBatch(t, p, tail)
		// The child's bid builds on 0xcc, proving the tail's payload was revealed.
		child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
		su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
		svc := newImporterService(t, current)

		_, err := svc.defaultBatchImporter(ctx, current, bt, su)
		require.ErrorIs(t, err, errTailColumnsNeeded)
		td := bt.columns.blockColumns(tailRoot)
		require.Equal(t, fullnessRevealed, td.fullness)
		require.Equal(t, custody.Count(), td.remaining.Count())
		_, saved := mdb.blocks[tailRoot]
		require.Equal(t, false, saved)
		require.Equal(t, uint64(gloasSlot+3), su.status().LowSlot)

		// Still missing columns on the next attempt: the batch must keep failing to import.
		_, err = svc.defaultBatchImporter(ctx, current, bt, su)
		require.ErrorIs(t, err, errTailColumnsNeeded)

		// Even if the fetch state is cleared without the columns being persisted, the DA check
		// must still refuse to import the revealed tail.
		td.remaining = peerdas.NewColumnIndices()
		bt.columns.store = das.NewLazilyPersistentStoreColumn(
			svc.dcStore, nil, p.NodeID(), 4,
			newColumnBisector(func(peer.ID, string, error) {}),
			func(primitives.Slot) bool { return true },
		)
		_, err = svc.defaultBatchImporter(ctx, current, bt, su)
		require.NotNil(t, err)
		require.Equal(t, false, errors.Is(err, errTailColumnsNeeded))
		_, saved = mdb.blocks[tailRoot]
		require.Equal(t, false, saved)
		require.Equal(t, uint64(gloasSlot+3), su.status().LowSlot)
	})

	t.Run("promoted columns downloaded and verified allow the next import", func(t *testing.T) {
		p, custody := testCustodyP2P(t)
		bt := newTailBatch(t, p, tail)
		// The child's bid builds on 0xcc, proving the tail's payload was revealed.
		child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
		mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
		su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
		svc := newImporterService(t, current)
		// Attach the batch availability store the same way newColumnSync does.
		bt.columns.store = das.NewLazilyPersistentStoreColumn(
			svc.dcStore, markAllVerified(&verification.MockDataColumnsVerifier{}), p.NodeID(), 4,
			newColumnBisector(func(peer.ID, string, error) {}),
			func(primitives.Slot) bool { return true },
		)

		_, err := svc.defaultBatchImporter(ctx, current, bt, su)
		require.ErrorIs(t, err, errTailColumnsNeeded)
		td := bt.columns.blockColumns(tailRoot)
		require.Equal(t, custody.Count(), td.remaining.Count())

		// Simulate the worker downloading the promoted columns: each sidecar is persisted to
		// the batch availability store and cleared from remaining, as countedValidation does.
		for _, idx := range td.remaining.ToSlice() {
			sc := &ethpb.DataColumnSidecarGloas{
				Index:           idx,
				Slot:            tail.Block().Slot(),
				BeaconBlockRoot: tailRoot[:],
				Column:          [][]byte{make([]byte, 2048)},
				KzgProofs:       [][]byte{make([]byte, 48)},
			}
			ro, err := blocks.NewRODataColumnGloasWithRoot(sc, tailRoot)
			require.NoError(t, err)
			require.NoError(t, bt.columns.store.Persist(current, ro))
			td.remaining.Unset(idx)
		}
		require.Equal(t, 0, td.remaining.Count())

		// The next import verifies and persists the columns, then succeeds.
		st, err := svc.defaultBatchImporter(ctx, current, bt, su)
		require.NoError(t, err)
		require.Equal(t, uint64(gloasSlot+2), st.LowSlot)
		_, saved := mdb.blocks[tailRoot]
		require.Equal(t, true, saved)
		sum := svc.dcStore.Summary(tailRoot)
		for _, idx := range custody.ToSlice() {
			require.Equal(t, true, sum.HasIndex(idx))
		}
	})

	t.Run("missing boundary child defers import", func(t *testing.T) {
		p, _ := testCustodyP2P(t)
		bt := newTailBatch(t, p, tail)
		child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
		// The status names the child, but it cannot be read from the db.
		mdb := &mockBackfillDB{}
		su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
		svc := newImporterService(t, current)

		_, err := svc.defaultBatchImporter(ctx, current, bt, su)
		require.NotNil(t, err)
		require.Equal(t, false, errors.Is(err, errTailColumnsNeeded))
		require.Equal(t, true, isRetryable(err))
		require.Equal(t, fullnessUnknown, bt.columns.fullness(tailRoot))
		require.Equal(t, uint64(gloasSlot+3), su.status().LowSlot)
	})

	t.Run("non-child boundary block is never assumed full", func(t *testing.T) {
		p, _ := testCustodyP2P(t)
		bt := newTailBatch(t, p, tail)
		// The lowest imported block does not descend from the tail.
		su := &Store{store: &mockBackfillDB{}, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: []byte{0xee}, LowParentRoot: []byte{0xff}}}
		svc := newImporterService(t, current)

		err := svc.resolveTailFullness(ctx, su, bt, svc.syncNeeds.Currently())
		require.ErrorIs(t, err, errChainBroken)
		require.Equal(t, fullnessUnknown, bt.columns.fullness(tailRoot))
		require.Equal(t, 0, bt.columns.blockColumns(tailRoot).remaining.Count())
	})

	// The tail was inside the column retention window when the batch was built, but leaves it
	// before import: no columns are required and no promotion may occur.
	t.Run("tail expired before import", func(t *testing.T) {
		retentionSlots := slots.UnsafeEpochStart(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
		expiredCurrent := tail.Block().Slot() + retentionSlots + 100

		t.Run("unknown tail imports without resolution", func(t *testing.T) {
			p, _ := testCustodyP2P(t)
			bt := newTailBatch(t, p, tail)
			// No child block is available at all; expiry must make resolution unnecessary.
			child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
			mdb := &mockBackfillDB{}
			su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
			svc := newImporterService(t, expiredCurrent)

			st, err := svc.defaultBatchImporter(ctx, expiredCurrent, bt, su)
			require.NoError(t, err)
			require.Equal(t, uint64(gloasSlot+2), st.LowSlot)
			require.Equal(t, fullnessUnknown, bt.columns.fullness(tailRoot))
			_, saved := mdb.blocks[tailRoot]
			require.Equal(t, true, saved)
		})

		t.Run("promoted revealed tail is not re-required", func(t *testing.T) {
			p, custody := testCustodyP2P(t)
			bt := newTailBatch(t, p, tail)
			child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
			mdb := &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{child.Root(): child}}
			su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
			// The tail was proven revealed and promoted while still retained.
			td := bt.columns.blockColumns(tailRoot)
			td.fullness = fullnessRevealed
			td.remaining = custody.Copy()
			svc := newImporterService(t, expiredCurrent)

			st, err := svc.defaultBatchImporter(ctx, expiredCurrent, bt, su)
			require.NoError(t, err)
			require.Equal(t, uint64(gloasSlot+2), st.LowSlot)
			_, saved := mdb.blocks[tailRoot]
			require.Equal(t, true, saved)
		})
	})
}

// TestPromotedTailColumnExpiryInPool verifies the column state-machine path: when the column
// retention window advances past a promoted tail while its block is still required, the pool
// prunes the expired column work and hands the batch back as importable without needing any
// suitable peer or column RPC, and the batch subsequently imports. This must hold both for a
// batch waiting in batchSyncColumns and for one parked in batchErrRetryable by a failed fetch.
func TestPromotedTailColumnExpiryInPool(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)
	ctx := t.Context()
	tail := gloasBlockWithBid(t, gloasSlot+2, [32]byte{0x99}, 0xcc, 0x99, 1)
	tailRoot := tail.Root()

	// Column retention advances past the tail slot while the block itself remains required.
	retentionSlots := slots.UnsafeEpochStart(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	expiredCurrent := tail.Block().Slot() + retentionSlots + 100
	sn, err := das.NewSyncNeeds(func() primitives.Slot { return expiredCurrent }, nil, 0)
	require.NoError(t, err)

	// newPromotedBatch builds a batch, constructed while the tail was still retained, whose
	// revealed tail was promoted with outstanding column work.
	newPromotedBatch := func(t *testing.T) (batch, *p2ptest.TestP2P) {
		p, custody := testCustodyP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)
		bt := batch{begin: gloasSlot, end: gloasSlot + 10, blocks: verifiedROBlocks{tail}}
		cb, err := buildColumnBatch(ctx, bt, bt.blocks, p, store, mockCurrentSpecNeeds(), nil)
		require.NoError(t, err)
		bt.columns = &columnSync{columnBatch: cb}
		td := cb.toDownload[tailRoot]
		td.fullness = fullnessRevealed
		td.remaining = custody.Copy()
		bt.state = batchSyncColumns
		require.Equal(t, true, len(bt.columns.columnsNeeded()) > 0)
		needs := sn.Currently()
		require.Equal(t, true, needs.Block.At(bt.end-1))
		require.Equal(t, false, needs.Col.At(tail.Block().Slot()))
		return bt, p
	}

	// completesWithoutPeers runs the state-machine step with no suitable peers and requires the
	// batch to come back importable with no column work left.
	completesWithoutPeers := func(t *testing.T, p *p2ptest.TestP2P, bt batch) batch {
		pool := newP2PBatchWorkerPool(p, 2, sn.Currently)
		remaining, err := pool.processTodo([]batch{bt}, &mockAssigner{err: peers.ErrInsufficientSuitable}, make(map[peer.ID]bool))
		require.NoError(t, err)
		require.Equal(t, 0, len(remaining))
		select {
		case got := <-pool.fromRouter:
			require.Equal(t, batchImportable, got.state)
			require.Equal(t, 0, len(got.columns.columnsNeeded()))
			return got
		default:
			t.Fatal("expected the expired-column batch to be handed back as importable")
			return batch{}
		}
	}

	t.Run("waiting in column sync", func(t *testing.T) {
		bt, p := newPromotedBatch(t)
		got := completesWithoutPeers(t, p, bt)

		// The batch subsequently imports through the service using the real importer. The empty
		// db also proves no boundary child lookup is attempted for the expired tail.
		child := gloasBlockWithBid(t, gloasSlot+3, tailRoot, 0xdd, 0xcc, 0)
		mdb := &mockBackfillDB{}
		su := &Store{store: mdb, bs: &dbval.BackfillStatus{LowSlot: uint64(gloasSlot + 3), LowRoot: child.RootSlice(), LowParentRoot: tailRoot[:]}}
		seqr := newBatchSequencer(1, gloasSlot+10, 10, sn.Currently)
		seqr.seq[0] = got
		svc := &Service{
			clock:     startup.NewClock(time.Now(), [32]byte{}),
			pool:      &mockPool{todoChan: make(chan batch, 1)},
			batchSeq:  seqr,
			store:     su,
			syncNeeds: sn,
			dcStore:   filesystem.NewEphemeralDataColumnStorage(t),
		}
		svc.batchImporter = svc.defaultBatchImporter
		svc.importBatches(ctx)

		require.Equal(t, uint64(gloasSlot+2), su.status().LowSlot)
		_, saved := mdb.blocks[tailRoot]
		require.Equal(t, true, saved)
	})

	t.Run("parked retryable by a failed column fetch", func(t *testing.T) {
		bt, p := newPromotedBatch(t)
		bt = bt.withRetryableError(errors.New("column RPC timeout"))
		require.Equal(t, batchErrRetryable, bt.state)
		require.Equal(t, batchSyncColumns, bt.retryFrom)

		completesWithoutPeers(t, p, bt)
	})
}

// TestImportBatchesTailPromotion verifies that a batch failing import with errTailColumnsNeeded
// is sent back through the worker pool in the column-sync state instead of being retried from scratch.
func TestImportBatchesTailPromotion(t *testing.T) {
	gloasSlot := setupGloasFullnessConfig(t)
	tail := gloasBlockWithBid(t, gloasSlot+2, [32]byte{0x99}, 0xcc, 0x99, 1)

	needsCb := mockCurrentNeedsFunc(1, primitives.Slot(math.MaxUint64))
	seqr := newBatchSequencer(1, gloasSlot+10, 10, needsCb)
	ib := batch{begin: gloasSlot, end: gloasSlot + 10, state: batchImportable, seq: 1, blocks: verifiedROBlocks{tail}}
	seqr.seq[0] = ib

	pool := &mockPool{todoChan: make(chan batch, 1)}
	svc := &Service{
		clock:    startup.NewClock(time.Now(), [32]byte{}),
		pool:     pool,
		batchSeq: seqr,
		store:    &Store{bs: &dbval.BackfillStatus{}},
		batchImporter: func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
			return nil, errTailColumnsNeeded
		},
	}
	svc.importBatches(t.Context())

	select {
	case got := <-pool.todoChan:
		require.Equal(t, batchSyncColumns, got.state)
		require.Equal(t, ib.begin, got.begin)
		require.Equal(t, ib.end, got.end)
	default:
		t.Fatal("expected the promoted batch to be sent back to the worker pool")
	}
	require.Equal(t, batchSyncColumns, seqr.seq[0].state)
}
