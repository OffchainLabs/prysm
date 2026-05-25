package verification

import (
	"bytes"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestBidVerifier_VerifyCurrentOrNextSlot(t *testing.T) {
	currentSlot := primitives.Slot(10)
	clock := testClockAtSlot(t, currentSlot)

	currentBid := testSignedExecutionPayloadBid(t, currentSlot)
	currentWrapped, err := blocks.WrappedROSignedExecutionPayloadBid(currentBid)
	require.NoError(t, err)
	verifier := &BidVerifier{
		sharedResources: &sharedResources{clock: clock},
		results:         newResults(RequireBidCurrentOrNextSlot),
		b:               currentWrapped,
	}
	require.NoError(t, verifier.VerifyCurrentOrNextSlot())

	nextBid := testSignedExecutionPayloadBid(t, currentSlot+1)
	nextWrapped, err := blocks.WrappedROSignedExecutionPayloadBid(nextBid)
	require.NoError(t, err)
	verifier = &BidVerifier{
		sharedResources: &sharedResources{clock: clock},
		results:         newResults(RequireBidCurrentOrNextSlot),
		b:               nextWrapped,
	}
	require.NoError(t, verifier.VerifyCurrentOrNextSlot())

	futureBid := testSignedExecutionPayloadBid(t, currentSlot+2)
	futureWrapped, err := blocks.WrappedROSignedExecutionPayloadBid(futureBid)
	require.NoError(t, err)
	verifier = &BidVerifier{
		sharedResources: &sharedResources{clock: clock},
		results:         newResults(RequireBidCurrentOrNextSlot),
		b:               futureWrapped,
	}
	require.ErrorIs(t, verifier.VerifyCurrentOrNextSlot(), ErrBidSlotNotCurrentOrNext)
}

func TestBidVerifier_VerifyBuilderActive(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	activeState := newBidState(t, 1, func(s *ethpb.BeaconStateGloas) {
		s.Builders = []*ethpb.Builder{{
			Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + 100),
			DepositEpoch:      0,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}}
		s.FinalizedCheckpoint = &ethpb.Checkpoint{Epoch: 1}
	})
	verifier := &BidVerifier{results: newResults(RequireBidBuilderActive), b: wrapped}
	require.NoError(t, verifier.VerifyBuilderActive(activeState))

	inactiveState := newBidState(t, 1, func(s *ethpb.BeaconStateGloas) {
		s.Builders = []*ethpb.Builder{{
			Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + 100),
			DepositEpoch:      0,
			WithdrawableEpoch: 2,
		}}
		s.FinalizedCheckpoint = &ethpb.Checkpoint{Epoch: 3}
	})
	verifier = &BidVerifier{results: newResults(RequireBidBuilderActive), b: wrapped}
	require.ErrorIs(t, verifier.VerifyBuilderActive(inactiveState), ErrBidBuilderNotActive)
}

func TestBidVerifier_VerifyExecutionPaymentZero(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	verifier := &BidVerifier{results: newResults(RequireBidExecutionPaymentZero), b: wrapped}
	require.NoError(t, verifier.VerifyExecutionPaymentZero())

	signed.Message.ExecutionPayment = 100
	wrapped, err = blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)
	verifier = &BidVerifier{results: newResults(RequireBidExecutionPaymentZero), b: wrapped}
	require.ErrorIs(t, verifier.VerifyExecutionPaymentZero(), ErrBidExecutionPaymentNonZero)
}

func TestBidVerifier_VerifyFeeRecipientMatches(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	verifier := &BidVerifier{results: newResults(RequireBidFeeRecipientMatches), b: wrapped}
	require.NoError(t, verifier.VerifyFeeRecipientMatches(signed.Message.FeeRecipient))

	verifier = &BidVerifier{results: newResults(RequireBidFeeRecipientMatches), b: wrapped}
	require.ErrorIs(t, verifier.VerifyFeeRecipientMatches(bytes.Repeat([]byte{0xff}, 20)), ErrBidFeeRecipientMismatch)
}

func TestBidVerifier_VerifyGasLimitTargetCompatible(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	// bid.gas_limit is 1 (from testSignedExecutionPayloadBid). With parent=1 the
	// elasticity rule allows only gas_limit=1, so compatible target is 1.
	parentGasLimit := signed.Message.GasLimit

	verifier := &BidVerifier{results: newResults(RequireBidGasLimitCompatible), b: wrapped}
	require.NoError(t, verifier.VerifyGasLimitTargetCompatible(parentGasLimit, signed.Message.GasLimit))

	verifier = &BidVerifier{results: newResults(RequireBidGasLimitCompatible), b: wrapped}
	require.ErrorIs(t, verifier.VerifyGasLimitTargetCompatible(parentGasLimit, signed.Message.GasLimit+1), ErrBidGasLimitIncompatible)
}

func TestBidVerifier_VerifyParentBlockRootSeen(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	verifier := &BidVerifier{results: newResults(RequireBidParentBlockRootSeen), b: wrapped}
	require.NoError(t, verifier.VerifyParentBlockRootSeen(func(root [32]byte) bool {
		return root == [32]byte(signed.Message.ParentBlockRoot)
	}))

	verifier = &BidVerifier{results: newResults(RequireBidParentBlockRootSeen), b: wrapped}
	require.ErrorIs(t, verifier.VerifyParentBlockRootSeen(func([32]byte) bool { return false }), ErrBidParentBlockRootNotSeen)
}

