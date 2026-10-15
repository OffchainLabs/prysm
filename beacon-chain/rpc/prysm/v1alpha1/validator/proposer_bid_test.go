//go:build minimal

package validator

import (
	"fmt"
	"math/big"
	"slices"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// blacklistedBreaker returns a circuit breaker that bans idx for the epoch of slot.
func blacklistedBreaker(t *testing.T, idx primitives.BuilderIndex, slot primitives.Slot) *cache.BuilderCircuitBreaker {
	c := cache.NewBuilderCircuitBreaker()
	epoch := slots.ToEpoch(slot)
	for i := uint64(0); i <= params.BeaconConfig().BuilderAllowedFailures; i++ {
		c.RecordFailure(idx, [32]byte{byte(idx), byte(i)}, epoch)
	}
	require.Equal(t, true, c.Blacklisted(idx, epoch))
	return c
}

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
		ExecutionData:          ed,
		Bid:                    bidValue,
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	vs := &Server{}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceSelfBuild, src)

	// Verify the signed bid was set on the block.
	signedBid := signedBidProto(t, sBlk)
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

func TestSetSelfBuildExecutionPayloadBid_BlobCommitments(t *testing.T) {
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

	// Create blob commitments matching what the EL would return.
	commitments := [][]byte{
		make([]byte, 48),
		make([]byte, 48),
		make([]byte, 48),
	}
	for i := range commitments {
		commitments[i][0] = byte(i + 1)
	}

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData: ed,
		BlobsBundler: &enginev1.BlobsBundle{
			KzgCommitments: commitments,
		},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	vs := &Server{}
	_, err = vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, true)
	require.NoError(t, err)

	signedBid := signedBidProto(t, sBlk)
	require.NotNil(t, signedBid.Message)

	// Verify blob KZG commitments are set on the bid (not empty).
	require.Equal(t, 3, len(signedBid.Message.BlobKzgCommitments))
	require.DeepEqual(t, commitments, signedBid.Message.BlobKzgCommitments)
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

	_, err = vs.setExecutionPayloadBid(t.Context(), sBlk, nil, nil, nil, nil, false)
	require.ErrorContains(t, "local execution payload is nil", err)

	_, err = vs.setExecutionPayloadBid(t.Context(), sBlk, nil, &consensusblocks.GetPayloadResponse{}, nil, nil, false)
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
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	// Populate the highest bid cache with a P2P bid.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          5,
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
	bidCache.SetIfHigher(wrapBid(t, p2pBid))

	vs := &Server{HighestBidCache: bidCache}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceP2P, src)

	signedBid := signedBidProto(t, sBlk)
	require.NotNil(t, signedBid)

	// Should use the P2P bid, not the self-build bid.
	require.Equal(t, primitives.BuilderIndex(5), signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(1000), signedBid.Message.Value)
	require.Equal(t, primitives.Gwei(500), signedBid.Message.ExecutionPayment)
}

// A cached P2P bid from a blacklisted builder must be ignored, even though it outbids the local
// payload: bids can be cached before the ban lands.
func TestSetExecutionPayloadBid_IgnoresBlacklistedP2PBid(t *testing.T) {
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
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	bidCache := cache.NewHighestExecutionPayloadBidCache()
	bidCache.SetIfHigher(wrapBid(t, &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          5,
			Value:                 1000,
			ExecutionPayment:      500,
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			PrevRandao:            make([]byte, 32),
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}))

	vs := &Server{
		HighestBidCache:       bidCache,
		BuilderCircuitBreaker: blacklistedBreaker(t, 5, slot),
	}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceSelfBuild, src)

	signedBid := signedBidProto(t, sBlk)
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, signedBid.Message.BuilderIndex)
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
		ExecutionData:          ed,
		Bid:                    big.NewInt(2000_000_000_000),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	// P2P bid is only 1000 Gwei — local should win.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          5,
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
	bidCache.SetIfHigher(wrapBid(t, p2pBid))

	vs := &Server{HighestBidCache: bidCache}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceSelfBuild, src)

	signedBid := signedBidProto(t, sBlk)
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
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	// P2P bid has higher value, but selfBuildOnly=true should force self-build.
	p2pBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentHash[:],
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          5,
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
	bidCache.SetIfHigher(wrapBid(t, p2pBid))

	vs := &Server{HighestBidCache: bidCache}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, true)
	require.NoError(t, err)
	require.Equal(t, bidSourceSelfBuild, src)

	signedBid := signedBidProto(t, sBlk)
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
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	// Empty cache — no P2P bids.
	bidCache := cache.NewHighestExecutionPayloadBidCache()
	vs := &Server{HighestBidCache: bidCache}

	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, nil, local, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceSelfBuild, src)

	signedBid := signedBidProto(t, sBlk)
	require.NotNil(t, signedBid)

	// Should fall back to self-build.
	require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(0), signedBid.Message.Value)
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)
}

