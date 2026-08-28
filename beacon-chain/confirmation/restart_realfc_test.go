package confirmation_test

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/confirmation"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type stubCommittees struct{}

func (stubCommittees) Committee(_ context.Context, _ primitives.Slot) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

func (stubCommittees) Seed(_ context.Context, epoch primitives.Epoch) ([32]byte, error) {
	return [32]byte{byte(epoch + 1)}, nil
}

type stubBalances struct{}

func (stubBalances) BalanceInfoByCheckpoint(_ context.Context, _ forkchoicetypes.Checkpoint) ([]uint64, uint64, error) {
	return []uint64{32e9, 32e9, 32e9, 32e9}, 128e9, nil
}

func (stubBalances) PulledUpHeadState(_ context.Context, _ [32]byte) (*confirmation.FFGStateInfo, error) {
	return &confirmation.FFGStateInfo{TotalActiveBalance: 128e9, Balances: []uint64{32e9, 32e9, 32e9, 32e9}}, nil
}

func fcState(t *testing.T, slot primitives.Slot, root, parent [32]byte, jc, fc *ethpb.Checkpoint) (state.BeaconState, blocks.ROBlock) {
	t.Helper()
	base := &ethpb.BeaconStateBellatrix{
		Slot:                         slot,
		RandaoMixes:                  make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		CurrentJustifiedCheckpoint:   jc,
		FinalizedCheckpoint:          fc,
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeader{BlockHash: root[:]},
		LatestBlockHeader:            &ethpb.BeaconBlockHeader{ParentRoot: parent[:]},
	}
	st, err := state_native.InitializeFromProtoBellatrix(base)
	require.NoError(t, err)
	blk := &ethpb.SignedBeaconBlockBellatrix{
		Block: &ethpb.BeaconBlockBellatrix{
			Slot:       slot,
			ParentRoot: parent[:],
			Body: &ethpb.BeaconBlockBodyBellatrix{
				ExecutionPayload: &enginev1.ExecutionPayload{BlockHash: root[:]},
			},
		},
	}
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlockWithRoot(signed, root)
	require.NoError(t, err)
	return st, roblock
}

func rootForSlot(s primitives.Slot) [32]byte {
	return [32]byte{byte(s), 0xAB}
}

func TestOnFastConfirmation_RealForkchoice_EpochBoundaryRestart(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig().Copy())
	ctx := context.Background()

	fc := doublylinkedtree.New()
	fc.SetBalancesByRooter(func(_ context.Context, _ [32]byte) ([]uint64, error) { return []uint64{}, nil })

	setWallSlot := func(s primitives.Slot) {
		fc.SetGenesisTime(time.Now().Add(-time.Duration(uint64(s)*params.BeaconConfig().SecondsPerSlot) * time.Second))
	}

	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	cpRoot := func(e primitives.Epoch) [32]byte { return rootForSlot(primitives.Slot(e) * spe) }
	finalizedRoot := cpRoot(2)
	target3 := cpRoot(3)

	setWallSlot(0)
	genesisRoot := rootForSlot(0)
	st, blk := fcState(t, 0, genesisRoot, [32]byte{},
		&ethpb.Checkpoint{Epoch: 0, Root: genesisRoot[:]},
		&ethpb.Checkpoint{Epoch: 0, Root: genesisRoot[:]})
	require.NoError(t, fc.InsertNode(ctx, st, blk))

	parent := genesisRoot
	for s := primitives.Slot(1); s <= 127; s++ {
		setWallSlot(s)
		e := primitives.Epoch(s / spe)
		je := e
		if s%spe < 22 && je > 0 {
			je--
		}
		fe := primitives.Epoch(0)
		if je > 0 {
			fe = je - 1
		}
		jr, fr := cpRoot(je), cpRoot(fe)
		jc := &ethpb.Checkpoint{Epoch: je, Root: jr[:]}
		fcp := &ethpb.Checkpoint{Epoch: fe, Root: fr[:]}
		r := rootForSlot(s)
		st, blk := fcState(t, s, r, parent, jc, fcp)
		require.NoError(t, fc.InsertNode(ctx, st, blk))
		parent = r
	}
	head, err := fc.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootForSlot(127), head)

	// Sanity: the forkchoice state FCR will read at the boundary.
	ujc := fc.UnrealizedJustifiedCheckpoint()
	t.Logf("store UJC: epoch=%d root=%#x (target3=%#x)", ujc.Epoch, ujc.Root, target3)
	headUJ, err := fc.UnrealizedJustification(head)
	require.NoError(t, err)
	t.Logf("head UJ: epoch=%d root=%#x", headUJ.Epoch, headUJ.Root)

	fcr := confirmation.New(fc, stubCommittees{}, stubBalances{}, forkchoicetypes.Checkpoint{Epoch: 2, Root: finalizedRoot})

	// Last slot of epoch 3: snapshots previousEpochGreatestUnrealizedCheckpoint.
	setWallSlot(127)
	fcr.OnFastConfirmation(ctx, 127)
	require.Equal(t, finalizedRoot, fcr.ConfirmedRoot())

	// First slot of epoch 4.
	setWallSlot(128)
	require.NoError(t, fc.NewSlot(ctx, 128))
	fcr.OnFastConfirmation(ctx, 128)

	t.Logf("previousEpochGreatestUnrealized: %+v", fcr.PreviousEpochGreatestUnrealizedCheckpoint())
	t.Logf("currentEpochObserved: %+v", fcr.CurrentEpochObservedJustifiedCheckpoint())
	require.Equal(t, target3, fcr.ConfirmedRoot(), "phase 2 restart did not move confirmed root to the epoch 3 checkpoint")
}
