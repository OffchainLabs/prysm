package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
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
	if len(signedPreferences.Message.DependentRoot) != fieldparams.RootLength {
		return pubsub.ValidationReject, errors.New("dependent_root must be 32 bytes")
	}

	v := s.newSignedProposerPreferencesVerifier(signedPreferences, verification.SignedProposerPreferencesGossipRequirements)

	// [IGNORE] proposal_slot is in current or next epoch and not already passed (wall-clock only).
	if err := v.VerifyCurrentOrNextEpoch(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	dependentRoot := bytesutil.ToBytes32(signedPreferences.Message.DependentRoot)
	// [IGNORE] block with root preferences.dependent_root has been seen.
	if !s.cfg.chain.InForkchoice(dependentRoot) && !s.cfg.beaconDB.HasBlock(ctx, dependentRoot) {
		return pubsub.ValidationIgnore, errors.New("dependent_root block not seen yet")
	}

	slot := signedPreferences.Message.ProposalSlot
	// [IGNORE] dedup on (dependent_root, proposal_slot) before the checkpoint
	// state load so byte-mutated duplicates can't amplify state work.
	if s.proposerPreferencesCache.Has(dependentRoot, slot) {
		return pubsub.ValidationIgnore, nil
	}

	// Checkpoint state at epoch(proposal_slot)-1 anchored to dependent_root.
	proposalEpoch := slots.ToEpoch(slot)
	// Underflow at epoch 0 collapses to epoch 0 — the spec's genesis-adjacent fallback.
	dependentEpoch, _ := proposalEpoch.SafeSub(1)
	boundarySlot, err := slots.EpochStart(dependentEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "compute checkpoint boundary slot")
	}
	var st state.ReadOnlyBeaconState
	// NextSlotState may return a state at slot < boundarySlot when the
	// dependent block was at an earlier slot (empty boundary slot); only use it
	// if it lands exactly on the boundary, otherwise fall through to load+advance.
	if cached := transition.NextSlotState(dependentRoot[:], boundarySlot); cached != nil && cached.Slot() == boundarySlot {
		st = cached
	} else {
		loaded, err := s.cfg.stateGen.StateByRootNoCopy(ctx, dependentRoot)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "load checkpoint state")
		}
		st, err = transition.ProcessSlotsIfNeeded(ctx, loaded, dependentRoot[:], boundarySlot)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "advance checkpoint state to boundary")
		}
	}

	// [REJECT] is_valid_proposal_slot(state, preferences) returns True, where state
	// is the checkpoint state at the epoch compute_epoch_at_slot(proposal_slot) - 1
	// and the root preferences.dependent_root.
	if err := v.VerifyValidProposalSlot(st); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] signed_proposer_preferences.signature is valid with respect to the
	// validator's public key.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}

	s.proposerPreferencesCache.Add(dependentRoot, slot, signedPreferences.Message.ValidatorIndex, signedPreferences.Message.FeeRecipient, signedPreferences.Message.GasLimit)
	msg.ValidatorData = signedPreferences
	return pubsub.ValidationAccept, nil
}

func (s *Service) signedProposerPreferencesSubscriber(_ context.Context, msg proto.Message) error {
	_, ok := msg.(*ethpb.SignedProposerPreferences)
	if !ok {
		return errWrongMessage
	}
	return nil
}
