package stategen

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	blt "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestStateByRoot_GenesisState(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	b := util.NewBeaconBlock()
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState, err := service.StateByRoot(ctx, params.BeaconConfig().ZeroHash) // Zero hash is genesis state root.
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestStateByRoot_ColdState(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.finalizedInfo.slot = 2
	service.slotsPerArchivedPoint = 1

	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	val, err := beaconState.ValidatorAtIndex(0)
	require.NoError(t, err)
	val.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(0, val))
	roval, err := beaconState.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)
	require.Equal(t, true, roval.Slashed())

	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState, err := service.StateByRoot(ctx, bRoot)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())

	bal, err := service.ActiveNonSlashedBalancesByRoot(ctx, bRoot)
	require.NoError(t, err)
	require.Equal(t, 32, len(bal))
	for _, balance := range bal[1:] {
		require.Equal(t, params.BeaconConfig().MaxEffectiveBalance, balance)
	}
	require.Equal(t, uint64(0), bal[0])
}

func TestStateByRootIfCachedNoCopy_HotState(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	r := [32]byte{'A'}
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Root: r[:]}))
	service.hotStateCache.put(r, beaconState)

	loadedState := service.StateByRootIfCachedNoCopy(r)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestStateByRootIfCachedNoCopy_ColdState(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.finalizedInfo.slot = 2
	service.slotsPerArchivedPoint = 1

	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState := service.StateByRootIfCachedNoCopy(bRoot)
	require.NoError(t, err)
	require.Equal(t, loadedState, nil)
}

func TestDeleteStateFromCaches(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	r := [32]byte{'A'}

	require.Equal(t, false, service.hotStateCache.has(r))
	_, has, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, false, has)

	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.epochBoundaryStateCache.put(r, beaconState))

	require.Equal(t, true, service.hotStateCache.has(r))
	_, has, err = service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, true, has)

	require.NoError(t, service.DeleteStateFromCaches(ctx, r))

	require.Equal(t, false, service.hotStateCache.has(r))
	_, has, err = service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, false, has)
}

// testChainSlot represents one slot of the test chain
type testChainSlot struct {
	st   state.BeaconState
	root [32]byte
	blk  blt.ROBlock
}

// testChain represents the test block chain that is written to the DB / cache.
// Used to test the StateByRoot, StateByRootInitSync and loadStateByRoot methods.
type testChain struct {
	t      *testing.T
	ctx    context.Context
	d      db.Database
	srv    *State
	cslots map[primitives.Slot]testChainSlot
}

// the following are helpers used in the test cases and helpers to concisely get the different
// components of the chain by slot.
func (c testChain) cslot(t *testing.T, s primitives.Slot) testChainSlot {
	cs, ok := c.cslots[s]
	require.Equal(t, true, ok, fmt.Sprintf("state not found for slot %d", s))
	return cs
}

func (c testChain) state(t *testing.T, s primitives.Slot) state.BeaconState {
	return c.cslot(t, s).st
}

func (c testChain) blockRoot(t *testing.T, s primitives.Slot) [32]byte {
	return c.cslot(t, s).root
}

func (c testChain) block(t *testing.T, s primitives.Slot) blt.ROBlock {
	return c.cslot(t, s).blk
}

type testSetupSlots struct {
	stateAt   primitives.Slot
	lastblock primitives.Slot
}

