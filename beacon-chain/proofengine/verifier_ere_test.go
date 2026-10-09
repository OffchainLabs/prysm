//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && arm64))

package proofengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// fixtureProofs are genuine proofs of the fixture payload: the stand-in proofs of mock mode, from
// github.com/eth-act/zkboost v0.11.1, crates/server/src/proof/zkvm/mock.
var fixtureProofs = standInProofNames

func readFixtureProof(t *testing.T, proofType ethpb.ProofType) []byte {
	proofs, err := standInProofs()
	require.NoError(t, err)
	return proofs[proofType]
}

func TestEreVerifier_Verify(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	publicInput := PublicInput(fixtureNewPayloadRequestRoot)
	keys := defaultVerificationKeys()

	for proofType := range fixtureProofs {
		kind, err := zkVMKindOf(proofType)
		require.NoError(t, err)

		verifier, err := newZkVMVerifier(kind, keys[proofType])
		require.NoError(t, err)

		t.Run(proofType.String()+" valid", func(t *testing.T) {
			publicValues, err := verifier.verify(readFixtureProof(t, proofType))
			require.NoError(t, err)
			require.Equal(t, true, publicValuesMatch(kind, publicValues, publicInput))
		})

		t.Run(proofType.String()+" garbage", func(t *testing.T) {
			_, err := verifier.verify([]byte{0x01, 0x02, 0x03})
			require.ErrorIs(t, err, ErrProofInvalid)
		})

		t.Run(proofType.String()+" tampered", func(t *testing.T) {
			proof := readFixtureProof(t, proofType)
			proof[len(proof)/2] ^= 0xff
			_, err := verifier.verify(proof)
			require.ErrorIs(t, err, ErrProofInvalid)
		})
	}

	t.Run("proof of another guest program", func(t *testing.T) {
		verifier, err := newZkVMVerifier(zkVMSP1, keys[ethpb.ProofTypeEthrexSP1])
		require.NoError(t, err)

		// A genuine SP1 proof of the Reth guest, from the same zkboost fixtures.
		proof, err := os.ReadFile(filepath.Join("testdata", "stateless-validator-reth-sp1-v6.4.0.proof"))
		require.NoError(t, err)

		_, err = verifier.verify(proof)
		require.ErrorIs(t, err, ErrProofInvalid)
	})
}

func TestNew(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	ctx := context.Background()
	publicInput := PublicInput(fixtureNewPayloadRequestRoot)

	t.Run("built-in keys", func(t *testing.T) {
		engine, err := New("", false)
		require.NoError(t, err)
		require.Equal(t, len(proofTypes()), len(engine.verifiers))

		for proofType := range fixtureProofs {
			require.NoError(t, engine.Verify(ctx, proofType, readFixtureProof(t, proofType), publicInput))
		}
	})

	t.Run("proof of another payload", func(t *testing.T) {
		engine, err := New("", false)
		require.NoError(t, err)

		otherPublicInput := PublicInput([32]byte{0x01})
		for proofType := range fixtureProofs {
			require.ErrorIs(t, engine.Verify(ctx, proofType, readFixtureProof(t, proofType), otherPublicInput), ErrProofInvalid)
		}
	})

	t.Run("configuration file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "proof-engine.json")
		content := `{"execution_proofs":[{"proof_type":2,"program_vk":"` + defaultVerificationKeysHex[ethpb.ProofTypeEthrexSP1] + `"}]}`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		engine, err := New(path, false)
		require.NoError(t, err)
		require.Equal(t, 1, len(engine.verifiers))
		require.NoError(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, readFixtureProof(t, ethpb.ProofTypeEthrexSP1), publicInput))
	})

	t.Run("bad verification key", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "proof-engine.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"execution_proofs":[{"proof_type":1,"program_vk":"0x01"}]}`), 0o600))

		_, err := New(path, false)
		require.ErrorContains(t, "new openvm verifier", err)
	})

	t.Run("mock proofs", func(t *testing.T) {
		engine, err := New("", true)
		require.NoError(t, err)

		for proofType := range fixtureProofs {
			require.NoError(t, engine.Verify(ctx, proofType, mockProof(publicInput), publicInput))
			require.ErrorIs(t, engine.Verify(ctx, proofType, mockProof(PublicInput([32]byte{0x01})), publicInput), ErrProofInvalid)

			// Real proofs are still verified.
			require.NoError(t, engine.Verify(ctx, proofType, readFixtureProof(t, proofType), publicInput))
		}
	})

	t.Run("mock proofs disabled", func(t *testing.T) {
		engine, err := New("", false)
		require.NoError(t, err)

		for proofType := range fixtureProofs {
			require.ErrorIs(t, engine.Verify(ctx, proofType, mockProof(publicInput), publicInput), ErrProofInvalid)
		}
	})

	t.Run("mock proofs with a key the stand-in does not verify with", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "proof-engine.json")
		// The verification key of the Reth SP1 guest of eth-act/ere-guests v0.17.0.
		const rethSP1Key = "0x00a03cbfa95559cfee3b45ef925f3f7a631181e35e774e92b040277d893511dd"
		content := `{"execution_proofs":[{"proof_type":2,"program_vk":"` + rethSP1Key + `"}]}`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		_, err := New(path, false)
		require.NoError(t, err)

		_, err = New(path, true)
		require.ErrorContains(t, "stand-in proof of ethrex-sp1 does not verify", err)
	})
}
