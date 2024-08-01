package helpers_test

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/epbs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/math"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
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
		committeeCount     int
		memberPerCommittee uint64
		committeesPerSlot  uint64
	}{
		{1, 512, 1},
		{4, 128, 4},
		{128, 4, 128},
		{512, 1, 512},
		{1024, 1, 512},
	}

	for _, test := range tests {
		committeesPerSlot, memberPerCommittee := helpers.PtcAllocation(test.committeeCount)
		if memberPerCommittee != test.memberPerCommittee {
			t.Errorf("memberPerCommittee(%d) = %d; expected %d", test.committeeCount, memberPerCommittee, test.memberPerCommittee)
		}
		if committeesPerSlot != test.committeesPerSlot {
			t.Errorf("committeesPerSlot(%d) = %d; expected %d", test.committeeCount, committeesPerSlot, test.committeesPerSlot)
		}
	}
}

func TestGetPayloadAttestingIndices(t *testing.T) {
	helpers.ClearCache()

	// Create 10 committees. Total 40960 validators.
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize * uint64(params.BeaconConfig().SlotsPerEpoch)
	validators := make([]*ethpb.Validator, validatorCount)

	for i := 0; i < len(validators); i++ {
		pubkey := make([]byte, 48)
		copy(pubkey, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey:             pubkey,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	// Create a beacon state.
	state, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)

	// Get PTC.
	ptc, err := helpers.GetPayloadTimelinessCommittee(context.Background(), state, state.Slot())
	require.NoError(t, err)
	require.Equal(t, fieldparams.PTCSize, len(ptc))

	// Generate random indices. PTC members at the corresponding indices are considered attested.
	randGen := rand.NewDeterministicGenerator()
	attesterCount := randGen.Intn(fieldparams.PTCSize) + 1
	indices := randGen.Perm(fieldparams.PTCSize)[:attesterCount]
	slices.Sort(indices)
	require.Equal(t, attesterCount, len(indices))

	// Create a PayloadAttestation with AggregationBits set true at the indices.
	aggregationBits := bitfield.NewBitvector512()
	for _, index := range indices {
		aggregationBits.SetBitAt(uint64(index), true)
	}

	payloadAttestation := &eth.PayloadAttestation{
		AggregationBits: aggregationBits,
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	// Get attesting indices.
	attesters, err := helpers.GetPayloadAttestingIndices(context.Background(), state, state.Slot(), payloadAttestation)
	require.NoError(t, err)
	require.Equal(t, len(indices), len(attesters))

	// Check if each attester equals to the PTC member at the corresponding index.
	for i, index := range indices {
		require.Equal(t, attesters[i], ptc[index])
	}
}

func TestGetIndexedPayloadAttestation(t *testing.T) {
	helpers.ClearCache()

	// Create 10 committees. Total 40960 validators.
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize * uint64(params.BeaconConfig().SlotsPerEpoch)
	validators := make([]*ethpb.Validator, validatorCount)

	for i := 0; i < len(validators); i++ {
		publicKey := make([]byte, 48)
		copy(publicKey, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey:             publicKey,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	// Create a beacon state.
	state, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)

	// Get PTC.
	ptc, err := helpers.GetPayloadTimelinessCommittee(context.Background(), state, state.Slot())
	require.NoError(t, err)
	require.Equal(t, fieldparams.PTCSize, len(ptc))

	// Generate random indices. PTC members at the corresponding indices are considered attested.
	randGen := rand.NewDeterministicGenerator()
	attesterCount := randGen.Intn(fieldparams.PTCSize) + 1
	indices := randGen.Perm(fieldparams.PTCSize)[:attesterCount]
	slices.Sort(indices)
	require.Equal(t, attesterCount, len(indices))

	// Create a PayloadAttestation with AggregationBits set true at the indices.
	aggregationBits := bitfield.NewBitvector512()
	for _, index := range indices {
		aggregationBits.SetBitAt(uint64(index), true)
	}

	payloadAttestation := &eth.PayloadAttestation{
		AggregationBits: aggregationBits,
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	// Get attesting indices.
	ctx := context.Background()
	attesters, err := helpers.GetPayloadAttestingIndices(ctx, state, state.Slot(), payloadAttestation)
	require.NoError(t, err)
	require.Equal(t, len(indices), len(attesters))

	// Get an IndexedPayloadAttestation.
	indexedPayloadAttestation, err := helpers.GetIndexedPayloadAttestation(ctx, state, state.Slot(), payloadAttestation)
	require.NoError(t, err)
	require.Equal(t, len(indices), len(indexedPayloadAttestation.AttestingIndices))
	require.DeepEqual(t, payloadAttestation.Data, indexedPayloadAttestation.Data)
	require.DeepEqual(t, payloadAttestation.Signature, indexedPayloadAttestation.Signature)

	// Check if the attesting indices are the same.
	slices.Sort(attesters) // GetIndexedPayloadAttestation sorts attesting indices.
	require.DeepEqual(t, attesters, indexedPayloadAttestation.AttestingIndices)
}

func TestIsValidIndexedPayloadAttestation(t *testing.T) {
	helpers.ClearCache()

	// Create validators.
	validatorCount := uint64(350)
	validators := make([]*ethpb.Validator, validatorCount)
	_, secretKeys, err := util.DeterministicDepositsAndKeys(validatorCount)
	require.NoError(t, err)

	for i := 0; i < len(validators); i++ {
		validators[i] = &ethpb.Validator{
			PublicKey:             secretKeys[i].PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	// Create a beacon state.
	state, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators: validators,
		Fork: &ethpb.Fork{
			Epoch:           0,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)

	// Define test cases.
	tests := []struct {
		attestation *epbs.IndexedPayloadAttestation
	}{
		{
			attestation: &epbs.IndexedPayloadAttestation{
				AttestingIndices: []primitives.ValidatorIndex{1},
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
		{
			attestation: &epbs.IndexedPayloadAttestation{
				AttestingIndices: []primitives.ValidatorIndex{13, 19},
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
		{
			attestation: &epbs.IndexedPayloadAttestation{
				AttestingIndices: []primitives.ValidatorIndex{123, 234, 345},
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
		{
			attestation: &epbs.IndexedPayloadAttestation{
				AttestingIndices: []primitives.ValidatorIndex{38, 46, 54, 62, 70, 78, 86, 194},
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
		{
			attestation: &epbs.IndexedPayloadAttestation{
				AttestingIndices: []primitives.ValidatorIndex{5},
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
	}

	// Run test cases.
	for _, test := range tests {
		signatures := make([]bls.Signature, len(test.attestation.AttestingIndices))
		for i, index := range test.attestation.AttestingIndices {
			signedBytes, err := signing.ComputeDomainAndSign(
				state,
				slots.ToEpoch(test.attestation.Data.Slot),
				test.attestation.Data,
				params.BeaconConfig().DomainPTCAttester,
				secretKeys[index],
			)
			require.NoError(t, err)

			signature, err := bls.SignatureFromBytes(signedBytes)
			require.NoError(t, err)

			signatures[i] = signature
		}

		aggregatedSignature := bls.AggregateSignatures(signatures)
		test.attestation.Signature = aggregatedSignature.Marshal()

		isValid, err := helpers.IsValidIndexedPayloadAttestation(state, test.attestation)
		require.NoError(t, err)
		require.Equal(t, true, isValid)
	}
}
