package verification

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestExecutionPayloadEnvelope_VerifyBlockRootSeen(t *testing.T) {
	beaconBlockRoot := [32]byte{1, 2, 3}
	verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload:            &enginev1.ExecutionPayloadElectra{},
			BeaconBlockRoot:    beaconBlockRoot[:],
			BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
			StateRoot:          make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})

	t.Run("parentSeen nil", func(t *testing.T) {
		require.ErrorIs(t, verifier.VerifyBlockRootSeen(nil), ErrEnvelopeBlockRootNotSeen)
		require.Equal(t, true, verifier.results.executed(RequireBlockRootSeen))
		require.Equal(t, ErrEnvelopeBlockRootNotSeen, verifier.results.result(RequireBlockRootSeen))
	})

	t.Run("parentSeen true", func(t *testing.T) {
		require.NoError(t, verifier.VerifyBlockRootSeen(badParentCb(t, beaconBlockRoot, true)))
		require.Equal(t, true, verifier.results.executed(RequireBlockRootSeen))
		require.NoError(t, verifier.results.result(RequireBlockRootSeen))
	})

	t.Run("parentSeen false", func(t *testing.T) {
		require.ErrorIs(t, verifier.VerifyBlockRootSeen(badParentCb(t, beaconBlockRoot, false)), ErrEnvelopeBlockRootNotSeen)
		require.Equal(t, true, verifier.results.executed(RequireBlockRootSeen))
		require.Equal(t, ErrEnvelopeBlockRootNotSeen, verifier.results.result(RequireBlockRootSeen))
	})
}

func TestExecutionPayloadEnvelope_VerifyBlockRootValid(t *testing.T) {
	beaconBlockRoot := [32]byte{1, 2, 3}
	verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload:            &enginev1.ExecutionPayloadElectra{},
			BeaconBlockRoot:    beaconBlockRoot[:],
			BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
			StateRoot:          make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})

	t.Run("badBlock true", func(t *testing.T) {
		require.ErrorIs(t, verifier.VerifyBlockRootValid(badParentCb(t, beaconBlockRoot, true)), ErrPayloadAttBlockRootInvalid)
		require.Equal(t, true, verifier.results.executed(RequireBlockRootValid))
		require.Equal(t, ErrPayloadAttBlockRootInvalid, verifier.results.result(RequireBlockRootValid))
	})

	t.Run("badBlock false", func(t *testing.T) {
		require.NoError(t, verifier.VerifyBlockRootValid(badParentCb(t, beaconBlockRoot, false)))
		require.Equal(t, true, verifier.results.executed(RequireBlockRootValid))
		require.NoError(t, verifier.results.result(RequireBlockRootValid))
	})
}

func TestExecutionPayloadEnvelope_VerifyBuilderValid(t *testing.T) {
	builderIndexWanted := primitives.ValidatorIndex(1)
	builderIndexMatch := builderIndexWanted
	builderIndexMismatch := primitives.ValidatorIndex(2)
	verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload:            &enginev1.ExecutionPayloadElectra{},
			BuilderIndex:       builderIndexWanted,
			BeaconBlockRoot:    make([]byte, 32),
			BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
			StateRoot:          make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})

	t.Run("builder index match", func(t *testing.T) {
		header, err := blocks.WrappedROExecutionPayloadHeaderEPBS(&enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              make([]byte, 32),
			BuilderIndex:           builderIndexMatch,
			BlobKzgCommitmentsRoot: make([]byte, 32),
		})
		require.NoError(t, err)
		require.NoError(t, verifier.VerifyBuilderValid(header))
		require.Equal(t, true, verifier.results.executed(RequireBuilderValid))
		require.NoError(t, verifier.results.result(RequireBuilderValid))
	})

	t.Run("builder index mismatch", func(t *testing.T) {
		header, err := blocks.WrappedROExecutionPayloadHeaderEPBS(&enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              make([]byte, 32),
			BuilderIndex:           builderIndexMismatch,
			BlobKzgCommitmentsRoot: make([]byte, 32),
		})
		require.NoError(t, err)
		require.ErrorIs(t, verifier.VerifyBuilderValid(header), ErrIncorrectEnvelopeBuilder)
		require.Equal(t, true, verifier.results.executed(RequireBuilderValid))
		require.Equal(t, ErrIncorrectEnvelopeBuilder, verifier.results.result(RequireBuilderValid))
	})
}

