package verification

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// SignedProposerPreferencesGossipRequirements is the requirement list for gossip
// signed proposer preferences.
var SignedProposerPreferencesGossipRequirements = requirementList([]Requirement{
	RequireProposerPreferencesNextEpoch,
	RequireProposerPreferencesProposalSlotValid,
	RequireProposerPreferencesSignatureValid,
})

var (
	ErrProposerPreferencesNotNextEpoch        = errors.New("proposer preferences proposal slot is not in the next epoch")
	ErrProposerPreferencesInvalidProposalSlot = errors.New("proposer preferences validator is not assigned to the proposal slot")
)

var _ SignedProposerPreferencesVerifier = &ProposerPreferencesVerifier{}

// ProposerPreferencesVerifier is a read-only verifier for signed proposer preferences.
type ProposerPreferencesVerifier struct {
	*sharedResources
	results *results
	p       *ethpb.SignedProposerPreferences
}

// VerifyNextEpoch verifies the proposal slot is in the next epoch.
func (v *ProposerPreferencesVerifier) VerifyNextEpoch(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireProposerPreferencesNextEpoch, &err)

	msg := v.message()
	currentEpoch := v.clock.CurrentEpoch()
	if slots.ToEpoch(msg.ProposalSlot) != currentEpoch+1 {
		return fmt.Errorf("%w: got %d want %d", ErrProposerPreferencesNotNextEpoch, slots.ToEpoch(msg.ProposalSlot), currentEpoch+1)
	}
	return nil
}

// VerifyValidProposalSlot verifies the validator matches the proposer
// lookahead entry for the proposal slot. The lookahead covers two epochs
// relative to the state: indices [0, SlotsPerEpoch) for the state's epoch
// and [SlotsPerEpoch, 2*SlotsPerEpoch) for the next epoch.
//
// The caller must ensure that the state's epoch is either equal to or one
// less than the proposal slot's epoch. If the state is further behind, it
// should be advanced before calling this method.
func (v *ProposerPreferencesVerifier) VerifyValidProposalSlot(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireProposerPreferencesProposalSlotValid, &err)

	msg := v.message()
	lookahead, err := st.ProposerLookahead()
	if err != nil {
		return errors.Wrap(err, "failed to get proposer lookahead")
	}

	spe := params.BeaconConfig().SlotsPerEpoch
	stateEpoch := slots.ToEpoch(st.Slot())
	proposalEpoch := slots.ToEpoch(msg.ProposalSlot)

	var slotIndex primitives.Slot
	switch {
	case proposalEpoch == stateEpoch:
		slotIndex = msg.ProposalSlot % spe
	case proposalEpoch == stateEpoch+1:
		slotIndex = spe + (msg.ProposalSlot % spe)
	default:
		return fmt.Errorf("%w: proposal epoch %d out of range for state epoch %d",
			ErrProposerPreferencesInvalidProposalSlot, proposalEpoch, stateEpoch)
	}

	if uint64(len(lookahead)) <= uint64(slotIndex) {
		return fmt.Errorf("%w: proposer lookahead index %d out of bounds", ErrProposerPreferencesInvalidProposalSlot, slotIndex)
	}
	if lookahead[slotIndex] != msg.ValidatorIndex {
		return fmt.Errorf("%w: slot=%d got=%d want=%d", ErrProposerPreferencesInvalidProposalSlot, msg.ProposalSlot, msg.ValidatorIndex, lookahead[slotIndex])
	}
	return nil
}

// VerifySignature verifies the signed proposer preferences signature against the validator public key.
func (v *ProposerPreferencesVerifier) VerifySignature(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireProposerPreferencesSignatureValid, &err)

	msg := v.message()
	epoch := slots.ToEpoch(msg.ProposalSlot)
	fork, err := params.Fork(epoch)
	if err != nil {
		return errors.Wrap(err, "fork")
	}
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainProposerPreferences, st.GenesisValidatorsRoot())
	if err != nil {
		return errors.Wrap(err, "domain")
	}

	val, err := st.ValidatorAtIndexReadOnly(msg.ValidatorIndex)
	if err != nil {
		return fmt.Errorf("validator %d: %w", msg.ValidatorIndex, err)
	}
	pubkey := val.PublicKey()
	if err := signing.VerifySigningRoot(msg, pubkey[:], v.p.Signature, domain); err != nil {
		return errors.Wrap(err, "verify signature")
	}
	return nil
}

// SatisfyRequirement allows the caller to manually mark a requirement as satisfied.
func (v *ProposerPreferencesVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

func (v *ProposerPreferencesVerifier) message() *ethpb.ProposerPreferences {
	return v.p.GetMessage()
}

func (v *ProposerPreferencesVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}

	v.results.record(req, *err)
}