// signedBidProto returns the signed execution payload bid protobuf of a Gloas block.
func signedBidProto(t *testing.T, blk interfaces.ReadOnlySignedBeaconBlock) *ethpb.SignedExecutionPayloadBid {
	pb, err := blk.Block().Body().Proto()
	require.NoError(t, err)
	body, ok := pb.(*ethpb.BeaconBlockBodyGloas)
	require.Equal(t, true, ok)
	return body.SignedExecutionPayloadBid
}

// hezeProposal is a proposal at a Heze slot whose inclusion list slot (slot - 1) has a known committee and dependent root.
type hezeProposal struct {
	slot          primitives.Slot
	parentRoot    [32]byte
	parentHash    [32]byte
	head          state.BeaconState
	committee     []primitives.ValidatorIndex
	dependentRoot [32]byte
	chain         *chainMock.ChainService
}

func newHezeProposal(t *testing.T) *hezeProposal {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch, cfg.DenebForkEpoch = 0, 0, 0, 0
	cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch, cfg.HezeForkEpoch = 0, 0, 0, 0
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()

	head, _ := util.DeterministicGenesisStateGloas(t, 64)
	// Epoch 2 keeps the dependent root out of the genesis fallback, which reads the DB.
	slot := 2*params.BeaconConfig().SlotsPerEpoch + 1
	require.NoError(t, head.SetSlot(slot))
	dependentRoot := [32]byte{'d'}
	dependentSlot, err := slots.ShufflingDependentSlot(slots.ToEpoch(slot - 1))
	require.NoError(t, err)
	require.NoError(t, head.UpdateBlockRootAtIndex(uint64(dependentSlot%params.BeaconConfig().SlotsPerHistoricalRoot), dependentRoot))
	committee, err := helpers.InclusionListCommittee(t.Context(), head, slot-1)
	require.NoError(t, err)
	require.NotEqual(t, committee[0], committee[1])
	bid, err := head.LatestExecutionPayloadBid()
	require.NoError(t, err)
	return &hezeProposal{
		slot:          slot,
		parentRoot:    [32]byte{1, 2, 3},
		parentHash:    bid.ParentBlockHash(),
		head:          head,
		committee:     committee,
		dependentRoot: dependentRoot,
		chain:         &chainMock.ChainService{},
	}
}

// addList stores a list of validator for the inclusion list slot under root.
func (p *hezeProposal) addList(t *testing.T, validator primitives.ValidatorIndex, root [32]byte, timely bool) {
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{{byte(validator), 1}})
	require.NoError(t, err)
	il := &ethpb.InclusionList{Slot: p.slot - 1, ValidatorIndex: validator, DependentRoot: root[:], Transactions: txs}
	p.chain.InclusionListStore().Process(&ethpb.SignedInclusionList{Message: il, Signature: make([]byte, 96)}, timely)
}

// bitsOf sets a bit for every committee position held by one of validators.
func (p *hezeProposal) bitsOf(validators ...primitives.ValidatorIndex) bitfield.Bitvector16 {
	bits := bitfield.NewBitvector16()
	for i, v := range p.committee {
		if slices.Contains(validators, v) {
			bits.SetBitAt(uint64(i), true)
		}
	}
	return bits
}

