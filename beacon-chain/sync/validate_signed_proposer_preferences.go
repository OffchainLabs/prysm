package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

func (s *Service) validateSignedProposerPreferencesGossip(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateSignedProposerPreferencesGossip")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	signedPreferences, ok := m.(*ethpb.SignedProposerPreferences)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if signedPreferences.Message == nil {
		return pubsub.ValidationReject, errNilMessage
	}

	headStateRO, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	v := s.newSignedProposerPreferencesVerifier(signedPreferences, verification.SignedProposerPreferencesGossipRequirements)
	// [IGNORE] preferences.proposal_slot is in the next epoch.
	if err := v.VerifyNextEpoch(headStateRO); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// Ensure the state's lookahead covers the proposal slot's epoch.
	// The lookahead spans [stateEpoch, stateEpoch+1]. If the proposal
	// epoch is more than one epoch ahead of the state, advance the state
	// so the lookahead is current.
	st, err := s.proposerPreferencesState(ctx, headStateRO, signedPreferences.Message.ProposalSlot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] preferences.validator_index is present at the correct slot in the
	// proposer lookahead for the proposal slot.
	if err := v.VerifyValidProposalSlot(st); err != nil {
		return pubsub.ValidationReject, err
	}

	slot := signedPreferences.Message.ProposalSlot
	// [IGNORE] This is the first valid signed proposer preferences message
	// received for the given proposal slot.
	if s.proposerPreferencesCache.Has(slot) {
		return pubsub.ValidationIgnore, nil
	}
	// [REJECT] signed_proposer_preferences.signature is valid with respect to the
	// validator's public key.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}

	s.proposerPreferencesCache.Add(slot, signedPreferences.Message.FeeRecipient, signedPreferences.Message.GasLimit)
	msg.ValidatorData = signedPreferences
	return pubsub.ValidationAccept, nil
}

// proposerPreferencesState returns a state whose lookahead covers the proposal
// slot. If the head state's epoch is the same as or one less than the proposal
// epoch, the head state is returned as-is. Otherwise the state is advanced to
// the first slot of the epoch before the proposal epoch so the lookahead is
// up to date.
func (s *Service) proposerPreferencesState(ctx context.Context, headStateRO state.ReadOnlyBeaconState, proposalSlot primitives.Slot) (state.ReadOnlyBeaconState, error) {
	stateEpoch := slots.ToEpoch(headStateRO.Slot())
	proposalEpoch := slots.ToEpoch(proposalSlot)

	// Lookahead covers [stateEpoch, stateEpoch+1], so if the proposal
	// epoch falls within that range the head state is sufficient.
	if proposalEpoch == stateEpoch || proposalEpoch == stateEpoch+1 {
		return headStateRO, nil
	}

	// State is too far behind — advance it to the first slot of the epoch
	// before the proposal epoch. After advancement the lookahead covers
	// [proposalEpoch-1, proposalEpoch], so the proposal slot is accessible
	// in the next-epoch portion.
	targetSlot, err := slots.EpochStart(proposalEpoch - 1)
	if err != nil {
		return nil, err
	}
	headRoot, err := s.cfg.chain.HeadRoot(ctx)
	if err != nil {
		return nil, err
	}
	headState, err := s.cfg.chain.HeadState(ctx)
	if err != nil {
		return nil, err
	}
	return transition.ProcessSlotsUsingNextSlotCache(ctx, headState, headRoot, targetSlot)
}

func (s *Service) signedProposerPreferencesSubscriber(_ context.Context, msg proto.Message) error {
	_, ok := msg.(*ethpb.SignedProposerPreferences)
	if !ok {
		return errWrongMessage
	}
	return nil
}
