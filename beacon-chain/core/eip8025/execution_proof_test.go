package eip8025

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestVerifyExecutionProofEnvelope(t *testing.T) {
	st, keys := util.DeterministicGenesisState(t, 8)

	envelope := func(t *testing.T, proofData []byte, signingKey int) blocks.ROSignedExecutionProofEnvelope {
		msg := &ethpb.ExecutionProofEnvelope{
			ProofData:       proofData,
			ProofType:       []byte{byte(ethpb.ProofTypeEthrexSP1)},
			BeaconBlockRoot: make([]byte, 32),
		}
		sig, err := signing.ComputeDomainAndSign(st, 0, msg, params.BeaconConfig().DomainExecutionProof, keys[signingKey])
		require.NoError(t, err)
		proof, err := blocks.NewROSignedExecutionProofEnvelope(&ethpb.SignedExecutionProofEnvelope{Message: msg, Signature: sig})
		require.NoError(t, err)
		return proof
	}

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, VerifyExecutionProofEnvelope(st, 0, envelope(t, []byte{0x01}, 0)))
	})

	t.Run("signed by another validator", func(t *testing.T) {
		require.ErrorIs(t, VerifyExecutionProofEnvelope(st, 0, envelope(t, []byte{0x01}, 1)), errProofSignatureInvalid)
	})
}
