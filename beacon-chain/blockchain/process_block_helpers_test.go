package blockchain

import (
	"testing"
	"time"

	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// When the current head is unchanged, saveHeadIfNeeded must return immediately
// without touching the head or panicking.
func TestSaveHeadIfNeeded_NotNewHead_NoOp(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	headRoot := bytesutil.ToBytes32([]byte("headroot"))
	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	base, blk := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlockWithRoot(signed, headRoot)
	require.NoError(t, err)

	// Pre-set head to the same root with full=false so isNewHead returns false.
	s.head = &head{root: headRoot, block: signed, state: st, slot: 1, full: false}

	cfg := &postBlockProcessConfig{ctx: ctx, roblock: roblock, headRoot: headRoot, postState: st}
	s.saveHeadIfNeeded(ctx, cfg)

	// Head must be untouched.
	require.Equal(t, headRoot, s.head.root)
	require.Equal(t, primitives.Slot(1), s.head.slot)
}

// Regression test for the honest reorg fix: when we are the next-slot proposer
// (non-empty payload attribute) and forkchoice says the late head should be
// orphaned (shouldOverrideFCU == true), saveHeadIfNeeded must NOT save the head.
// Against the pre-fix code (which always saved the head for Gloas) this would
// have set s.head to the late block.
func TestSaveHeadIfNeeded_ProposingAndOverride_SkipsSave(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	// Service clock: current slot == 2, so the proposing slot is 3.
	service.SetGenesisTime(time.Now().Add(-time.Duration(2*params.BeaconConfig().SecondsPerSlot) * time.Second))

	parentRoot := [32]byte{'a'}
	headRoot := [32]byte{'b'}
	ojc := &ethpb.Checkpoint{}
	st, ro, err := prepareForkchoiceState(ctx, 1, parentRoot, [32]byte{}, [32]byte{}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, ro))
	st, ro, err = prepareForkchoiceState(ctx, 2, headRoot, parentRoot, [32]byte{}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, ro))

	// Forkchoice clock: the head node (slot 2) is from the current slot and arrived
	// late, so ForkChoiceStore.ShouldOverrideFCU() returns true.
	fcs.SetGenesisTime(time.Now().Add(-29 * time.Second))
	head, err := fcs.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, head)

	// postState at slot 3 (== proposing slot) so getPayloadAttribute does not need to
	// process slots and yields a non-empty attribute (we are proposing).
	postState, _, err := prepareForkchoiceState(ctx, 3, [32]byte{'c'}, headRoot, [32]byte{}, ojc, ojc)
	require.NoError(t, err)

	require.Equal(t, primitives.Slot(2), service.CurrentSlot())
	proposingSlot := service.CurrentSlot() + 1
	attr := service.getPayloadAttribute(ctx, postState, proposingSlot, headRoot[:], false)
	require.Equal(t, false, attr.IsEmpty())
	require.Equal(t, true, service.shouldOverrideFCU(headRoot, proposingSlot))

	// s.head is nil → isNewHead is true; the override must trigger an early return
	// before saveHead is reached, leaving the head untouched (nil).
	cfg := &postBlockProcessConfig{ctx: ctx, headRoot: headRoot, postState: postState}
	service.saveHeadIfNeeded(ctx, cfg)
	require.Equal(t, true, service.head == nil)
}

type notSyncedChecker struct{}

func (notSyncedChecker) Synced() bool { return false }

func TestSaveHeadIfNeeded_NotSynced_SkipsPayloadAttribute(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs
	service.cfg.SyncChecker = notSyncedChecker{}

	// Service clock: current slot == 2, so the proposing slot is 3.
	service.SetGenesisTime(time.Now().Add(-time.Duration(2*params.BeaconConfig().SecondsPerSlot) * time.Second))

	parentRoot := [32]byte{'a'}
	headRoot := [32]byte{'b'}
	ojc := &ethpb.Checkpoint{}
	st, ro, err := prepareForkchoiceState(ctx, 1, parentRoot, [32]byte{}, [32]byte{}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, ro))
	st, ro, err = prepareForkchoiceState(ctx, 2, headRoot, parentRoot, [32]byte{}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, ro))

	// Forkchoice clock: the head node (slot 2) is from the current slot and arrived
	// late, so ForkChoiceStore.ShouldOverrideFCU() returns true.
	fcs.SetGenesisTime(time.Now().Add(-29 * time.Second))
	fcHead, err := fcs.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, fcHead)

	postState, _, err := prepareForkchoiceState(ctx, 3, [32]byte{'c'}, headRoot, [32]byte{}, ojc, ojc)
	require.NoError(t, err)

	// Not in regular sync: the attribute is empty even though the proposer is
	// tracked and the override conditions hold, so the save below must proceed.
	proposingSlot := service.CurrentSlot() + 1
	attr := service.getPayloadAttribute(ctx, postState, proposingSlot, headRoot[:], false)
	require.Equal(t, true, attr.IsEmpty())
	require.Equal(t, true, service.shouldOverrideFCU(headRoot, proposingSlot))

	// Old head and new head block plumbing so saveHead can complete.
	oldBlock := util.SaveBlock(t, ctx, service.cfg.BeaconDB, util.NewBeaconBlock())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	service.head = &head{root: oldRoot, block: oldBlock, state: postState}

	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 2
	newHeadSignedBlock.Block.ParentRoot = oldRoot[:]
	wsb := util.SaveBlock(t, ctx, service.cfg.BeaconDB, newHeadSignedBlock)
	roblock, err := blocks.NewROBlockWithRoot(wsb, headRoot)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 2, Root: headRoot[:]}))

	cfg := &postBlockProcessConfig{ctx: ctx, roblock: roblock, headRoot: headRoot, postState: postState}
	service.saveHeadIfNeeded(ctx, cfg)
	require.Equal(t, headRoot, service.head.root)
}
