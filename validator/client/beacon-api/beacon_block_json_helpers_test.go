package beacon_api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestBeaconBlockJsonHelpers_JsonifyBlsToExecutionChanges(t *testing.T) {
	input := []*ethpb.SignedBLSToExecutionChange{
		{
			Message: &ethpb.BLSToExecutionChange{
				ValidatorIndex:     1,
				FromBlsPubkey:      []byte{2},
				ToExecutionAddress: []byte{3},
			},
			Signature: []byte{7},
		},
		{
			Message: &ethpb.BLSToExecutionChange{
				ValidatorIndex:     4,
				FromBlsPubkey:      []byte{5},
				ToExecutionAddress: []byte{6},
			},
			Signature: []byte{8},
		},
	}

	expectedResult := []*structs.SignedBLSToExecutionChange{
		{
			Message: &structs.BLSToExecutionChange{
				ValidatorIndex:     "1",
				FromBLSPubkey:      hexutil.Encode([]byte{2}),
				ToExecutionAddress: hexutil.Encode([]byte{3}),
			},
			Signature: hexutil.Encode([]byte{7}),
		},
		{
			Message: &structs.BLSToExecutionChange{
				ValidatorIndex:     "4",
				FromBLSPubkey:      hexutil.Encode([]byte{5}),
				ToExecutionAddress: hexutil.Encode([]byte{6}),
			},
			Signature: hexutil.Encode([]byte{8}),
		},
	}

	assert.DeepEqual(t, expectedResult, structs.SignedBLSChangesFromConsensus(input))
}

func TestBeaconBlockJsonHelpers_JsonifyAttestations(t *testing.T) {
	input := []*ethpb.Attestation{
		{
			AggregationBits: []byte{1},
			Data: &ethpb.AttestationData{
				Slot:            2,
				CommitteeIndex:  3,
				BeaconBlockRoot: []byte{4},
				Source: &ethpb.Checkpoint{
					Epoch: 5,
					Root:  []byte{6},
				},
				Target: &ethpb.Checkpoint{
					Epoch: 7,
					Root:  []byte{8},
				},
			},
			Signature: []byte{9},
		},
		{
			AggregationBits: []byte{10},
			Data: &ethpb.AttestationData{
				Slot:            11,
				CommitteeIndex:  12,
				BeaconBlockRoot: []byte{13},
				Source: &ethpb.Checkpoint{
					Epoch: 14,
					Root:  []byte{15},
				},
				Target: &ethpb.Checkpoint{
					Epoch: 16,
					Root:  []byte{17},
				},
			},
			Signature: []byte{18},
		},
	}

	expectedResult := []*structs.Attestation{
		{
			AggregationBits: hexutil.Encode([]byte{1}),
			Data: &structs.AttestationData{
				Slot:            "2",
				CommitteeIndex:  "3",
				BeaconBlockRoot: hexutil.Encode([]byte{4}),
				Source: &structs.Checkpoint{
					Epoch: "5",
					Root:  hexutil.Encode([]byte{6}),
				},
				Target: &structs.Checkpoint{
					Epoch: "7",
					Root:  hexutil.Encode([]byte{8}),
				},
			},
			Signature: hexutil.Encode([]byte{9}),
		},
		{
			AggregationBits: hexutil.Encode([]byte{10}),
			Data: &structs.AttestationData{
				Slot:            "11",
				CommitteeIndex:  "12",
				BeaconBlockRoot: hexutil.Encode([]byte{13}),
				Source: &structs.Checkpoint{
					Epoch: "14",
					Root:  hexutil.Encode([]byte{15}),
				},
				Target: &structs.Checkpoint{
					Epoch: "16",
					Root:  hexutil.Encode([]byte{17}),
				},
			},
			Signature: hexutil.Encode([]byte{18}),
		},
	}

	result := jsonifyAttestations(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestBeaconBlockJsonHelpers_JsonifySignedVoluntaryExits(t *testing.T) {
	input := []*ethpb.SignedVoluntaryExit{
		{
			Exit: &ethpb.VoluntaryExit{
				Epoch:          1,
				ValidatorIndex: 2,
			},
			Signature: []byte{3},
		},
		{
			Exit: &ethpb.VoluntaryExit{
				Epoch:          4,
				ValidatorIndex: 5,
			},
			Signature: []byte{6},
		},
	}

	expectedResult := []*structs.SignedVoluntaryExit{
		{
			Message: &structs.VoluntaryExit{
				Epoch:          "1",
				ValidatorIndex: "2",
			},
			Signature: hexutil.Encode([]byte{3}),
		},
		{
			Message: &structs.VoluntaryExit{
				Epoch:          "4",
				ValidatorIndex: "5",
			},
			Signature: hexutil.Encode([]byte{6}),
		},
	}

	result := JsonifySignedVoluntaryExits(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestBeaconBlockJsonHelpers_JsonifyAttestationData(t *testing.T) {
	input := &ethpb.AttestationData{
		Slot:            1,
		CommitteeIndex:  2,
		BeaconBlockRoot: []byte{3},
		Source: &ethpb.Checkpoint{
			Epoch: 4,
			Root:  []byte{5},
		},
		Target: &ethpb.Checkpoint{
			Epoch: 6,
			Root:  []byte{7},
		},
	}

	expectedResult := &structs.AttestationData{
		Slot:            "1",
		CommitteeIndex:  "2",
		BeaconBlockRoot: hexutil.Encode([]byte{3}),
		Source: &structs.Checkpoint{
			Epoch: "4",
			Root:  hexutil.Encode([]byte{5}),
		},
		Target: &structs.Checkpoint{
			Epoch: "6",
			Root:  hexutil.Encode([]byte{7}),
		},
	}

	result := jsonifyAttestationData(input)
	assert.DeepEqual(t, expectedResult, result)
}
