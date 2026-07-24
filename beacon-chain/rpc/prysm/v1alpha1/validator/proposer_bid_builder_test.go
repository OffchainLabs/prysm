//go:build minimal

package validator

import (
	"math"
	"math/big"
	"testing"

	beaconbuilder "github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

// fakeBidVerifier implements verification.ExecutionPayloadBidVerifier with per-method
// configurable errors and captures the parent-linkage closures for assertion.
type fakeBidVerifier struct {
	slotErr, activeErr, versionErr, coverErr, blobErr, randaoErr, sigErr error
	rootSeenErr, parentHashErr, feeErr, gasErr                           error
	rootSeenFn                                                           func([32]byte) bool
	hasPayloadFn                                                         func([32]byte, [32]byte) bool
}

func (v *fakeBidVerifier) VerifyCurrentOrNextSlot() error             { return nil }
func (v *fakeBidVerifier) VerifyBidSlotMatches(primitives.Slot) error { return v.slotErr }
func (v *fakeBidVerifier) VerifyBuilderActive(state.ReadOnlyBeaconState) error {
	return v.activeErr
}
func (v *fakeBidVerifier) VerifyBuilderVersion(state.ReadOnlyBeaconState) error {
	return v.versionErr
}
func (v *fakeBidVerifier) VerifyExecutionPaymentZero() error      { return nil }
func (v *fakeBidVerifier) VerifyFeeRecipientMatches([]byte) error { return v.feeErr }
func (v *fakeBidVerifier) VerifyBlobKzgCommitmentsLimit() error   { return v.blobErr }
func (v *fakeBidVerifier) VerifyPrevRandao(state.ReadOnlyBeaconState) error {
	return v.randaoErr
}
func (v *fakeBidVerifier) VerifyParentBlockRootSeen(fn func([32]byte) bool) error {
	v.rootSeenFn = fn
	return v.rootSeenErr
}
func (v *fakeBidVerifier) VerifyBidSlotHigherThanParent(primitives.Slot) error { return nil }
func (v *fakeBidVerifier) VerifyParentBlockHash(fn func([32]byte, [32]byte) bool) error {
	v.hasPayloadFn = fn
	return v.parentHashErr
}
func (v *fakeBidVerifier) VerifyGasLimitTargetCompatible(uint64, uint64) error { return v.gasErr }
func (v *fakeBidVerifier) VerifyBuilderCanCoverBid(state.ReadOnlyBeaconState) error {
	return v.coverErr
}
func (v *fakeBidVerifier) VerifySignature(state.ReadOnlyBeaconState) error { return v.sigErr }
func (v *fakeBidVerifier) SatisfyRequirement(verification.Requirement)     {}

func newBid(value, payment primitives.Gwei, builderIndex primitives.BuilderIndex) *ethpb.SignedExecutionPayloadBid {
	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Value:            value,
			ExecutionPayment: payment,
			BuilderIndex:     builderIndex,
		},
	}
}

// localWithGwei builds a self-build candidate whose value is g Gwei.
func localWithGwei(g int64) *consensusblocks.GetPayloadResponse {
	return &consensusblocks.GetPayloadResponse{Bid: big.NewInt(g * 1_000_000_000)}
}

func TestEffectiveBidValue(t *testing.T) {
	tests := []struct {
		name       string
		value      primitives.Gwei
		payment    primitives.Gwei
		maxPayment uint64
		want       primitives.Gwei
	}{
		{"payment under cap", 1000, 300, 500, 1300},
		{"payment at cap", 1000, 500, 500, 1500},
		{"payment over cap is capped", 1000, 900, 500, 1500},
		{"zero payment", 1000, 0, 500, 1000},
		{"zero cap ignores payment", 1000, 900, 0, 1000},
		{"sum past uint64 saturates", math.MaxUint64 - 5, 100, math.MaxUint64, math.MaxUint64},
		{"max value zero payment unchanged", math.MaxUint64, 0, math.MaxUint64, math.MaxUint64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, effectiveBidValue(newBid(tt.value, tt.payment, 0), tt.maxPayment))
		})
	}
}

