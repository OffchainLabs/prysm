package verification

import (
	"bytes"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestProposerPreferencesVerifier_VerifyCurrentOrNextEpoch(t *testing.T) {
	// Next epoch future slot is accepted.
	st, _, signed := newSignedProposerPreferencesState(t, 31, 40, 0)
	verifier := &ProposerPreferencesVerifier{sharedResources: &sharedResources{clock: testClockAtSlotForProposerPreferences(t, st.Slot())}, results: newResults(RequireProposerPreferencesCurrentOrNextEpoch), p: signed}
	require.NoError(t, verifier.VerifyCurrentOrNextEpoch(st))

	// Current epoch future slot is accepted.
	signed.Message.ProposalSlot = st.Slot() + 1
	verifier = &ProposerPreferencesVerifier{sharedResources: &sharedResources{clock: testClockAtSlotForProposerPreferences(t, st.Slot())}, results: newResults(RequireProposerPreferencesCurrentOrNextEpoch), p: signed}
	require.NoError(t, verifier.VerifyCurrentOrNextEpoch(st))

	// Current slot (already passed) is rejected.
	signed.Message.ProposalSlot = st.Slot()
	verifier = &ProposerPreferencesVerifier{sharedResources: &sharedResources{clock: testClockAtSlotForProposerPreferences(t, st.Slot())}, results: newResults(RequireProposerPreferencesCurrentOrNextEpoch), p: signed}
	require.ErrorIs(t, verifier.VerifyCurrentOrNextEpoch(st), ErrProposerPreferencesSlotAlreadyPassed)

	// Same-epoch future slot with more room.
	st2, _, signed2 := newSignedProposerPreferencesState(t, 24, 28, 0)
	verifier = &ProposerPreferencesVerifier{sharedResources: &sharedResources{clock: testClockAtSlotForProposerPreferences(t, st2.Slot())}, results: newResults(RequireProposerPreferencesCurrentOrNextEpoch), p: signed2}
	require.NoError(t, verifier.VerifyCurrentOrNextEpoch(st2))
}

func TestProposerPreferencesVerifier_VerifyCurrentOrNextEpoch_UsesClockWhenStateLags(t *testing.T) {
	st, _, signed := newSignedProposerPreferencesState(t, 31, 32, 0)
	verifier := &ProposerPreferencesVerifier{
		sharedResources: &sharedResources{clock: testClockAtSlotForProposerPreferences(t, 32)},
		results:         newResults(RequireProposerPreferencesCurrentOrNextEpoch),
		p:               signed,
	}
	require.ErrorIs(t, verifier.VerifyCurrentOrNextEpoch(st), ErrProposerPreferencesSlotAlreadyPassed)
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

	// Signature from the wrong key must fail.
	badSig := signProposerPreferencesWithConfigFork(t, keys[6], signed.Message, st)
	signed.Signature = badSig
	verifier = &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesSignatureValid), p: signed}
	require.ErrorContains(t, "verify signature", verifier.VerifySignature(st))
}

func TestProposerPreferencesVerifier_VerifySignature_ForkBoundary(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	validatorIndex := primitives.ValidatorIndex(5)
	proposalSlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch) + 8 // epoch 1

	st, keys := util.DeterministicGenesisStateFulu(t, 64)
	// State is at epoch 0 (pre-gloas), but proposal is for epoch 1 (gloas).
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	require.NoError(t, st.SetFork(&ethpb.Fork{
		PreviousVersion: cfg.FuluForkVersion,
		CurrentVersion:  cfg.FuluForkVersion,
		Epoch:           0,
	}))

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
	// Sign using config fork (like the DomainData RPC does).
	signed.Signature = signProposerPreferencesWithConfigFork(t, keys[validatorIndex], signed.Message, st)

	verifier := &ProposerPreferencesVerifier{results: newResults(RequireProposerPreferencesSignatureValid), p: signed}
	require.NoError(t, verifier.VerifySignature(st))
}

func newSignedProposerPreferencesState(t *testing.T, currentSlot, proposalSlot primitives.Slot, validatorIndex primitives.ValidatorIndex) (state.BeaconState, []bls.SecretKey, *ethpb.SignedProposerPreferences) {
	t.Helper()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	st, keys := util.DeterministicGenesisStateFulu(t, 64)
	require.NoError(t, st.SetSlot(currentSlot))
	require.NoError(t, st.SetFork(&ethpb.Fork{
		PreviousVersion: cfg.FuluForkVersion,
		CurrentVersion:  cfg.GloasForkVersion,
		Epoch:           0,
	}))

	lookaheadSize := int(uint64(params.BeaconConfig().MinSeedLookahead+1) * uint64(params.BeaconConfig().SlotsPerEpoch))
	lookahead := make([]primitives.ValidatorIndex, lookaheadSize)
	currentEpoch := slots.ToEpoch(currentSlot)
	proposalEpoch := slots.ToEpoch(proposalSlot)
	index := primitives.Slot(proposalEpoch-currentEpoch)*params.BeaconConfig().SlotsPerEpoch + (proposalSlot % params.BeaconConfig().SlotsPerEpoch)
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
	signed.Signature = signProposerPreferencesWithConfigFork(t, keys[validatorIndex], signed.Message, st)
	return st, keys, signed
}

// signProposerPreferencesWithConfigFork signs preferences using the config-based fork
// for the target epoch, matching the DomainData RPC behavior used by the validator client.
func signProposerPreferencesWithConfigFork(t *testing.T, sk bls.SecretKey, preferences *ethpb.ProposerPreferences, st state.ReadOnlyBeaconState) []byte {
	t.Helper()

	epoch := slots.ToEpoch(preferences.ProposalSlot)
	fork, err := params.Fork(epoch)
	require.NoError(t, err)
	sig, err := signing.ComputeDomainAndSignWithoutState(fork, epoch, params.BeaconConfig().DomainProposerPreferences, st.GenesisValidatorsRoot(), preferences, sk)
	require.NoError(t, err)
	return sig
}

func testClockAtSlotForProposerPreferences(t *testing.T, slot primitives.Slot) *startup.Clock {
	t.Helper()

	genesis := time.Unix(1_700_000_000, 0)
	now, err := slots.StartTime(genesis, slot)
	require.NoError(t, err)
	return startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))
}
