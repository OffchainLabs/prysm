package beacon_api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestBeaconBlockJsonHelpers_JsonifyTransactions(t *testing.T) {
	input := [][]byte{{1}, {2}, {3}, {4}}

	expectedResult := []string{
		hexutil.Encode([]byte{1}),
		hexutil.Encode([]byte{2}),
		hexutil.Encode([]byte{3}),
		hexutil.Encode([]byte{4}),
	}

	result := jsonifyTransactions(input)
	assert.DeepEqual(t, expectedResult, result)
}

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

func TestBeaconBlockJsonHelpers_JsonifyEth1Data(t *testing.T) {
	input := &ethpb.Eth1Data{
		DepositRoot:  []byte{1},
		DepositCount: 2,
		BlockHash:    []byte{3},
	}

	expectedResult := &structs.Eth1Data{
		DepositRoot:  hexutil.Encode([]byte{1}),
		DepositCount: "2",
		BlockHash:    hexutil.Encode([]byte{3}),
	}

	result := jsonifyEth1Data(input)
	assert.DeepEqual(t, expectedResult, result)
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

func TestBeaconBlockJsonHelpers_JsonifySignedBeaconBlockHeader(t *testing.T) {
	input := &ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:          1,
			ProposerIndex: 2,
			ParentRoot:    []byte{3},
			StateRoot:     []byte{4},
			BodyRoot:      []byte{5},
		},
		Signature: []byte{6},
	}

	expectedResult := &structs.SignedBeaconBlockHeader{
		Message: &structs.BeaconBlockHeader{
			Slot:          "1",
			ProposerIndex: "2",
			ParentRoot:    hexutil.Encode([]byte{3}),
			StateRoot:     hexutil.Encode([]byte{4}),
			BodyRoot:      hexutil.Encode([]byte{5}),
		},
		Signature: hexutil.Encode([]byte{6}),
	}

	result := jsonifySignedBeaconBlockHeader(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestBeaconBlockJsonHelpers_JsonifyIndexedAttestation(t *testing.T) {
	input := &ethpb.IndexedAttestation{
		AttestingIndices: []uint64{1, 2},
		Data: &ethpb.AttestationData{
			Slot:            3,
			CommitteeIndex:  4,
			BeaconBlockRoot: []byte{5},
			Source: &ethpb.Checkpoint{
				Epoch: 6,
				Root:  []byte{7},
			},
			Target: &ethpb.Checkpoint{
				Epoch: 8,
				Root:  []byte{9},
			},
		},
		Signature: []byte{10},
	}

	expectedResult := &structs.IndexedAttestation{
		AttestingIndices: []string{"1", "2"},
		Data: &structs.AttestationData{
			Slot:            "3",
			CommitteeIndex:  "4",
			BeaconBlockRoot: hexutil.Encode([]byte{5}),
			Source: &structs.Checkpoint{
				Epoch: "6",
				Root:  hexutil.Encode([]byte{7}),
			},
			Target: &structs.Checkpoint{
				Epoch: "8",
				Root:  hexutil.Encode([]byte{9}),
			},
		},
		Signature: hexutil.Encode([]byte{10}),
	}

	result := jsonifyIndexedAttestation(input)
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

func TestBeaconBlockJsonHelpers_JsonifyWithdrawals(t *testing.T) {
	input := []*enginev1.Withdrawal{
		{
			Index:          1,
			ValidatorIndex: 2,
			Address:        []byte{3},
			Amount:         4,
		},
		{
			Index:          5,
			ValidatorIndex: 6,
			Address:        []byte{7},
			Amount:         8,
		},
	}

	expectedResult := []*structs.Withdrawal{
		{
			WithdrawalIndex:  "1",
			ValidatorIndex:   "2",
			ExecutionAddress: hexutil.Encode([]byte{3}),
			Amount:           "4",
		},
		{
			WithdrawalIndex:  "5",
			ValidatorIndex:   "6",
			ExecutionAddress: hexutil.Encode([]byte{7}),
			Amount:           "8",
		},
	}

	result := jsonifyWithdrawals(input)
	assert.DeepEqual(t, expectedResult, result)
}
