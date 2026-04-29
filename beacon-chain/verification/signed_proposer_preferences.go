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
	RequireProposerPreferencesCurrentOrNextEpoch,
	RequireProposerPreferencesProposalSlotValid,
	RequireProposerPreferencesSignatureValid,
})

var (
	ErrProposerPreferencesNotCurrentOrNextEpoch = errors.New("proposer preferences proposal slot is not in the current or next epoch")
	ErrProposerPreferencesSlotAlreadyPassed     = errors.New("proposer preferences proposal slot has already passed")
	ErrProposerPreferencesInvalidProposalSlot   = errors.New("proposer preferences validator is not assigned to the proposal slot")
)

var _ SignedProposerPreferencesVerifier = &ProposerPreferencesVerifier{}

// ProposerPreferencesVerifier is a read-only verifier for signed proposer preferences.
type ProposerPreferencesVerifier struct {
	*sharedResources
	results *results
	p       *ethpb.SignedProposerPreferences
}

// VerifyCurrentOrNextEpoch checks proposal_slot is in current or next epoch
// (wall-clock) and not already passed. The state argument is unused.
func (v *ProposerPreferencesVerifier) VerifyCurrentOrNextEpoch(_ state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireProposerPreferencesCurrentOrNextEpoch, &err)

	msg := v.message()
	currentSlot := v.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)
	proposalEpoch := slots.ToEpoch(msg.ProposalSlot)
	if proposalEpoch < currentEpoch || proposalEpoch > currentEpoch.Add(1) {
		return fmt.Errorf("%w: proposal epoch %d, current epoch %d",
			ErrProposerPreferencesNotCurrentOrNextEpoch, proposalEpoch, currentEpoch)
	}
	if msg.ProposalSlot <= currentSlot {
		return fmt.Errorf("%w: proposal slot %d <= current slot %d",
			ErrProposerPreferencesSlotAlreadyPassed, msg.ProposalSlot, currentSlot)
	}
	return nil
}

// VerifyValidProposalSlot checks the validator matches the proposer_lookahead
// entry for proposal_slot. The caller must pass the checkpoint state at
// epoch(proposal_slot)-1 anchored to preferences.dependent_root, with slots
// advanced through that boundary.
func (v *ProposerPreferencesVerifier) VerifyValidProposalSlot(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireProposerPreferencesProposalSlotValid, &err)

	msg := v.message()
	lookahead, err := st.ProposerLookahead()
	if err != nil {
		return errors.Wrap(err, "failed to get proposer lookahead")
	}

	stateEpoch := slots.ToEpoch(st.Slot())
	proposalEpoch := slots.ToEpoch(msg.ProposalSlot)
	if proposalEpoch < stateEpoch {
		return fmt.Errorf("%w: proposal epoch %d precedes checkpoint state epoch %d",
			ErrProposerPreferencesInvalidProposalSlot, proposalEpoch, stateEpoch)
	}
	slotIndex := primitives.Slot(proposalEpoch.Sub(uint64(stateEpoch)))*params.BeaconConfig().SlotsPerEpoch + (msg.ProposalSlot % params.BeaconConfig().SlotsPerEpoch)
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
