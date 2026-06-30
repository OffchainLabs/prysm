package validator

import (
	"context"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSetP2PBidFallback_UsesCachedBid(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	parentBlockHash := bytesutil.ToBytes32([]byte("parent-block-hash"))
	parentRoot := bytesutil.ToBytes32([]byte("parent-root"))
	slot := primitives.Slot(100)

	blockHash := bytesutil.ToBytes32([]byte("block-hash"))
	st, err := util.NewBeaconStateGloas(func(state *ethpb.BeaconStateGloas) error {
		state.LatestExecutionPayloadBid.BlockHash = blockHash[:]
		state.LatestExecutionPayloadBid.ParentBlockHash = parentBlockHash[:]
		return nil
	})
	require.NoError(t, err)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentBlockHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          7,
			Value:                 1000,
			ExecutionPayment:      500,
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			PrevRandao:            make([]byte, 32),
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(p2pBid)

	vs := &Server{HighestBidCache: bidCache, ForkchoiceFetcher: &chainMock.ChainService{}}

	require.NoError(t, vs.setP2PBidFallback(context.Background(), sBlk, st, false))

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)
	require.Equal(t, primitives.BuilderIndex(7), signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(1000), signedBid.Message.Value)
}

func TestSetP2PBidFallback_NoCachedBidErrors(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	parentRoot := bytesutil.ToBytes32([]byte("parent-root"))
	parentBlockHash := bytesutil.ToBytes32([]byte("parent-block-hash"))
	st, err := util.NewBeaconStateGloas(func(state *ethpb.BeaconStateGloas) error {
		state.LatestExecutionPayloadBid.ParentBlockHash = parentBlockHash[:]
		return nil
	})
	require.NoError(t, err)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       primitives.Slot(100),
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	vs := &Server{HighestBidCache: cache.NewHighestExecutionPayloadBidCache(), ForkchoiceFetcher: &chainMock.ChainService{}}

	err = vs.setP2PBidFallback(context.Background(), sBlk, st, false)
	require.ErrorContains(t, "no cached P2P bid", err)
}
