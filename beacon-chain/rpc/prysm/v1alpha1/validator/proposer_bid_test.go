package validator

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSetSelfBuildExecutionPayloadBid(t *testing.T) {
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)
	proposerIndex := primitives.ValidatorIndex(42)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:          slot,
			ProposerIndex: proposerIndex,
			ParentRoot:    parentRoot[:],
			Body:          &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	// 5 Gwei = 5,000,000,000 Wei
	bidValue := big.NewInt(5_000_000_000)
	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               bidValue,
		BlobsBundler:      nil,
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	vs := &Server{}

	isSelfBuild, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, false)
	require.NoError(t, err)
	require.Equal(t, true, isSelfBuild)

	// Verify the signed bid was set on the block.
	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)
	require.NotNil(t, signedBid.Message)

	// Per spec (process_execution_payload_bid): for self-builds,
	// signature must be G2 point-at-infinity.
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)

	// Verify bid fields.
	bid := signedBid.Message
	require.Equal(t, slot, bid.Slot)
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, bid.BuilderIndex)
	require.DeepEqual(t, parentRoot[:], bid.ParentBlockRoot)
	require.Equal(t, primitives.Gwei(0), bid.Value)
	require.Equal(t, primitives.Gwei(0), bid.ExecutionPayment)
}

func TestSetSelfBuildExecutionPayloadBid_NilPayload(t *testing.T) {
	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       1,
			ParentRoot: make([]byte, 32),
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	vs := &Server{}

	_, err = vs.setExecutionPayloadBid(t.Context(), sBlk, nil, false)
	require.ErrorContains(t, "local execution payload is nil", err)

	_, err = vs.setExecutionPayloadBid(t.Context(), sBlk, &consensusblocks.GetPayloadResponse{}, false)
	require.ErrorContains(t, "local execution payload is nil", err)
}

func TestSetExecutionPayloadBid_PrefersP2PBid(t *testing.T) {
	parentHash := [32]byte{10, 20, 30}
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    parentHash[:],
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               big.NewInt(0),
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	// Populate the highest bid cache with a P2P bid.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:               slot,
			ParentBlockHash:    parentHash[:],
			ParentBlockRoot:    parentRoot[:],
			BlockHash:          make([]byte, 32),
			BuilderIndex:       5,
			Value:              1000,
			ExecutionPayment:   500,
			FeeRecipient:       make([]byte, 20),
			GasLimit:           30_000_000,
			PrevRandao:         make([]byte, 32),
			BlobKzgCommitments: [][]byte{},
		},
		Signature: make([]byte, 96),
	}

	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(p2pBid)

	vs := &Server{HighestBidCache: bidCache}

	isSelfBuild, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, false)
	require.NoError(t, err)
	require.Equal(t, false, isSelfBuild)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)

	// Should use the P2P bid, not the self-build bid.
	require.Equal(t, primitives.BuilderIndex(5), signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(1000), signedBid.Message.Value)
	require.Equal(t, primitives.Gwei(500), signedBid.Message.ExecutionPayment)
}

func TestSetExecutionPayloadBid_PrefersLocalWhenHigherValue(t *testing.T) {
	parentHash := [32]byte{10, 20, 30}
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    parentHash[:],
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	// Local bid is 2000 Gwei (in Wei: 2000 * 1e9).
	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               big.NewInt(2000_000_000_000),
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	// P2P bid is only 1000 Gwei — local should win.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:               slot,
			ParentBlockHash:    parentHash[:],
			ParentBlockRoot:    parentRoot[:],
			BlockHash:          make([]byte, 32),
			BuilderIndex:       5,
			Value:              1000,
			ExecutionPayment:   500,
			FeeRecipient:       make([]byte, 20),
			GasLimit:           30_000_000,
			PrevRandao:         make([]byte, 32),
			BlobKzgCommitments: [][]byte{},
		},
		Signature: make([]byte, 96),
	}

	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(p2pBid)

	vs := &Server{HighestBidCache: bidCache}

	isSelfBuild, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, false)
	require.NoError(t, err)
	require.Equal(t, true, isSelfBuild)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)

	// Local value is higher, so self-build should be used.
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(0), signedBid.Message.Value)
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)
}

func TestSetExecutionPayloadBid_SelfBuildOnlyIgnoresCache(t *testing.T) {
	parentHash := [32]byte{10, 20, 30}
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    parentHash[:],
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               big.NewInt(0),
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	// P2P bid has higher value, but selfBuildOnly=true should force self-build.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:               slot,
			ParentBlockHash:    parentHash[:],
			ParentBlockRoot:    parentRoot[:],
			BlockHash:          make([]byte, 32),
			BuilderIndex:       5,
			Value:              1000,
			ExecutionPayment:   500,
			FeeRecipient:       make([]byte, 20),
			GasLimit:           30_000_000,
			PrevRandao:         make([]byte, 32),
			BlobKzgCommitments: [][]byte{},
		},
		Signature: make([]byte, 96),
	}

	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(p2pBid)

	vs := &Server{HighestBidCache: bidCache}

	isSelfBuild, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, true)
	require.NoError(t, err)
	require.Equal(t, true, isSelfBuild)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)

	// selfBuildOnly forces self-build even when P2P bid is higher.
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(0), signedBid.Message.Value)
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)
}

func TestSetExecutionPayloadBid_FallsBackToSelfBuildWhenNoCachedBid(t *testing.T) {
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               big.NewInt(0),
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	// Empty cache — no P2P bids.
	bidCache := cache.NewHighestExecutionPayloadBidCache()
	vs := &Server{HighestBidCache: bidCache}

	isSelfBuild, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, false)
	require.NoError(t, err)
	require.Equal(t, true, isSelfBuild)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)

	// Should fall back to self-build.
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(0), signedBid.Message.Value)
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)
}
