//go:build minimal

package validator

import (
	"context"
	"math"
	"math/big"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
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
	newServer := func(t *testing.T, p2pBid *ethpb.SignedExecutionPayloadBid) *Server {
		bidCache := cache.NewHighestExecutionPayloadBidCache()
		if p2pBid != nil {
			bidCache.SetIfHigher(wrapBid(t, p2pBid))
		}
		return &Server{HighestBidCache: bidCache, ForkchoiceFetcher: &chainMock.ChainService{}}
	}
	builderWin := func(t *testing.T, value primitives.Gwei) *winningBuilderBid {
		return &winningBuilderBid{
			bid:   wrapBid(t, newRemoteBid(9, value)),
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
		{name: "builder bid only", builder: builderWin(t, 1000), wantSrc: bidSourceBuilderAPI, wantIdx: 9},
		{name: "builder bid beats p2p", p2p: newRemoteBid(7, 1000), builder: builderWin(t, 2000), wantSrc: bidSourceBuilderAPI, wantIdx: 9},
		{name: "p2p bid beats builder", p2p: newRemoteBid(7, 3000), builder: builderWin(t, 2000), wantSrc: bidSourceP2P, wantIdx: 7},
		{name: "no remote bids", wantErrStr: "no builder or cached P2P bid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sBlk := newBlock(t)
			src, err := newServer(t, tt.p2p).setRemoteBidFallback(context.Background(), sBlk, st, false, tt.builder, nil)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, tt.wantErrStr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSrc, src)
			signedBid := signedBidProto(t, sBlk)
			require.NotNil(t, signedBid)
			require.Equal(t, tt.wantIdx, signedBid.Message.BuilderIndex)
		})
	}

	t.Run("p2p bid is looked up on the state-derived parent hash", func(t *testing.T) {
		sBlk := newBlock(t)
		_, err := newServer(t, newRemoteBid(7, 1000)).setRemoteBidFallback(context.Background(), sBlk, st, true, nil, nil)
		require.ErrorContains(t, "no builder or cached P2P bid", err)
	})

	t.Run("nil bid cache still uses the builder bid", func(t *testing.T) {
		sBlk := newBlock(t)
		vs := &Server{ForkchoiceFetcher: &chainMock.ChainService{}}
		src, err := vs.setRemoteBidFallback(context.Background(), sBlk, st, false, builderWin(t, 1000), nil)
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
	bidCache.SetIfHigher(wrapBid(t, &ethpb.SignedExecutionPayloadBid{
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
	}))

	vs := &Server{HighestBidCache: bidCache, ForkchoiceFetcher: &chainMock.ChainService{BlockSlot: slot - 1}}
	src, err := vs.setRemoteBidFallback(t.Context(), sBlk, st, true, nil, nil)
	require.NoError(t, err)
	require.Equal(t, bidSourceP2P, src)

	signedBid := signedBidProto(t, sBlk)
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

func TestBuildBlockGloas_EagerSelfBuild(t *testing.T) {
	for _, tt := range []struct {
		name      string
		hezeEpoch primitives.Epoch
		version   int
	}{
		{name: "Gloas", hezeEpoch: params.BeaconConfig().FarFutureEpoch, version: version.Gloas},
		{name: "Heze", hezeEpoch: 1, version: version.Heze},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch, cfg.DenebForkEpoch = 0, 0, 0, 0
			cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch, cfg.HezeForkEpoch = 0, 0, 0, tt.hezeEpoch
			params.OverrideBeaconConfig(cfg)
			helpers.ClearCache()
			t.Cleanup(helpers.ClearCache)
			// Other tests cache slot transitions of the same genesis state under other fork schedules.
			transition.SkipSlotCache.Disable()
			t.Cleanup(transition.SkipSlotCache.Enable)

			// The genesis state is Gloas; with Heze at epoch 1, the slot transition into epoch 1 upgrades it.
			genesis, _ := util.DeterministicGenesisStateGloas(t, 64)
			stateRoot, err := genesis.HashTreeRoot(ctx)
			require.NoError(t, err)
			header := genesis.LatestBlockHeader()
			header.StateRoot = stateRoot[:]
			parentRoot, err := header.HashTreeRoot()
			require.NoError(t, err)

			db := dbutil.SetupDB(t)
			require.NoError(t, db.SaveGenesisBlockRoot(ctx, parentRoot))
			vs := getProposerServer(ctx, db, genesis, parentRoot[:])
			stateGen := mockstategen.NewService()
			stateGen.AddStateForRoot(genesis, parentRoot)
			vs.StateGen = stateGen
			vs.ExecutionPayloadEnvelopeCache = cache.NewExecutionPayloadEnvelopeCache()

			slot := params.BeaconConfig().SlotsPerEpoch + 1
			head, err := transition.ProcessSlots(ctx, genesis.Copy(), slot)
			require.NoError(t, err)
			require.Equal(t, tt.version, head.Version())
			parentHash, err := head.LatestBlockHash()
			require.NoError(t, err)
			prevRandao, err := helpers.RandaoMix(head, slots.ToEpoch(slot))
			require.NoError(t, err)
			ed, err := consensusblocks.WrappedExecutionPayloadGloas(&enginev1.ExecutionPayloadGloas{
				ParentHash:    parentHash[:],
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    prevRandao,
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     bytesutil.PadTo([]byte{'b'}, 32),
				ExtraData:     make([]byte, 0),
				SlotNumber:    slot,
			})
			require.NoError(t, err)
			vs.ExecutionEngineCaller = &mockExecution.EngineClient{GetPayloadResponse: &consensusblocks.GetPayloadResponse{
				ExecutionData:          ed,
				Bid:                    big.NewInt(0),
				BlobsBundler:           &enginev1.BlobsBundle{},
				ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
			}}
			vs.PayloadIDCache.Set(slot, parentRoot, false, primitives.PayloadID{1})

			sBlk, err := getEmptyBlock(slot)
			require.NoError(t, err)
			require.Equal(t, tt.version, sBlk.Version())
			sBlk.SetSlot(slot)
			sBlk.SetParentRoot(parentRoot[:])
			proposer, err := helpers.BeaconProposerIndexAtSlot(ctx, head, slot)
			require.NoError(t, err)
			sBlk.SetProposerIndex(proposer)

			result, err := vs.buildBlockGloas(ctx, sBlk, head, true, false, true, nil)
			require.NoError(t, err)
			if tt.version == version.Gloas {
				// The eager path bundles the envelope with the block.
				contents := result.GetGloasContents()
				require.NotNil(t, contents)
				require.NotNil(t, contents.ExecutionPayloadEnvelope)
				require.Equal(t, slot, contents.Block.Slot)
				return
			}
			// A Heze block has no block contents type, so the eager path returns the bare block.
			require.NotNil(t, result.GetHeze())
			require.Equal(t, slot, result.GetHeze().Slot)
		})
	}
}
