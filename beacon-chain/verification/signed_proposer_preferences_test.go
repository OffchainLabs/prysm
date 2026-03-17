package verification

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestProposerPreferencesVerifier_VerifyNextEpoch(t *testing.T) {
	st, _, signed := newSignedProposerPreferencesState(t, 31, 40, 0)

	verifier := &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesNextEpoch), p: signed}
	require.NoError(t, verifier.VerifyNextEpoch(st))

	signed.Message.ProposalSlot = st.Slot()
	verifier = &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesNextEpoch), p: signed}
	require.ErrorIs(t, verifier.VerifyNextEpoch(st), ErrProposerPreferencesNotNextEpoch)
}

func TestProposerPreferencesVerifier_VerifyValidProposalSlot(t *testing.T) {
	st, _, signed := newSignedProposerPreferencesState(t, 31, 40, 3)

	verifier := &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesProposalSlotValid), p: signed}
	require.NoError(t, verifier.VerifyValidProposalSlot(st))

	signed.Message.ValidatorIndex = 4
	verifier = &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesProposalSlotValid), p: signed}
	require.ErrorIs(t, verifier.VerifyValidProposalSlot(st), ErrProposerPreferencesInvalidProposalSlot)
}

func TestProposerPreferencesVerifier_VerifySignature(t *testing.T) {
	st, keys, signed := newSignedProposerPreferencesState(t, 31, 40, 5)

	verifier := &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesSignatureValid), p: signed}
	require.NoError(t, verifier.VerifySignature(st))

	badSig := signProposerPreferencesForState(t, keys[6], signed.Message, st)
	signed.Signature = badSig
	verifier = &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesSignatureValid), p: signed}
	require.ErrorContains(t, "verify signature", verifier.VerifySignature(st))
}

func newSignedProposerPreferencesState(t *testing.T, currentSlot, proposalSlot primitives.Slot, validatorIndex primitives.ValidatorIndex) (state.BeaconState, []bls.SecretKey, *ethpb.SignedProposerPreferences) {
	t.Helper()

	st, keys := util.DeterministicGenesisStateFulu(t, 64)
	require.NoError(t, st.SetSlot(currentSlot))

	lookaheadSize := int(uint64(params.BeaconConfig().MinSeedLookahead+1) * uint64(params.BeaconConfig().SlotsPerEpoch))
	lookahead := make([]primitives.ValidatorIndex, lookaheadSize)
	index := params.BeaconConfig().SlotsPerEpoch + (proposalSlot % params.BeaconConfig().SlotsPerEpoch)
	lookahead[index] = validatorIndex
	require.NoError(t, st.SetProposerLookahead(lookahead))

	signed := &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			ProposalSlot:   proposalSlot,
			ValidatorIndex: validatorIndex,
			FeeRecipient:   bytes.Repeat([]byte{0x01}, 20),
			GasLimit:       30_000_000,
		},
	}
	signed.Signature = signProposerPreferencesForState(t, keys[validatorIndex], signed.Message, st)
	return st, keys, signed
}

func signProposerPreferencesForState(t *testing.T, sk bls.SecretKey, preferences *ethpb.ProposerPreferences, st state.ReadOnlyBeaconState) []byte {
	t.Helper()

	epoch := slots.ToEpoch(preferences.ProposalSlot)
	domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainBeaconProposer, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(preferences, domain)
	require.NoError(t, err)
	return sk.Sign(root[:]).Marshal()
}