func TestBidVerifier_VerifyParentBlockHash(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	wantHash := [32]byte(signed.Message.ParentBlockHash)
	verifier := &BidVerifier{results: newResults(RequireBidParentBlockHashValid), b: wrapped}
	require.NoError(t, verifier.VerifyParentBlockHash(func([32]byte) ([32]byte, error) {
		return wantHash, nil
	}))

	verifier = &BidVerifier{results: newResults(RequireBidParentBlockHashValid), b: wrapped}
	require.ErrorIs(t, verifier.VerifyParentBlockHash(func([32]byte) ([32]byte, error) {
		return [32]byte{0xFF}, nil
	}), ErrBidParentBlockHashMismatch)
}

func TestBidVerifier_VerifyBuilderCanCoverBid(t *testing.T) {
	signed := testSignedExecutionPayloadBid(t, 1)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	coverState := newBidState(t, 1, func(s *ethpb.BeaconStateGloas) {
		s.Builders = []*ethpb.Builder{{
			Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + uint64(signed.Message.Value) + 100),
			DepositEpoch:      0,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}}
		s.FinalizedCheckpoint = &ethpb.Checkpoint{Epoch: 1}
	})
	verifier := &BidVerifier{results: newResults(RequireBidBuilderCanCover), b: wrapped}
	require.NoError(t, verifier.VerifyBuilderCanCoverBid(coverState))

	insufficientState := newBidState(t, 1, func(s *ethpb.BeaconStateGloas) {
		s.Builders = []*ethpb.Builder{{
			Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + uint64(signed.Message.Value) - 1),
			DepositEpoch:      0,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}}
		s.FinalizedCheckpoint = &ethpb.Checkpoint{Epoch: 1}
	})
	verifier = &BidVerifier{results: newResults(RequireBidBuilderCanCover), b: wrapped}
	require.ErrorIs(t, verifier.VerifyBuilderCanCoverBid(insufficientState), ErrBidBuilderCannotCover)
}

func TestBidVerifier_VerifySignature(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	builderPubkey := sk.PublicKey().Marshal()
	signed := testSignedExecutionPayloadBid(t, 4)
	signed.Message.BuilderIndex = 0
	st := newBidState(t, 4, func(s *ethpb.BeaconStateGloas) {
		s.Builders = []*ethpb.Builder{{
			Pubkey:            builderPubkey,
			Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + 100),
			DepositEpoch:      0,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}}
		s.FinalizedCheckpoint = &ethpb.Checkpoint{Epoch: 1}
	})

	sig := signBidForState(t, sk, signed.Message, st)
	signed.Signature = sig[:]
	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)

	verifier := &BidVerifier{results: newResults(RequireBidSignatureValid), b: wrapped}
	require.NoError(t, verifier.VerifySignature(st))

	badKey, err := bls.RandKey()
	require.NoError(t, err)
	badSig := signBidForState(t, badKey, signed.Message, st)
	signed.Signature = badSig[:]
	wrapped, err = blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)
	verifier = &BidVerifier{results: newResults(RequireBidSignatureValid), b: wrapped}
	require.ErrorIs(t, verifier.VerifySignature(st), signing.ErrSigFailedToVerify)
}

func testSignedExecutionPayloadBid(t *testing.T, slot primitives.Slot) *ethpb.SignedExecutionPayloadBid {
	t.Helper()

	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			BuilderIndex:          0,
			ParentBlockHash:       bytes.Repeat([]byte{0x01}, 32),
			ParentBlockRoot:       bytes.Repeat([]byte{0x02}, 32),
			BlockHash:             bytes.Repeat([]byte{0x03}, 32),
			PrevRandao:            bytes.Repeat([]byte{0x04}, 32),
			FeeRecipient:          bytes.Repeat([]byte{0x05}, 20),
			GasLimit:              30_000_000,
			Value:                 100,
			ExecutionPayment:      0,
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: bytes.Repeat([]byte{0x06}, 96),
	}
}

func newBidState(t *testing.T, slot primitives.Slot, mutate func(*ethpb.BeaconStateGloas)) state.BeaconState {
	t.Helper()

	genesisRoot := bytes.Repeat([]byte{0x11}, 32)
	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.Slot = slot
		s.GenesisValidatorsRoot = genesisRoot
		if mutate != nil {
			mutate(s)
		}
		return nil
	})
	require.NoError(t, err)
	return st
}

func signBidForState(t *testing.T, sk bls.SecretKey, bid *ethpb.ExecutionPayloadBid, st state.ReadOnlyBeaconState) [96]byte {
	t.Helper()

	epoch := slots.ToEpoch(bid.Slot)
	domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(bid, domain)
	require.NoError(t, err)
	return [96]byte(sk.Sign(root[:]).Marshal())
}

func testClockAtSlot(t *testing.T, slot primitives.Slot) *startup.Clock {
	t.Helper()

	secondsPerSlot := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	genesis := time.Now().Add(-time.Duration(slot)*secondsPerSlot - secondsPerSlot/2)
	return startup.NewClock(genesis, [32]byte{})
}
