package helpers_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/math"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestValidateNilPayloadAttestation(t *testing.T) {
	require.ErrorIs(t, helpers.ErrNilData, helpers.ValidateNilPayloadAttestationData(nil))
	data := &eth.PayloadAttestationData{}
	require.ErrorIs(t, helpers.ErrNilBeaconBlockRoot, helpers.ValidateNilPayloadAttestationData(data))
	data.BeaconBlockRoot = make([]byte, 32)
	require.NoError(t, helpers.ValidateNilPayloadAttestationData(data))

	require.ErrorIs(t, helpers.ErrNilMessage, helpers.ValidateNilPayloadAttestationMessage(nil))
	message := &eth.PayloadAttestationMessage{}
	require.ErrorIs(t, helpers.ErrNilSignature, helpers.ValidateNilPayloadAttestationMessage(message))
	message.Signature = make([]byte, 96)
	require.ErrorIs(t, helpers.ErrNilData, helpers.ValidateNilPayloadAttestationMessage(message))
	message.Data = data
	require.NoError(t, helpers.ValidateNilPayloadAttestationMessage(message))

	require.ErrorIs(t, helpers.ErrNilPayloadAttestation, helpers.ValidateNilPayloadAttestation(nil))
	att := &eth.PayloadAttestation{}
	require.ErrorIs(t, helpers.ErrNilAggregationBits, helpers.ValidateNilPayloadAttestation(att))
	att.AggregationBits = bitfield.NewBitvector512()
	require.ErrorIs(t, helpers.ErrNilSignature, helpers.ValidateNilPayloadAttestation(att))
	att.Signature = message.Signature
	require.ErrorIs(t, helpers.ErrNilData, helpers.ValidateNilPayloadAttestation(att))
	att.Data = data
	require.NoError(t, helpers.ValidateNilPayloadAttestation(att))
}

func TestGetPayloadTimelinessCommittee(t *testing.T) {
	helpers.ClearCache()

	// Create 10 committees
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize * uint64(params.BeaconConfig().SlotsPerEpoch)
	validators := make([]*ethpb.Validator, validatorCount)

	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey:             k,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	state, err := state_native.InitializeFromProtoEpbs(random.BeaconState(t))
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(200))

	ctx := context.Background()
	indices, err := helpers.BeaconCommitteeFromState(ctx, state, state.Slot(), 1)
	require.NoError(t, err)
	require.Equal(t, 128, len(indices))

	epoch := slots.ToEpoch(state.Slot())
	activeCount, err := helpers.ActiveValidatorCount(ctx, state, epoch)
	require.NoError(t, err)
	require.Equal(t, uint64(40960), activeCount)

	computedCommitteeCount := helpers.SlotCommitteeCount(activeCount)
	require.Equal(t, committeeCount, computedCommitteeCount)
	committeesPerSlot := math.LargestPowerOfTwo(math.Min(committeeCount, fieldparams.PTCSize))
	require.Equal(t, uint64(8), committeesPerSlot)

	ptc, err := helpers.GetPayloadTimelinessCommittee(ctx, state, state.Slot())
	require.NoError(t, err)
	require.Equal(t, fieldparams.PTCSize, len(ptc))

	committee1, err := helpers.BeaconCommitteeFromState(ctx, state, state.Slot(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, committee1[:64], ptc[:64])
}

func Test_PtcAllocation(t *testing.T) {
	tests := []struct {
		totalActive        uint64
		memberPerCommittee uint64
		committeesPerSlot  uint64
	}{
		{64, 512, 1},
		{params.BeaconConfig().MinGenesisActiveValidatorCount, 128, 4},
		{25600, 128, 4},
		{256000, 16, 32},
		{1024000, 8, 64},
	}

	for _, test := range tests {
		committeesPerSlot, memberPerCommittee := helpers.PtcAllocation(test.totalActive)
		if memberPerCommittee != test.memberPerCommittee {
			t.Errorf("memberPerCommittee(%d) = %d; expected %d", test.totalActive, memberPerCommittee, test.memberPerCommittee)
		}
		if committeesPerSlot != test.committeesPerSlot {
			t.Errorf("committeesPerSlot(%d) = %d; expected %d", test.totalActive, committeesPerSlot, test.committeesPerSlot)
		}
	}
}
