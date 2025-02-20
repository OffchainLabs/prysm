package structs

import (
	"testing"

	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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

func TestAttesterSlashing_ToConsensus(t *testing.T) {
	a := &AttesterSlashing{Attestation1: nil, Attestation2: nil}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
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