func TestBestBid(t *testing.T) {
	const (
		p2pIdx     = primitives.BuilderIndex(1)
		builderIdx = primitives.BuilderIndex(2)
	)
	tests := []struct {
		name       string
		local      *consensusblocks.GetPayloadResponse
		p2p        *ethpb.SignedExecutionPayloadBid
		builder    *ethpb.SignedExecutionPayloadBid
		maxPayment uint64
		wantSrc    bidSource
		wantNil    bool
	}{
		{name: "no remote bids", local: localWithGwei(100), wantSrc: bidSourceSelfBuild, wantNil: true},
		{name: "p2p beats local", local: localWithGwei(0), p2p: newBid(1000, 0, p2pIdx), wantSrc: bidSourceP2P},
		{name: "p2p below local", local: localWithGwei(2000), p2p: newBid(1000, 0, p2pIdx), wantSrc: bidSourceSelfBuild, wantNil: true},
		{name: "builder beats local", local: localWithGwei(0), builder: newBid(1000, 0, builderIdx), maxPayment: 1000, wantSrc: bidSourceBuilderAPI},
		{name: "builder below local", local: localWithGwei(2000), builder: newBid(1000, 0, builderIdx), maxPayment: 1000, wantSrc: bidSourceSelfBuild, wantNil: true},
		{name: "builder beats p2p", local: localWithGwei(0), p2p: newBid(1000, 0, p2pIdx), builder: newBid(2000, 0, builderIdx), maxPayment: 1000, wantSrc: bidSourceBuilderAPI},
		{name: "p2p beats builder", local: localWithGwei(0), p2p: newBid(2000, 0, p2pIdx), builder: newBid(1000, 0, builderIdx), maxPayment: 1000, wantSrc: bidSourceP2P},
		{name: "tie prefers p2p", local: localWithGwei(0), p2p: newBid(1000, 0, p2pIdx), builder: newBid(1000, 0, builderIdx), maxPayment: 1000, wantSrc: bidSourceP2P},
		{name: "payment cap keeps local ahead", local: localWithGwei(100), builder: newBid(50, 100, builderIdx), maxPayment: 40, wantSrc: bidSourceSelfBuild, wantNil: true},
		{name: "payment within cap wins", local: localWithGwei(100), builder: newBid(50, 100, builderIdx), maxPayment: 60, wantSrc: bidSourceBuilderAPI},
		{name: "saturated bid still beats local", local: localWithGwei(100), builder: newBid(math.MaxUint64-5, 100, builderIdx), maxPayment: math.MaxUint64, wantSrc: bidSourceBuilderAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := bestBid(tt.local, tt.p2p, tt.builder, tt.maxPayment)
			require.Equal(t, tt.wantSrc, src)
			if tt.wantNil {
				require.IsNil(t, got)
				return
			}
			require.NotNil(t, got)
			wantIdx := builderIdx
			if tt.wantSrc == bidSourceP2P {
				wantIdx = p2pIdx
			}
			require.Equal(t, wantIdx, got.Message.BuilderIndex)
		})
	}
}

func TestValidateBuilderBid(t *testing.T) {
	slot := primitives.Slot(100)
	parentRoot := [32]byte{1, 2, 3}
	parentHash := [32]byte{9, 9, 9}
	head, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	fullBid := func() *ethpb.SignedExecutionPayloadBid {
		return &ethpb.SignedExecutionPayloadBid{
			Message: &ethpb.ExecutionPayloadBid{
				Slot:             slot,
				ParentBlockRoot:  parentRoot[:],
				ParentBlockHash:  parentHash[:],
				BlockHash:        make([]byte, 32),
				PrevRandao:       make([]byte, 32),
				FeeRecipient:     make([]byte, 20),
				Value:            1000,
				ExecutionPayment: 100,
				BuilderIndex:     3,
			},
			Signature: make([]byte, 96),
		}
	}

	query := func(maxPayment uint64) *builderBidQuery {
		return &builderBidQuery{
			slot:       slot,
			parentRoot: parentRoot,
			parentHash: parentHash,
			maxPayment: maxPayment,
		}
	}

	t.Run("nil bid", func(t *testing.T) {
		vs := &Server{}
		require.ErrorContains(t, "nil builder bid", vs.validateBuilderBid(head, nil, query(1000)))
	})

	t.Run("payment exceeds max", func(t *testing.T) {
		vs := &Server{}
		err := vs.validateBuilderBid(head, fullBid(), query(50))
		require.ErrorContains(t, "exceeds max", err)
	})

	t.Run("verifier not ready", func(t *testing.T) {
		vs := &Server{}
		err := vs.validateBuilderBid(head, fullBid(), query(1000))
		require.ErrorContains(t, "bid verifier not ready", err)
	})

	t.Run("all checks pass", func(t *testing.T) {
		var captured *fakeBidVerifier
		vs := &Server{NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			captured = &fakeBidVerifier{}
			return captured
		}}
		require.NoError(t, vs.validateBuilderBid(head, fullBid(), query(1000)))

		// The parent-linkage closures must match only the block being produced.
		require.Equal(t, true, captured.rootSeenFn(parentRoot))
		require.Equal(t, false, captured.rootSeenFn([32]byte{7, 7, 7}))
		require.Equal(t, true, captured.hasPayloadFn(parentRoot, parentHash))
		require.Equal(t, false, captured.hasPayloadFn(parentRoot, [32]byte{8, 8, 8}))
	})

	t.Run("verifier check fails", func(t *testing.T) {
		vs := &Server{NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &fakeBidVerifier{sigErr: errors.New("bad signature")}
		}}
		err := vs.validateBuilderBid(head, fullBid(), query(1000))
		require.ErrorContains(t, "bad signature", err)
	})

	t.Run("fee recipient mismatch fails", func(t *testing.T) {
		vs := &Server{NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &fakeBidVerifier{feeErr: errors.New("fee recipient mismatch")}
		}}
		err := vs.validateBuilderBid(head, fullBid(), query(1000))
		require.ErrorContains(t, "fee recipient mismatch", err)
	})

	t.Run("gas limit incompatible fails", func(t *testing.T) {
		vs := &Server{NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &fakeBidVerifier{gasErr: errors.New("gas limit incompatible")}
		}}
		err := vs.validateBuilderBid(head, fullBid(), query(1000))
		require.ErrorContains(t, "gas limit incompatible", err)
	})
}

