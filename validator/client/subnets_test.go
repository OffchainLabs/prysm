package client

import (
	"context"
	"encoding/binary"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

// stubAggregatorSelector returns a fixed selection proof per pubkey so tests
// can drive isAggregator outcomes deterministically without touching BLS.
type stubAggregatorSelector struct {
	proofs map[[fieldparams.BLSPubkeyLength]byte][]byte
}

func (s *stubAggregatorSelector) RefreshSelectionProofs(context.Context) error { return nil }

func (s *stubAggregatorSelector) AttestationSelectionProof(_ context.Context, _ primitives.Slot, pk [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	sig, ok := s.proofs[pk]
	if !ok {
		return nil, errors.Errorf("no selection proof configured for pubkey %x", pk[:4])
	}
	return sig, nil
}

func (s *stubAggregatorSelector) ClaimAggregateSlot(primitives.Slot, primitives.CommitteeIndex) bool {
	return true
}

func (s *stubAggregatorSelector) SyncCommitteeAggregators(_ context.Context, _ primitives.Slot, pks [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error) {
	return pks, nil
}

func (s *stubAggregatorSelector) SyncCommitteeSelectionProofs(context.Context, primitives.Slot, [fieldparams.BLSPubkeyLength]byte, *ethpb.SyncSubcommitteeIndexResponse) ([][]byte, error) {
	return nil, nil
}

// Regression test for the bug where subscribeToSubnets cached isAggregator
// per (slot, committee). Two validators sharing the same (slot, committee)
// have independent aggregator outcomes because the selection proof is a BLS
// signature over their own pubkey — they MUST be evaluated independently.
func TestSubscribeToSubnets_AggregatorEvaluatedPerValidator(t *testing.T) {
	committeeLength := uint64(64)
	modulo := committeeLength / params.BeaconConfig().TargetAggregatorsPerCommittee
	require.Equal(t, true, modulo > 1, "test requires modulo > 1 so outcomes can differ")

	sigAgg, sigNotAgg := pickDistinguishingProofs(t, modulo)

	pkA := [fieldparams.BLSPubkeyLength]byte{0xaa}
	pkB := [fieldparams.BLSPubkeyLength]byte{0xbb}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)
	v := &validator{
		validatorClient: client,
		aggSelector: &stubAggregatorSelector{
			proofs: map[[fieldparams.BLSPubkeyLength]byte][]byte{
				pkA: sigAgg,
				pkB: sigNotAgg,
			},
		},
	}

	slot := primitives.Slot(10)
	committee := primitives.CommitteeIndex(3)
	duties := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{AttesterSlot: slot, CommitteeIndex: committee, CommitteeLength: committeeLength, PublicKey: pkA[:], Status: ethpb.ValidatorStatus_ACTIVE, ValidatorIndex: 1},
			{AttesterSlot: slot, CommitteeIndex: committee, CommitteeLength: committeeLength, PublicKey: pkB[:], Status: ethpb.ValidatorStatus_ACTIVE, ValidatorIndex: 2},
		},
	}

	var captured *ethpb.CommitteeSubnetsSubscribeRequest
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			captured = req
			return &emptypb.Empty{}, nil
		})

	require.NoError(t, v.subscribeToSubnets(t.Context(), duties))
	require.NotNil(t, captured)
	require.Equal(t, 2, len(captured.IsAggregator))
	// If a (slot, committee)-keyed cache short-circuits the second call,
	// both entries collapse to the first validator's outcome. They must not.
	assert.Equal(t, true, captured.IsAggregator[0], "pkA (sigAgg) should be aggregator")
	assert.Equal(t, false, captured.IsAggregator[1], "pkB (sigNotAgg) should not be aggregator")

	// Reversing the duty order must not flip outcomes either — i.e. neither
	// the first nor the second call may poison a shared cache.
	duties.CurrentEpochDuties[0], duties.CurrentEpochDuties[1] = duties.CurrentEpochDuties[1], duties.CurrentEpochDuties[0]
	captured = nil
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			captured = req
			return &emptypb.Empty{}, nil
		})
	require.NoError(t, v.subscribeToSubnets(t.Context(), duties))
	require.NotNil(t, captured)
	assert.Equal(t, false, captured.IsAggregator[0], "pkB still not aggregator when evaluated first")
	assert.Equal(t, true, captured.IsAggregator[1], "pkA still aggregator when evaluated second")
}

// pickDistinguishingProofs returns two stub selection proofs that map to opposite isAggregator outcomes.
func pickDistinguishingProofs(t *testing.T, modulo uint64) (agg, notAgg []byte) {
	t.Helper()
	for i := 0; i < 256; i++ {
		sig := []byte{byte(i)}
		h := hash.Hash(sig)
		isAgg := binary.LittleEndian.Uint64(h[:8])%modulo == 0
		if isAgg && agg == nil {
			agg = sig
		} else if !isAgg && notAgg == nil {
			notAgg = sig
		}
		if agg != nil && notAgg != nil {
			return agg, notAgg
		}
	}
	t.Fatalf("could not find distinguishing proofs for modulo=%d", modulo)
	return nil, nil
}
