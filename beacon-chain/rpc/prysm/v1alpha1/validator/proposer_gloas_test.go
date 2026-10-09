//go:build minimal

package validator

import (
	"context"
	"math"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSetRemoteBidFallback(t *testing.T) {
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

	newBlock := func(t *testing.T) interfaces.SignedBeaconBlock {
		sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
			Block: &ethpb.BeaconBlockGloas{
				Slot:       slot,
				ParentRoot: parentRoot[:],
				Body:       &ethpb.BeaconBlockBodyGloas{},
			},
		})
		require.NoError(t, err)
		return sBlk
	}
	newRemoteBid := func(builderIndex primitives.BuilderIndex, value primitives.Gwei) *ethpb.SignedExecutionPayloadBid {
		return &ethpb.SignedExecutionPayloadBid{
			Message: &ethpb.ExecutionPayloadBid{
				Slot:                  slot,
				ParentBlockHash:       parentBlockHash[:],
				ParentBlockRoot:       parentRoot[:],
				BlockHash:             make([]byte, 32),
				BuilderIndex:          builderIndex,
				Value:                 value,
				FeeRecipient:          make([]byte, 20),
				GasLimit:              30_000_000,
				PrevRandao:            make([]byte, 32),
				BlobKzgCommitments:    [][]byte{},
				ExecutionRequestsRoot: make([]byte, 32),
			},
			Signature: make([]byte, 96),
		}
	}
	newServer := func(p2pBid *ethpb.SignedExecutionPayloadBid) *Server {
		bidCache := cache.NewHighestExecutionPayloadBidCache()
		if p2pBid != nil {
			bidCache.SetIfHigher(p2pBid)
		}
		return &Server{HighestBidCache: bidCache, ForkchoiceFetcher: &chainMock.ChainService{}}
	}
	builderWin := func(value primitives.Gwei) *winningBuilderBid {
		return &winningBuilderBid{
			bid:   newRemoteBid(9, value),
			entry: &ethpb.BuilderEntry{Url: []byte("http://builder"), MaxExecutionPayment: math.MaxUint64, BuilderBoostFactor: 100},
		}
	}

	tests := []struct {
		name       string
		p2p        *ethpb.SignedExecutionPayloadBid
		builder    *winningBuilderBid
		wantSrc    bidSource
		wantIdx    primitives.BuilderIndex
		wantErrStr string
	}{
		{name: "cached p2p bid only", p2p: newRemoteBid(7, 1000), wantSrc: bidSourceP2P, wantIdx: 7},
		{name: "builder bid only", builder: builderWin(1000), wantSrc: bidSourceBuilderAPI, wantIdx: 9},
		{name: "builder bid beats p2p", p2p: newRemoteBid(7, 1000), builder: builderWin(2000), wantSrc: bidSourceBuilderAPI, wantIdx: 9},
		{name: "p2p bid beats builder", p2p: newRemoteBid(7, 3000), builder: builderWin(2000), wantSrc: bidSourceP2P, wantIdx: 7},
		{name: "no remote bids", wantErrStr: "no builder or cached P2P bid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sBlk := newBlock(t)
			src, err := newServer(tt.p2p).setRemoteBidFallback(context.Background(), sBlk, st, false, tt.builder, nil)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, tt.wantErrStr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSrc, src)
			signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
			require.NoError(t, err)
			require.NotNil(t, signedBid)
			require.Equal(t, tt.wantIdx, signedBid.Message.BuilderIndex)
		})
	}

	t.Run("p2p bid is looked up on the state-derived parent hash", func(t *testing.T) {
		sBlk := newBlock(t)
		_, err := newServer(newRemoteBid(7, 1000)).setRemoteBidFallback(context.Background(), sBlk, st, true, nil, nil)
		require.ErrorContains(t, "no builder or cached P2P bid", err)
	})

	t.Run("nil bid cache still uses the builder bid", func(t *testing.T) {
		sBlk := newBlock(t)
		vs := &Server{ForkchoiceFetcher: &chainMock.ChainService{}}
		src, err := vs.setRemoteBidFallback(context.Background(), sBlk, st, false, builderWin(1000), nil)
		require.NoError(t, err)
		require.Equal(t, bidSourceBuilderAPI, src)
	})
}

func TestSetRemoteBidFallback_GloasForkBoundary(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	fuluBlockHash := bytesutil.ToBytes32([]byte("fulu-block-hash"))
	parentRoot := bytesutil.ToBytes32([]byte("parent-root"))
	slot := params.BeaconConfig().SlotsPerEpoch
	st := upgradedGloasState(t, fuluBlockHash)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       fuluBlockHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          7,
			Value:                 1000,
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			PrevRandao:            make([]byte, 32),
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})

	vs := &Server{HighestBidCache: bidCache, ForkchoiceFetcher: &chainMock.ChainService{BlockSlot: slot - 1}}
	src, err := vs.setRemoteBidFallback(t.Context(), sBlk, st, true, nil, nil)
	require.NoError(t, err)
	require.Equal(t, bidSourceP2P, src)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.Equal(t, primitives.BuilderIndex(7), signedBid.Message.BuilderIndex)
}

func TestGloasPayloadValue(t *testing.T) {
	vs := &Server{}
	newBlockWithBid := func(value, payment primitives.Gwei) interfaces.SignedBeaconBlock {
		blk := util.NewBeaconBlockGloas()
		blk.Block.Body.SignedExecutionPayloadBid.Message.Value = value
		blk.Block.Body.SignedExecutionPayloadBid.Message.ExecutionPayment = payment
		sBlk, err := consensusblocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		return sBlk
	}

	t.Run("self-built uses local bid", func(t *testing.T) {
		local := &consensusblocks.GetPayloadResponse{Bid: primitives.Uint64ToWei(123456789)}
		got := vs.gloasPayloadValue(newBlockWithBid(0, 0), local, true)
		require.Equal(t, "123456789", primitives.WeiToBigInt(got).String())
	})
	t.Run("self-built without local bid is zero", func(t *testing.T) {
		got := vs.gloasPayloadValue(newBlockWithBid(0, 0), nil, true)
		require.Equal(t, "0", primitives.WeiToBigInt(got).String())
	})
	t.Run("external bid uses bid value in wei", func(t *testing.T) {
		got := vs.gloasPayloadValue(newBlockWithBid(3, 2), &consensusblocks.GetPayloadResponse{}, false)
		require.Equal(t, "3000000000", primitives.WeiToBigInt(got).String())
	})
	t.Run("external bid value does not overflow", func(t *testing.T) {
		got := vs.gloasPayloadValue(newBlockWithBid(primitives.Gwei(math.MaxUint64), 0), nil, false)
		require.Equal(t, "18446744073709551615000000000", primitives.WeiToBigInt(got).String())
	})
}