func TestGetBuilderExecutionPayloadBid(t *testing.T) {
	slot := primitives.Slot(100)
	parentRoot := [32]byte{1, 2, 3}
	parentHash := [32]byte{9, 9, 9}
	pubkey := [48]byte{4, 5, 6}
	auths := []*ethpb.SignedRequestAuthV1{{}}
	head, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	bid := func(builderIndex primitives.BuilderIndex, value primitives.Gwei) beaconbuilder.PayloadBid {
		return beaconbuilder.PayloadBid{
			BuilderURL: "http://builder",
			Bid: &ethpb.SignedExecutionPayloadBid{
				Message: &ethpb.ExecutionPayloadBid{
					Slot:            slot,
					ParentBlockRoot: parentRoot[:],
					ParentBlockHash: parentHash[:],
					BlockHash:       make([]byte, 32),
					PrevRandao:      make([]byte, 32),
					FeeRecipient:    make([]byte, 20),
					BuilderIndex:    builderIndex,
					Value:           value,
				},
				Signature: make([]byte, 96),
			},
		}
	}
	passAll := func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
		return &fakeBidVerifier{}
	}
	query := func(auths []*ethpb.SignedRequestAuthV1) *builderBidQuery {
		return &builderBidQuery{
			slot:       slot,
			parentRoot: parentRoot,
			parentHash: parentHash,
			pubkey:     pubkey,
			auths:      auths,
		}
	}

	t.Run("no builder configured", func(t *testing.T) {
		vs := &Server{}
		got, url := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(auths))
		require.IsNil(t, got)
		require.Equal(t, "", url)
	})

	t.Run("no auths", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{}}
		got, _ := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(nil))
		require.IsNil(t, got)
	})

	t.Run("picks highest valid bid", func(t *testing.T) {
		vs := &Server{
			BlockBuilder:                   &builderTest.MockBuilderService{PayloadBids: []beaconbuilder.PayloadBid{bid(1, 500), bid(2, 1500), bid(3, 900)}},
			NewExecutionPayloadBidVerifier: passAll,
		}
		got, url := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(auths))
		require.NotNil(t, got)
		require.Equal(t, primitives.BuilderIndex(2), got.Message.BuilderIndex)
		require.Equal(t, "http://builder", url)
	})

	t.Run("discards invalid bids", func(t *testing.T) {
		vs := &Server{
			BlockBuilder: &builderTest.MockBuilderService{PayloadBids: []beaconbuilder.PayloadBid{bid(1, 500), bid(2, 1500)}},
			NewExecutionPayloadBidVerifier: func(b interfaces.ROSignedExecutionPayloadBid, _ []verification.Requirement) verification.ExecutionPayloadBidVerifier {
				wrapped, err := b.Bid()
				require.NoError(t, err)
				if wrapped.BuilderIndex() == 2 {
					return &fakeBidVerifier{activeErr: errors.New("not active")}
				}
				return &fakeBidVerifier{}
			},
		}
		got, _ := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(auths))
		require.NotNil(t, got)
		require.Equal(t, primitives.BuilderIndex(1), got.Message.BuilderIndex)
	})

	t.Run("nil when all invalid", func(t *testing.T) {
		vs := &Server{
			BlockBuilder: &builderTest.MockBuilderService{PayloadBids: []beaconbuilder.PayloadBid{bid(1, 500)}},
			NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
				return &fakeBidVerifier{activeErr: errors.New("not active")}
			},
		}
		got, url := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(auths))
		require.IsNil(t, got)
		require.Equal(t, "", url)
	})

	t.Run("nil on builder error", func(t *testing.T) {
		vs := &Server{
			BlockBuilder:                   &builderTest.MockBuilderService{ErrGetExecutionPayloadBid: errors.New("boom")},
			NewExecutionPayloadBidVerifier: passAll,
		}
		got, _ := vs.getBuilderExecutionPayloadBid(t.Context(), head, query(auths))
		require.IsNil(t, got)
	})
}

func TestSetExecutionPayloadBid_PrefersBuilderBid(t *testing.T) {
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

	builderBid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       parentHash[:],
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

	// No P2P cache, so the Builder-API bid is the only remote candidate.
	vs := &Server{}
	src, err := vs.setExecutionPayloadBid(t.Context(), sBlk, local, builderBid, 1000, false)
	require.NoError(t, err)
	require.Equal(t, bidSourceBuilderAPI, src)

	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.Equal(t, primitives.BuilderIndex(7), signedBid.Message.BuilderIndex)
	require.Equal(t, primitives.Gwei(1000), signedBid.Message.Value)
}
