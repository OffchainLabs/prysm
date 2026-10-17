package blocks

import (
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestNewROSignedExecutionProofEnvelope(t *testing.T) {
	envelope := func(proofType ethpb.ProofType, proofData []byte) *ethpb.SignedExecutionProofEnvelope {
		return &ethpb.SignedExecutionProofEnvelope{
			Message: &ethpb.ExecutionProofEnvelope{
				ProofData:       proofData,
				ProofType:       []byte{byte(proofType)},
				BeaconBlockRoot: make([]byte, 32),
			},
			Signature: make([]byte, 96),
		}
	}

	t.Run("valid", func(t *testing.T) {
		proof, err := NewROSignedExecutionProofEnvelope(envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}))
		require.NoError(t, err)
		require.Equal(t, ethpb.ProofTypeEthrexSP1, proof.ProofType())
	})

	t.Run("nil", func(t *testing.T) {
		_, err := NewROSignedExecutionProofEnvelope(nil)
		require.ErrorIs(t, err, errNilExecutionProof)
	})

	t.Run("nil message", func(t *testing.T) {
		_, err := NewROSignedExecutionProofEnvelope(&ethpb.SignedExecutionProofEnvelope{})
		require.ErrorIs(t, err, errNilExecutionProofEnvelope)
	})

	t.Run("empty proof data", func(t *testing.T) {
		_, err := NewROSignedExecutionProofEnvelope(envelope(ethpb.ProofTypeEthrexSP1, nil))
		require.ErrorIs(t, err, errEmptyExecutionProof)
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		// The specification supports proof types 1, 2 and 3 only.
		for _, proofType := range []ethpb.ProofType{0, 4} {
			_, err := NewROSignedExecutionProofEnvelope(envelope(proofType, []byte{0x01}))
			require.ErrorIs(t, err, errUnsupportedExecutionProofType)
		}
	})
}