func (p *hezeProposal) block(t *testing.T) interfaces.SignedBeaconBlock {
	sBlk, err := getEmptyBlock(p.slot)
	require.NoError(t, err)
	require.Equal(t, version.Heze, sBlk.Version())
	sBlk.SetSlot(p.slot)
	sBlk.SetParentRoot(p.parentRoot[:])
	return sBlk
}

func (p *hezeProposal) local(t *testing.T) *consensusblocks.GetPayloadResponse {
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(&enginev1.ExecutionPayloadDeneb{
		ParentHash:    p.parentHash[:],
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	})
	require.NoError(t, err)
	return &consensusblocks.GetPayloadResponse{
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		BlobsBundler:           &enginev1.BlobsBundle{},
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}
}

func (p *hezeProposal) server(t *testing.T, bid *ethpb.SignedExecutionPayloadBidHeze) *Server {
	bidCache := cache.NewHighestExecutionPayloadBidCache()
	if bid != nil {
		ro, err := consensusblocks.WrappedROSignedExecutionPayloadBidHeze(bid)
		require.NoError(t, err)
		bidCache.SetIfHigher(ro)
	}
	return &Server{HighestBidCache: bidCache, ForkchoiceFetcher: p.chain}
}

func (p *hezeProposal) p2pBid(bits bitfield.Bitvector16) *ethpb.SignedExecutionPayloadBidHeze {
	return &ethpb.SignedExecutionPayloadBidHeze{
		Message: &ethpb.ExecutionPayloadBidHeze{
			Slot:                  p.slot,
			ParentBlockHash:       p.parentHash[:],
			ParentBlockRoot:       p.parentRoot[:],
			BlockHash:             make([]byte, 32),
			BuilderIndex:          5,
			Value:                 1000,
			ExecutionPayment:      500,
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			PrevRandao:            make([]byte, 32),
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: make([]byte, 32),
			InclusionListBits:     bits,
		},
		Signature: make([]byte, 96),
	}
}

// hezeBidProto returns the signed execution payload bid protobuf of a Heze block.
func hezeBidProto(t *testing.T, blk interfaces.ReadOnlySignedBeaconBlock) *ethpb.SignedExecutionPayloadBidHeze {
	pb, err := blk.Block().Body().Proto()
	require.NoError(t, err)
	body, ok := pb.(*ethpb.BeaconBlockBodyHeze)
	require.Equal(t, true, ok)
	return body.SignedExecutionPayloadBid
}