func TestLoadStateByRoot(t *testing.T) {
	ctx := t.Context()
	persistEpochBoundary := func(r testChain, slot primitives.Slot) {
		require.NoError(t, r.srv.epochBoundaryStateCache.put(r.blockRoot(r.t, slot), r.state(t, slot)))
	}
	persistHotStateCache := func(r testChain, slot primitives.Slot) {
		r.srv.hotStateCache.put(r.blockRoot(t, slot), r.state(t, slot))
	}
	persistDB := func(r testChain, slot primitives.Slot) {
		require.NoError(r.t, r.d.SaveState(r.ctx, r.state(t, slot), r.blockRoot(t, slot)))
	}
	persistFinalizedStruct := func(r testChain, slot primitives.Slot) {
		st := r.state(t, slot)
		r.srv.finalizedInfo.state = st
		r.srv.finalizedInfo.slot = st.Slot()
		r.srv.finalizedInfo.root = r.blockRoot(t, slot)
	}

	type testLoader func(r testChain) (state.BeaconState, error)
	lsbr := func(slot primitives.Slot) testLoader {
		return func(tc testChain) (state.BeaconState, error) {
			return tc.srv.loadStateByRoot(tc.ctx, tc.blockRoot(t, slot))
		}
	}
	sbrInit := func(slot primitives.Slot) testLoader {
		return func(tc testChain) (state.BeaconState, error) {
			return tc.srv.StateByRootInitialSync(tc.ctx, tc.blockRoot(t, slot))
		}
	}
	sbr := func(slot primitives.Slot) testLoader {
		return func(tc testChain) (state.BeaconState, error) {
			return tc.srv.StateByRoot(tc.ctx, tc.blockRoot(t, slot))
		}
	}

	cases := []struct {
		name         string
		slots        testSetupSlots
		persistState func(r testChain, s primitives.Slot)
		loader       testLoader // ie loadStateByRoot; StateByRootInitialSync; StateByRoot
	}{
		// loadStateByRoot tests
		{
			name:         "loadStateByRoot - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       lsbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "loadStateByRoot - with replay - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       lsbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "loadStateByRoot - using hot state cache",
			persistState: persistHotStateCache,
			loader:       lsbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "loadStateByRoot - with replay - using hot state cache",
			persistState: persistHotStateCache,
			loader:       lsbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "loadStateByRoot - using db",
			persistState: persistDB,
			loader:       lsbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "loadStateByRoot - with replay - using db",
			persistState: persistDB,
			loader:       lsbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "loadStateByRoot - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       lsbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "loadStateByRoot - with replay - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       lsbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		// StateByRootInitSync tests
		{
			name:         "StateByRootInitSync - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       sbrInit(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRootInitSync - with replay - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       sbrInit(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRootInitSync - using hot state cache",
			persistState: persistHotStateCache,
			loader:       sbrInit(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRootInitSync - with replay - using hot state cache",
			persistState: persistHotStateCache,
			loader:       sbrInit(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRootInitSync - using db",
			persistState: persistDB,
			loader:       sbrInit(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRootInitSync - with replay - using db",
			persistState: persistDB,
			loader:       sbrInit(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRootInitSync - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       sbrInit(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRootInitSync - with replay - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       sbrInit(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		// StateByRoot tests
		{
			name:         "StateByRoot - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       sbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRoot - with replay - using epoch boundary cache",
			persistState: persistEpochBoundary,
			loader:       sbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRoot - using hot state cache",
			persistState: persistHotStateCache,
			loader:       sbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRoot - with replay - using hot state cache",
			persistState: persistHotStateCache,
			loader:       sbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRoot - using db",
			persistState: persistDB,
			loader:       sbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRoot - with replay - using db",
			persistState: persistDB,
			loader:       sbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
		{
			name:         "StateByRoot - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       sbr(10),
			slots:        testSetupSlots{stateAt: 9, lastblock: 10},
		},
		{
			name:         "StateByRoot - with replay - using finalizedInfo struct field",
			persistState: persistFinalizedStruct,
			loader:       sbr(11),
			slots:        testSetupSlots{stateAt: 9, lastblock: 11},
		},
	}

	// Do all the state setup just once

	// generate state and wind up to slot 9
	st9, _ := util.DeterministicGenesisState(t, 32)
	st9, err := ReplayProcessSlots(ctx, st9, 9)
	require.NoError(t, err)
	// take latest block header at slot 9 as parent root for block at slot 10
	hdr := st9.LatestBlockHeader()
	hdrRoot, err := hdr.HashTreeRoot()
	require.NoError(t, err)

	st10 := st9.Copy()
	// set up block at slot 10, pointed to the latest block header as parent root
	// at slot 10
	// using correctly computed proposer index
	blk10 := util.NewBeaconBlock()
	blk10.Block.Slot = 10
	blk10.Block.ParentRoot = hdrRoot[:]
	idx10, err := helpers.BeaconProposerIndexAtSlot(ctx, st10, blk10.Block.Slot)
	require.NoError(t, err)
	blk10.Block.ProposerIndex = idx10
	// modernized block types for slot 10
	ib10, err := blt.NewSignedBeaconBlock(blk10)
	require.NoError(t, err)

	// make state at slot 10 by transitioning a copy of st9 with ib10 (aka blk10)
	st10, err = executeStateTransitionStateGen(t.Context(), st10, ib10)
	require.NoError(t, err)
	st10Root, err := st10.HashTreeRoot(t.Context())
	require.NoError(t, err)
	// update state root for block 10 now that its been through stf
	blk10.Block.StateRoot = st10Root[:]
	ib10, err = blt.NewSignedBeaconBlock(blk10)
	require.NoError(t, err)
	rob10, err := blt.NewROBlock(ib10)
	require.NoError(t, err)

	// same series of steps for block at slot 11 - pointing to block 10 as parent
	blk11 := util.NewBeaconBlock()
	blk11.Block.Slot = 11
	blk11.Block.ParentRoot = rob10.RootSlice()
	idx11, err := helpers.BeaconProposerIndexAtSlot(t.Context(), st10, blk11.Block.Slot)
	require.NoError(t, err)
	blk11.Block.ProposerIndex = idx11
	ib11, err := blt.NewSignedBeaconBlock(blk11)
	require.NoError(t, err)

	// same steps as 9->10; stf 10->11, then block update
	st11 := st10.Copy()
	st11, err = executeStateTransitionStateGen(t.Context(), st11, ib11)
	require.NoError(t, err)
	st11Root, err := st11.HashTreeRoot(t.Context())
	require.NoError(t, err)
	// update state root for block 11 now that its been through stf
	blk11.Block.StateRoot = st11Root[:]
	ib11, err = blt.NewSignedBeaconBlock(blk11)
	require.NoError(t, err)
	rob11, err := blt.NewROBlock(ib11)
	require.NoError(t, err)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			helpers.ClearCache()
			beaconDB := testDB.SetupDB(t)
			service := New(beaconDB, doublylinkedtree.New())
			r := testChain{
				t:   t,
				ctx: ctx,
				d:   beaconDB,
				srv: service,
				cslots: map[primitives.Slot]testChainSlot{
					9: testChainSlot{
						// note blk is nil for slot 9
						st:   st9.Copy(),
						root: hdrRoot,
					},
					10: testChainSlot{
						st:   st10.Copy(),
						blk:  rob10,
						root: rob10.Root(),
					},
					11: testChainSlot{
						st:   st11.Copy(),
						blk:  rob11,
						root: rob11.Root(),
					},
				},
			}
			slots := c.slots
			sumRoot := r.blockRoot(t, slots.stateAt)
			require.NoError(t, r.srv.beaconDB.SaveStateSummary(r.ctx, &ethpb.StateSummary{Slot: slots.stateAt, Root: sumRoot[:]}))
			// Second param controls the highest slot that we save blocks for, save all blocks <= that slot
			for _, ut := range []primitives.Slot{10, 11} {
				if ut <= slots.lastblock {
					require.NoError(t, r.d.SaveBlock(r.ctx, r.block(t, ut)))
				}
			}
			c.persistState(r, slots.stateAt)
			// DeepSSZEqual spams full state diffs on failures, so try to fail faster with more specific assertions.
			expect := r.state(t, slots.lastblock)
			got, err := c.loader(r)
			require.NoError(t, err)
			require.Equal(t, slots.lastblock, got.Slot())
			lbrE, err := expect.LatestBlockHeader().HashTreeRoot()
			require.NoError(t, err)
			lbrG, err := got.LatestBlockHeader().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, lbrE, lbrG)
			require.DeepSSZEqual(t, expect.ToProtoUnsafe(), got.ToProtoUnsafe())
		})
	}
}

func TestLastAncestorState_CanGetUsingDB(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	b0 := util.NewBeaconBlock()
	b0.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r0, err := b0.Block.HashTreeRoot()
	require.NoError(t, err)
	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo(r0[:], 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b2 := util.NewBeaconBlock()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = bytesutil.PadTo(r1[:], 32)
	r2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	b3 := util.NewBeaconBlock()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = bytesutil.PadTo(r2[:], 32)
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)

	b1State, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, b1State.SetSlot(1))

	util.SaveBlock(t, ctx, service.beaconDB, b0)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	util.SaveBlock(t, ctx, service.beaconDB, b2)
	util.SaveBlock(t, ctx, service.beaconDB, b3)
	require.NoError(t, service.beaconDB.SaveState(ctx, b1State, r1))

	lastState, err := service.latestAncestor(ctx, r3)
	require.NoError(t, err)
	assert.Equal(t, b1State.Slot(), lastState.Slot(), "Did not get wanted state")
}

func TestLastAncestorState_CanGetUsingCache(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	b0 := util.NewBeaconBlock()
	b0.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r0, err := b0.Block.HashTreeRoot()
	require.NoError(t, err)
	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo(r0[:], 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b2 := util.NewBeaconBlock()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = bytesutil.PadTo(r1[:], 32)
	r2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	b3 := util.NewBeaconBlock()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = bytesutil.PadTo(r2[:], 32)
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)

	b1State, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, b1State.SetSlot(1))

	util.SaveBlock(t, ctx, service.beaconDB, b0)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	util.SaveBlock(t, ctx, service.beaconDB, b2)
	util.SaveBlock(t, ctx, service.beaconDB, b3)
	service.hotStateCache.put(r1, b1State)

	lastState, err := service.latestAncestor(ctx, r3)
	require.NoError(t, err)
	assert.Equal(t, b1State.Slot(), lastState.Slot(), "Did not get wanted state")
}

func TestState_HasState(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	rHit1 := [32]byte{1}
	rHit2 := [32]byte{2}
	rMiss := [32]byte{3}
	service.hotStateCache.put(rHit1, s)
	require.NoError(t, service.epochBoundaryStateCache.put(rHit2, s))

	b := util.NewBeaconBlock()
	rHit3, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveState(ctx, s, rHit3))
	tt := []struct {
		root [32]byte
		want bool
	}{
		{rHit1, true},
		{rHit2, true},
		{rMiss, false},
		{rHit3, true},
	}
	for _, tc := range tt {
		got, err := service.HasState(ctx, tc.root)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestState_HasStateInCache(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	rHit1 := [32]byte{1}
	rHit2 := [32]byte{2}
	rMiss := [32]byte{3}
	service.hotStateCache.put(rHit1, s)
	require.NoError(t, service.epochBoundaryStateCache.put(rHit2, s))

	tt := []struct {
		root [32]byte
		want bool
	}{
		{rHit1, true},
		{rHit2, true},
		{rMiss, false},
	}
	for _, tc := range tt {
		got, err := service.hasStateInCache(ctx, tc.root)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}
