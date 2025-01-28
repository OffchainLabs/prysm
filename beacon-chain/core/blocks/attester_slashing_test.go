package blocks_test

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	v "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/validators"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestSlashableAttestationData_CanSlash(t *testing.T) {
	att1 := util.HydrateAttestationData(&ethpb.AttestationData{
		Target: &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'A'}, 32)},
	})
	att2 := util.HydrateAttestationData(&ethpb.AttestationData{
		Target: &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'B'}, 32)},
	})
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
	att1.Target.Epoch = 4
	att1.Source.Epoch = 2
	att2.Source.Epoch = 3
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
}

func TestProcessAttesterSlashings_DataNotSlashable(t *testing.T) {
	slashings := []*ethpb.AttesterSlashing{{
		Attestation_1: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{}),
		Attestation_2: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
			Data: &ethpb.AttestationData{
				Source: &ethpb.Checkpoint{Epoch: 1},
				Target: &ethpb.Checkpoint{Epoch: 1}},
		})}}

	var registry []*ethpb.Validator
	currentSlot := primitives.Slot(0)

	beaconState, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}
	ss := make([]ethpb.AttSlashing, len(b.Block.Body.AttesterSlashings))
	for i, s := range b.Block.Body.AttesterSlashings {
		ss[i] = s
	}
	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, ss, v.SlashValidator)
	assert.ErrorContains(t, "attestations are not slashable", err)
}

func TestProcessAttesterSlashings_IndexedAttestationFailedToVerify(t *testing.T) {
	var registry []*ethpb.Validator
	currentSlot := primitives.Slot(0)

	beaconState, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)

	slashings := []*ethpb.AttesterSlashing{
		{
			Attestation_1: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 1},
				},
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
			Attestation_2: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
		},
	}

	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}

	ss := make([]ethpb.AttSlashing, len(b.Block.Body.AttesterSlashings))
	for i, s := range b.Block.Body.AttesterSlashings {
		ss[i] = s
	}
	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, ss, v.SlashValidator)
	assert.ErrorContains(t, "validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE", err)
}

func TestProcessAttesterSlashings_AppliesCorrectStatus(t *testing.T) {
	statePhase0, keysPhase0 := util.DeterministicGenesisState(t, 100)
	stateAltair, keysAltair := util.DeterministicGenesisStateAltair(t, 100)
	stateBellatrix, keysBellatrix := util.DeterministicGenesisStateBellatrix(t, 100)
	stateCapella, keysCapella := util.DeterministicGenesisStateCapella(t, 100)
	stateDeneb, keysDeneb := util.DeterministicGenesisStateDeneb(t, 100)
	stateElectra, keysElectra := util.DeterministicGenesisStateElectra(t, 100)

	att1Phase0 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	att2Phase0 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	att1Electra := util.HydrateIndexedAttestationElectra(&ethpb.IndexedAttestationElectra{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	att2Electra := util.HydrateIndexedAttestationElectra(&ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{0, 1},
	})

	slashingPhase0 := &ethpb.AttesterSlashing{
		Attestation_1: att1Phase0,
		Attestation_2: att2Phase0,
	}
	slashingElectra := &ethpb.AttesterSlashingElectra{
		Attestation_1: att1Electra,
		Attestation_2: att2Electra,
	}

	type testCase struct {
		name           string
		st             state.BeaconState
		keys           []bls.SecretKey
		att1           ethpb.IndexedAtt
		att2           ethpb.IndexedAtt
		slashing       ethpb.AttSlashing
		slashedBalance uint64
	}

	testCases := []testCase{
		{
			name:           "phase0",
			st:             statePhase0,
			keys:           keysPhase0,
			att1:           att1Phase0,
			att2:           att2Phase0,
			slashing:       slashingPhase0,
			slashedBalance: 31750000000,
		},
		{
			name:           "altair",
			st:             stateAltair,
			keys:           keysAltair,
			att1:           att1Phase0,
			att2:           att2Phase0,
			slashing:       slashingPhase0,
			slashedBalance: 31500000000,
		},
		{
			name:           "bellatrix",
			st:             stateBellatrix,
			keys:           keysBellatrix,
			att1:           att1Phase0,
			att2:           att2Phase0,
			slashing:       slashingPhase0,
			slashedBalance: 31000000000,
		},
		{
			name:           "capella",
			st:             stateCapella,
			keys:           keysCapella,
			att1:           att1Phase0,
			att2:           att2Phase0,
			slashing:       slashingPhase0,
			slashedBalance: 31000000000,
		},
		{
			name:           "deneb",
			st:             stateDeneb,
			keys:           keysDeneb,
			att1:           att1Phase0,
			att2:           att2Phase0,
			slashing:       slashingPhase0,
			slashedBalance: 31000000000,
		},
		{
			name:           "electra",
			st:             stateElectra,
			keys:           keysElectra,
			att1:           att1Electra,
			att2:           att2Electra,
			slashing:       slashingElectra,
			slashedBalance: 31992187500,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, vv := range tc.st.Validators() {
				vv.WithdrawableEpoch = primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)
			}

			domain, err := signing.Domain(tc.st.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, tc.st.GenesisValidatorsRoot())
			require.NoError(t, err)
			signingRoot, err := signing.ComputeSigningRoot(tc.att1.GetData(), domain)
			assert.NoError(t, err, "Could not get signing root of beacon block header")
			sig0 := tc.keys[0].Sign(signingRoot[:])
			sig1 := tc.keys[1].Sign(signingRoot[:])
			aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})

			if tc.att1.Version() >= version.Electra {
				tc.att1.(*ethpb.IndexedAttestationElectra).Signature = aggregateSig.Marshal()
			} else {
				tc.att1.(*ethpb.IndexedAttestation).Signature = aggregateSig.Marshal()
			}

			signingRoot, err = signing.ComputeSigningRoot(tc.att2.GetData(), domain)
			assert.NoError(t, err, "Could not get signing root of beacon block header")
			sig0 = tc.keys[0].Sign(signingRoot[:])
			sig1 = tc.keys[1].Sign(signingRoot[:])
			aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})

			if tc.att2.Version() >= version.Electra {
				tc.att2.(*ethpb.IndexedAttestationElectra).Signature = aggregateSig.Marshal()
			} else {
				tc.att2.(*ethpb.IndexedAttestation).Signature = aggregateSig.Marshal()
			}

			currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
			require.NoError(t, tc.st.SetSlot(currentSlot))

			newState, err := blocks.ProcessAttesterSlashings(context.Background(), tc.st, []ethpb.AttSlashing{tc.slashing}, v.SlashValidator)
			require.NoError(t, err)
			newRegistry := newState.Validators()

			// Given the intersection of slashable indices is [1], only validator
			// at index 1 should be slashed and exited. We confirm this below.
			if newRegistry[1].ExitEpoch != tc.st.Validators()[1].ExitEpoch {
				t.Errorf(
					`
			Expected validator at index 1's exit epoch to match
			%d, received %d instead
			`,
					tc.st.Validators()[1].ExitEpoch,
					newRegistry[1].ExitEpoch,
				)
			}

			require.Equal(t, tc.slashedBalance, newState.Balances()[1])
			require.Equal(t, uint64(32000000000), newState.Balances()[2])
		})
	}
}