func TestExecutionPayloadEnvelope_VerifyPayloadHash(t *testing.T) {
	blockHashWanted := [32]byte{1, 2, 3}
	blockHashMatch := blockHashWanted
	blockHashMismatch := [32]byte{4, 5, 6}
	verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadElectra{
				BlockHash: blockHashWanted[:],
			},
			BeaconBlockRoot:    make([]byte, 32),
			BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
			StateRoot:          make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})

	t.Run("payload hash match", func(t *testing.T) {
		header, err := blocks.WrappedROExecutionPayloadHeaderEPBS(&enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              blockHashMatch[:],
			BlobKzgCommitmentsRoot: make([]byte, 32),
		})
		require.NoError(t, err)
		require.NoError(t, verifier.VerifyPayloadHash(header))
		require.Equal(t, true, verifier.results.executed(RequirePayloadHashValid))
		require.NoError(t, verifier.results.result(RequirePayloadHashValid))
	})

	t.Run("payload hash mismatch", func(t *testing.T) {
		header, err := blocks.WrappedROExecutionPayloadHeaderEPBS(&enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              blockHashMismatch[:],
			BlobKzgCommitmentsRoot: make([]byte, 32),
		})
		require.NoError(t, err)
		require.ErrorIs(t, verifier.VerifyPayloadHash(header), ErrIncorrectEnvelopeBlockHash)
		require.Equal(t, true, verifier.results.executed(RequirePayloadHashValid))
		require.Equal(t, ErrIncorrectEnvelopeBlockHash, verifier.results.result(RequirePayloadHashValid))
	})
}

func TestExecutionPayloadEnvelope_VerifySignature(t *testing.T) {
	_, secretKeys, err := util.DeterministicDepositsAndKeys(2)
	require.NoError(t, err)
	st, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators: []*ethpb.Validator{
			{PublicKey: secretKeys[0].PublicKey().Marshal()},
			{PublicKey: secretKeys[1].PublicKey().Marshal()},
		},
		Fork: &ethpb.Fork{
			CurrentVersion:  params.BeaconConfig().EPBSForkVersion,
			PreviousVersion: params.BeaconConfig().ElectraForkVersion,
		},
	})
	require.NoError(t, err)
	epoch := primitives.Epoch(1)

	builderIndexWanted := primitives.ValidatorIndex(0)
	builderIndexMatch := builderIndexWanted
	builderIndexMismatch := primitives.ValidatorIndex(1)
	envelope := &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadElectra{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
		},
		BuilderIndex:       builderIndexWanted,
		BeaconBlockRoot:    make([]byte, 32),
		BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
		StateRoot:          make([]byte, 32),
	}

	t.Run("signature valid", func(t *testing.T) {
		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			epoch,
			envelope,
			params.BeaconConfig().DomainBeaconBuilder,
			secretKeys[builderIndexMatch],
		)
		require.NoError(t, err)
		signature, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)

		verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
			Message:   envelope,
			Signature: signature.Marshal(),
		})
		require.NoError(t, verifier.VerifySignature(st))
		require.Equal(t, true, verifier.results.executed(RequireSignatureValid))
		require.NoError(t, verifier.results.result(RequireSignatureValid))
	})

	t.Run("signature invalid", func(t *testing.T) {
		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			epoch,
			envelope,
			params.BeaconConfig().DomainBeaconBuilder,
			secretKeys[builderIndexMismatch],
		)
		require.NoError(t, err)
		signature, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)

		verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
			Message:   envelope,
			Signature: signature.Marshal(),
		})
		require.ErrorIs(t, verifier.VerifySignature(st), signing.ErrSigFailedToVerify)
		require.Equal(t, true, verifier.results.executed(RequireSignatureValid))
		require.Equal(t, signing.ErrSigFailedToVerify, verifier.results.result(RequireSignatureValid))
	})
}

func TestExecutionPayloadEnvelope_SatisfyRequirement(t *testing.T) {
	t.Run("requirements satisfy", func(t *testing.T) {
		verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
			Message: &enginev1.ExecutionPayloadEnvelope{
				Payload:            &enginev1.ExecutionPayloadElectra{},
				BeaconBlockRoot:    make([]byte, 32),
				BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
				StateRoot:          make([]byte, 32),
			},
			Signature: make([]byte, 96),
		})

		for _, requirement := range GossipExecutionPayloadEnvelopeRequirements {
			verifier.SatisfyRequirement(requirement)
		}
		require.Equal(t, true, verifier.results.allSatisfied())
	})

	t.Run("requirements dissatisfy", func(t *testing.T) {
		verifier := newExecutionPayloadEnvelopeVerifier(t, &enginev1.SignedExecutionPayloadEnvelope{
			Message: &enginev1.ExecutionPayloadEnvelope{
				Payload:            &enginev1.ExecutionPayloadElectra{},
				BeaconBlockRoot:    make([]byte, 32),
				BlobKzgCommitments: [][]byte{make([]byte, 48), make([]byte, 48), make([]byte, 48)},
				StateRoot:          make([]byte, 32),
			},
			Signature: make([]byte, 96),
		})

		for _, requirement := range GossipExecutionPayloadEnvelopeRequirements[:len(GossipExecutionPayloadEnvelopeRequirements)-1] {
			verifier.SatisfyRequirement(requirement)
		}
		require.Equal(t, false, verifier.results.allSatisfied())
	})
}

func newExecutionPayloadEnvelopeVerifier(t *testing.T, s *enginev1.SignedExecutionPayloadEnvelope) *EnvelopeVerifier {
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(s)
	require.NoError(t, err)

	return &EnvelopeVerifier{
		results: newResults(GossipExecutionPayloadEnvelopeRequirements...),
		e:       e,
	}
}
