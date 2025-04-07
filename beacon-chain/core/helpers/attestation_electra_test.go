package helpers_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// TestComputeSubnetForAttestationElectra tests subnet computation specifically for Electra attestations
func TestComputeSubnetForAttestationElectra(t *testing.T) {
	helpers.ClearCache()

	// Create 10 committees
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize
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

	state, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Validators:  validators,
		Slot:        200,
		BlockRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		StateRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	valCount, err := helpers.ActiveValidatorCount(context.Background(), state, slots.ToEpoch(34))
	require.NoError(t, err)

	// Test with an Electra attestation
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(4, true) // Set committee index 4
	att := &ethpb.AttestationElectra{
		AggregationBits: []byte{'A'},
		CommitteeBits:   cb,
		Data: &ethpb.AttestationData{
			Slot:            34,
			BeaconBlockRoot: []byte{'C'},
		},
		Signature: []byte{'B'},
	}
	
	// Verify the attestation is recognized as Electra
	assert.Equal(t, 5, att.Version(), "Expected attestation version to be Electra")
	
	// Compute the subnet
	sub := helpers.ComputeSubnetForAttestation(valCount, att)
	assert.Equal(t, uint64(6), sub, "Did not get correct subnet for Electra attestation")
} 
