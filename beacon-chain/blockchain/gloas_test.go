package blockchain

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func prepareGloasForkchoiceState(
	_ context.Context,
	slot primitives.Slot,
	blockRoot [32]byte,
	parentRoot [32]byte,
	blockHash [32]byte,
	parentBlockHash [32]byte,
	justifiedEpoch primitives.Epoch,
	finalizedEpoch primitives.Epoch,
) (state.BeaconState, blocks.ROBlock, error) {
	blockHeader := &ethpb.BeaconBlockHeader{
		ParentRoot: parentRoot[:],
	}

	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: justifiedEpoch,
	}

	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: finalizedEpoch,
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 64)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}

	base := &ethpb.BeaconStateGloas{
		Slot:                       slot,
		RandaoMixes:                make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		CurrentJustifiedCheckpoint: justifiedCheckpoint,
		FinalizedCheckpoint:        finalizedCheckpoint,
		LatestBlockHeader:          blockHeader,
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			BlockHash:          blockHash[:],
			ParentBlockHash:    parentBlockHash[:],
			ParentBlockRoot:    make([]byte, 32),
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Builders:                     make([]*ethpb.Builder, 0),
		BuilderPendingPayments:       builderPendingPayments,
		ExecutionPayloadAvailability: make([]byte, 1024),
		LatestBlockHash:              make([]byte, 32),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		ProposerLookahead:            make([]uint64, 64),
	}

	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}

	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})

	signed, err := blocks.NewSignedBeaconBlock(blk)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	return st, roblock, err
}

func TestGetLookupParentRoot_PreGloas(t *testing.T) {
	service, _ := minimalTestService(t)

	parentRoot := [32]byte{1}
	blk := util.HydrateSignedBeaconBlockDeneb(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			ParentRoot: parentRoot[:],
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	require.Equal(t, parentRoot, got)
}

func TestGetLookupParentRoot_GloasBuildsOnEmpty(t *testing.T) {
	service, req := minimalTestService(t)
	ctx := t.Context()

	parentRoot := [32]byte{1}
	parentBlockHash := [32]byte{20}
	parentNodeBlockHash := [32]byte{99} // Different from parentBlockHash => builds on empty

	// Insert a Gloas node for the parent so BlockHash works.
	st, parentROBlock, err := prepareGloasForkchoiceState(ctx, 1, parentRoot, params.BeaconConfig().ZeroHash, parentNodeBlockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, req.fcs.InsertNode(ctx, st, parentROBlock))

	blockHash := [32]byte{10}
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       2,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	// parentBlockHash != parentNodeBlockHash, so it builds on empty => returns parentRoot
	require.Equal(t, parentRoot, got)
}

func TestGetLookupParentRoot_GloasBuildsOnFull(t *testing.T) {
	service, req := minimalTestService(t)
	ctx := t.Context()

	parentRoot := [32]byte{1}
	parentNodeBlockHash := [32]byte{10}

	// Insert a Gloas node for the parent so BlockHash works.
	st, parentROBlock, err := prepareGloasForkchoiceState(ctx, 1, parentRoot, params.BeaconConfig().ZeroHash, parentNodeBlockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, req.fcs.InsertNode(ctx, st, parentROBlock))

	// Set parentBlockHash in the bid to match the parent's blockHash.
	blockHash := [32]byte{20}
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentNodeBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       2,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	// parentBlockHash == parentNodeBlockHash, so it builds on full => returns parentBlockHash
	require.Equal(t, parentNodeBlockHash, got)
}