func TestSetExecutionPayloadBid_Heze(t *testing.T) {
	t.Run("an inclusive P2P bid is chosen", func(t *testing.T) {
		p := newHezeProposal(t)
		p.addList(t, p.committee[0], p.dependentRoot, true)
		sBlk := p.block(t)
		src, err := p.server(t, p.p2pBid(p.bitsOf(p.committee[0]))).setExecutionPayloadBid(t.Context(), sBlk, p.head, p.local(t), nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, bidSourceP2P, src)
		got := hezeBidProto(t, sBlk)
		require.Equal(t, primitives.BuilderIndex(5), got.Message.BuilderIndex)
		require.DeepEqual(t, p.bitsOf(p.committee[0]), got.Message.InclusionListBits)
	})
	for _, timely := range []bool{true, false} {
		// A late list counts too, because the proposer uses only_timely=False.
		t.Run(fmt.Sprintf("a bid without a timely=%v local list falls back to self-build", timely), func(t *testing.T) {
			p := newHezeProposal(t)
			p.addList(t, p.committee[0], p.dependentRoot, timely)
			hook := logTest.NewGlobal()
			sBlk := p.block(t)
			src, err := p.server(t, p.p2pBid(bitfield.NewBitvector16())).setExecutionPayloadBid(t.Context(), sBlk, p.head, p.local(t), nil, nil, false)
			require.NoError(t, err)
			require.Equal(t, bidSourceSelfBuild, src)
			require.LogsContain(t, hook, "inclusion list bits do not include the local inclusion list view")
			got := hezeBidProto(t, sBlk)
			require.Equal(t, params.BeaconConfig().BuilderIndexSelfBuild, got.Message.BuilderIndex)
			require.DeepEqual(t, p.bitsOf(p.committee[0]), got.Message.InclusionListBits)
		})
	}
	t.Run("a Gloas bid is skipped", func(t *testing.T) {
		p := newHezeProposal(t)
		heze := p.p2pBid(bitfield.NewBitvector16()).Message
		gloas, err := consensusblocks.WrappedROSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
			Message: &ethpb.ExecutionPayloadBid{
				Slot:                  heze.Slot,
				ParentBlockHash:       heze.ParentBlockHash,
				ParentBlockRoot:       heze.ParentBlockRoot,
				BlockHash:             heze.BlockHash,
				BuilderIndex:          heze.BuilderIndex,
				Value:                 heze.Value,
				ExecutionPayment:      heze.ExecutionPayment,
				FeeRecipient:          heze.FeeRecipient,
				GasLimit:              heze.GasLimit,
				PrevRandao:            heze.PrevRandao,
				BlobKzgCommitments:    heze.BlobKzgCommitments,
				ExecutionRequestsRoot: heze.ExecutionRequestsRoot,
			},
			Signature: make([]byte, 96),
		})
		require.NoError(t, err)
		vs := p.server(t, nil)
		vs.HighestBidCache.SetIfHigher(gloas)
		hook := logTest.NewGlobal()
		sBlk := p.block(t)
		src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, p.head, p.local(t), nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, bidSourceSelfBuild, src)
		require.LogsContain(t, hook, "it is not a Heze bid")
	})
	t.Run("a bid with extra bits is inclusive", func(t *testing.T) {
		p := newHezeProposal(t)
		sBlk := p.block(t)
		src, err := p.server(t, p.p2pBid([]byte{0xff, 0xff})).setExecutionPayloadBid(t.Context(), sBlk, p.head, p.local(t), nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, bidSourceP2P, src)
	})
	t.Run("the self-build bid carries the local view", func(t *testing.T) {
		p := newHezeProposal(t)
		p.addList(t, p.committee[0], p.dependentRoot, true)
		p.addList(t, p.committee[1], p.dependentRoot, false)
		p.addList(t, p.committee[2], [32]byte{'o'}, true)
		sBlk := p.block(t)
		src, err := p.server(t, nil).setExecutionPayloadBid(t.Context(), sBlk, p.head, p.local(t), nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, bidSourceSelfBuild, src)
		got := hezeBidProto(t, sBlk)
		require.DeepEqual(t, common.InfiniteSignature[:], got.Signature)
		require.Equal(t, p.slot, got.Message.Slot)
		require.DeepEqual(t, p.parentRoot[:], got.Message.ParentBlockRoot)
		require.DeepEqual(t, p.bitsOf(p.committee[0], p.committee[1]), got.Message.InclusionListBits)
	})
	t.Run("the fallback without a local payload skips a non-inclusive bid", func(t *testing.T) {
		p := newHezeProposal(t)
		p.addList(t, p.committee[0], p.dependentRoot, true)
		_, err := p.server(t, p.p2pBid(bitfield.NewBitvector16())).setRemoteBidFallback(t.Context(), p.block(t), p.head, false, nil, nil)
		require.ErrorContains(t, "no builder or cached P2P bid", err)

		sBlk := p.block(t)
		src, err := p.server(t, p.p2pBid(p.bitsOf(p.committee[0]))).setRemoteBidFallback(t.Context(), sBlk, p.head, false, nil, nil)
		require.NoError(t, err)
		require.Equal(t, bidSourceP2P, src)
		require.Equal(t, primitives.BuilderIndex(5), hezeBidProto(t, sBlk).Message.BuilderIndex)
	})
	t.Run("the builder API is not asked at Heze", func(t *testing.T) {
		p := newHezeProposal(t)
		vs := p.server(t, nil)
		vs.BlockBuilder = &builderTest.MockBuilderService{}
		cfg := &ethpb.BuilderConfig{Builders: []*ethpb.BuilderEntry{{Url: []byte("http://builder")}}}
		require.Equal(t, true, vs.requestBuilderBid(t.Context(), p.block(t).Block(), p.head, false, cfg) == nil)
	})
}
