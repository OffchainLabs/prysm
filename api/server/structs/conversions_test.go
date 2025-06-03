package structs

import (
	"testing"

	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestDepositSnapshotFromConsensus(t *testing.T) {
	ds := &eth.DepositSnapshot{
		Finalized:      [][]byte{{0xde, 0xad, 0xbe, 0xef}, {0xca, 0xfe, 0xba, 0xbe}},
		DepositRoot:    []byte{0xab, 0xcd},
		DepositCount:   12345,
		ExecutionHash:  []byte{0x12, 0x34},
		ExecutionDepth: 67890,
	}

	res := DepositSnapshotFromConsensus(ds)
	require.NotNil(t, res)
	require.DeepEqual(t, []string{"0xdeadbeef", "0xcafebabe"}, res.Finalized)
	require.Equal(t, "0xabcd", res.DepositRoot)
	require.Equal(t, "12345", res.DepositCount)
	require.Equal(t, "0x1234", res.ExecutionBlockHash)
	require.Equal(t, "67890", res.ExecutionBlockHeight)
}

func TestSignedBLSToExecutionChange_ToConsensus(t *testing.T) {
	s := &SignedBLSToExecutionChange{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedValidatorRegistration_ToConsensus(t *testing.T) {
	s := &SignedValidatorRegistration{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedContributionAndProof_ToConsensus(t *testing.T) {
	s := &SignedContributionAndProof{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestContributionAndProof_ToConsensus(t *testing.T) {
	c := &ContributionAndProof{
		Contribution:    nil,
		AggregatorIndex: "invalid",
		SelectionProof:  "",
	}
	_, err := c.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedAggregateAttestationAndProof_ToConsensus(t *testing.T) {
	s := &SignedAggregateAttestationAndProof{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAggregateAttestationAndProof_ToConsensus(t *testing.T) {
	a := &AggregateAttestationAndProof{
		AggregatorIndex: "1",
		Aggregate:       nil,
		SelectionProof:  "",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAttestation_ToConsensus(t *testing.T) {
	a := &Attestation{
		AggregationBits: "0x10",
		Data:            nil,
		Signature:       "",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSingleAttestation_ToConsensus(t *testing.T) {
	s := &SingleAttestation{
		CommitteeIndex: "1",
		AttesterIndex:  "1",
		Data:           nil,
		Signature:      "",
	}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedVoluntaryExit_ToConsensus(t *testing.T) {
	s := &SignedVoluntaryExit{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestProposerSlashing_ToConsensus(t *testing.T) {
	p := &ProposerSlashing{SignedHeader1: nil, SignedHeader2: nil}
	_, err := p.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestProposerSlashing_FromConsensus(t *testing.T) {
	input := []*eth.ProposerSlashing{
		{
			Header_1: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          1,
					ProposerIndex: 2,
					ParentRoot:    []byte{3},
					StateRoot:     []byte{4},
					BodyRoot:      []byte{5},
				},
				Signature: []byte{6},
			},
			Header_2: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          7,
					ProposerIndex: 8,
					ParentRoot:    []byte{9},
					StateRoot:     []byte{10},
					BodyRoot:      []byte{11},
				},
				Signature: []byte{12},
			},
		},
		{
			Header_1: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          13,
					ProposerIndex: 14,
					ParentRoot:    []byte{15},
					StateRoot:     []byte{16},
					BodyRoot:      []byte{17},
				},
				Signature: []byte{18},
			},
			Header_2: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          19,
					ProposerIndex: 20,
					ParentRoot:    []byte{21},
					StateRoot:     []byte{22},
					BodyRoot:      []byte{23},
				},
				Signature: []byte{24},
			},
		},
	}

	expectedResult := []*ProposerSlashing{
		{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "1",
					ProposerIndex: "2",
					ParentRoot:    hexutil.Encode([]byte{3}),
					StateRoot:     hexutil.Encode([]byte{4}),
					BodyRoot:      hexutil.Encode([]byte{5}),
				},
				Signature: hexutil.Encode([]byte{6}),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "7",
					ProposerIndex: "8",
					ParentRoot:    hexutil.Encode([]byte{9}),
					StateRoot:     hexutil.Encode([]byte{10}),
					BodyRoot:      hexutil.Encode([]byte{11}),
				},
				Signature: hexutil.Encode([]byte{12}),
			},
		},
		{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "13",
					ProposerIndex: "14",
					ParentRoot:    hexutil.Encode([]byte{15}),
					StateRoot:     hexutil.Encode([]byte{16}),
					BodyRoot:      hexutil.Encode([]byte{17}),
				},
				Signature: hexutil.Encode([]byte{18}),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "19",
					ProposerIndex: "20",
					ParentRoot:    hexutil.Encode([]byte{21}),
					StateRoot:     hexutil.Encode([]byte{22}),
					BodyRoot:      hexutil.Encode([]byte{23}),
				},
				Signature: hexutil.Encode([]byte{24}),
			},
		},
	}

	result := ProposerSlashingsFromConsensus(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestAttesterSlashing_ToConsensus(t *testing.T) {
	a := &AttesterSlashing{Attestation1: nil, Attestation2: nil}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAttesterSlashing_FromConsensus(t *testing.T) {
	input := []*eth.AttesterSlashing{
		{
			Attestation_1: &eth.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &eth.AttestationData{
					Slot:            3,
					CommitteeIndex:  4,
					BeaconBlockRoot: []byte{5},
					Source: &eth.Checkpoint{
						Epoch: 6,
						Root:  []byte{7},
					},
					Target: &eth.Checkpoint{
						Epoch: 8,
						Root:  []byte{9},
					},
				},
				Signature: []byte{10},
			},
			Attestation_2: &eth.IndexedAttestation{
				AttestingIndices: []uint64{11, 12},
				Data: &eth.AttestationData{
					Slot:            13,
					CommitteeIndex:  14,
					BeaconBlockRoot: []byte{15},
					Source: &eth.Checkpoint{
						Epoch: 16,
						Root:  []byte{17},
					},
					Target: &eth.Checkpoint{
						Epoch: 18,
						Root:  []byte{19},
					},
				},
				Signature: []byte{20},
			},
		},
		{
			Attestation_1: &eth.IndexedAttestation{
				AttestingIndices: []uint64{21, 22},
				Data: &eth.AttestationData{
					Slot:            23,
					CommitteeIndex:  24,
					BeaconBlockRoot: []byte{25},
					Source: &eth.Checkpoint{
						Epoch: 26,
						Root:  []byte{27},
					},
					Target: &eth.Checkpoint{
						Epoch: 28,
						Root:  []byte{29},
					},
				},
				Signature: []byte{30},
			},
			Attestation_2: &eth.IndexedAttestation{
				AttestingIndices: []uint64{31, 32},
				Data: &eth.AttestationData{
					Slot:            33,
					CommitteeIndex:  34,
					BeaconBlockRoot: []byte{35},
					Source: &eth.Checkpoint{
						Epoch: 36,
						Root:  []byte{37},
					},
					Target: &eth.Checkpoint{
						Epoch: 38,
						Root:  []byte{39},
					},
				},
				Signature: []byte{40},
			},
		},
	}

	expectedResult := []*AttesterSlashing{
		{
			Attestation1: &IndexedAttestation{
				AttestingIndices: []string{"1", "2"},
				Data: &AttestationData{
					Slot:            "3",
					CommitteeIndex:  "4",
					BeaconBlockRoot: hexutil.Encode([]byte{5}),
					Source: &Checkpoint{
						Epoch: "6",
						Root:  hexutil.Encode([]byte{7}),
					},
					Target: &Checkpoint{
						Epoch: "8",
						Root:  hexutil.Encode([]byte{9}),
					},
				},
				Signature: hexutil.Encode([]byte{10}),
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: []string{"11", "12"},
				Data: &AttestationData{
					Slot:            "13",
					CommitteeIndex:  "14",
					BeaconBlockRoot: hexutil.Encode([]byte{15}),
					Source: &Checkpoint{
						Epoch: "16",
						Root:  hexutil.Encode([]byte{17}),
					},
					Target: &Checkpoint{
						Epoch: "18",
						Root:  hexutil.Encode([]byte{19}),
					},
				},
				Signature: hexutil.Encode([]byte{20}),
			},
		},
		{
			Attestation1: &IndexedAttestation{
				AttestingIndices: []string{"21", "22"},
				Data: &AttestationData{
					Slot:            "23",
					CommitteeIndex:  "24",
					BeaconBlockRoot: hexutil.Encode([]byte{25}),
					Source: &Checkpoint{
						Epoch: "26",
						Root:  hexutil.Encode([]byte{27}),
					},
					Target: &Checkpoint{
						Epoch: "28",
						Root:  hexutil.Encode([]byte{29}),
					},
				},
				Signature: hexutil.Encode([]byte{30}),
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: []string{"31", "32"},
				Data: &AttestationData{
					Slot:            "33",
					CommitteeIndex:  "34",
					BeaconBlockRoot: hexutil.Encode([]byte{35}),
					Source: &Checkpoint{
						Epoch: "36",
						Root:  hexutil.Encode([]byte{37}),
					},
					Target: &Checkpoint{
						Epoch: "38",
						Root:  hexutil.Encode([]byte{39}),
					},
				},
				Signature: hexutil.Encode([]byte{40}),
			},
		},
	}

	result := AttesterSlashingsFromConsensus(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestIndexedAttestation_ToConsensus(t *testing.T) {
	a := &IndexedAttestation{
		AttestingIndices: []string{"1"},
		Data:             nil,
		Signature:        "invalid",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}
