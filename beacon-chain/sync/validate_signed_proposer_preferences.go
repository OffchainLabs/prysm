package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
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

	// [IGNORE] compute_epoch_at_slot(proposal_slot) >= GLOAS_FORK_EPOCH.
	if slots.ToEpoch(signedPreferences.Message.ProposalSlot) < params.BeaconConfig().GloasForkEpoch {
		return pubsub.ValidationIgnore, nil
	}

	v := s.newSignedProposerPreferencesVerifier(signedPreferences, verification.SignedProposerPreferencesGossipRequirements)

	// [IGNORE] proposal_slot is in current or next epoch and not already passed (wall-clock only).
	if err := v.VerifyCurrentOrNextEpoch(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	dependentRoot := bytesutil.ToBytes32(signedPreferences.Message.DependentRoot)
	// [IGNORE] block with root preferences.dependent_root has been seen.
	seen := func(root [32]byte) bool {
		return s.cfg.chain.InForkchoice(root) || s.cfg.beaconDB.HasBlock(ctx, root)
	}
	if err := v.VerifyDependentRootSeen(seen); err != nil {
		return pubsub.ValidationIgnore, err
	}

	slot := signedPreferences.Message.ProposalSlot
	// [IGNORE] dedup on (dependent_root, proposal_slot) before any state work
	// so byte-mutated duplicates can't amplify it.
	if s.proposerPreferencesCache.Has(dependentRoot, slot) {
		return pubsub.ValidationIgnore, nil
	}

	proposalEpoch := slots.ToEpoch(slot)
	dependentEpoch := slots.ShufflingLookaheadEpoch(proposalEpoch)

	// [REJECT] the dependent block is not after compute_shuffling_dependent_slot(proposal_epoch).
	dependentSlot, err := slots.ShufflingDependentSlot(proposalEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "shuffling dependent slot")
	}
	blockSlot, err := s.cfg.chain.RecentBlockSlot(dependentRoot)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "dependent root slot")
	}
	if blockSlot > dependentSlot {
		return pubsub.ValidationReject, errors.Errorf("dependent block slot %d is after shuffling dependent slot %d", blockSlot, dependentSlot)
	}

	// [IGNORE] is_valid_dependent_root(store, preferences.dependent_root, lookahead_epoch).
	// The state comes from the head state or a cached dependent state, never from a regeneration.
	st, err := s.cfg.chain.DependentState(ctx, dependentRoot, dependentEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "dependent state")
	}

	// [REJECT] signed_proposer_preferences.signature is valid with respect to the
	// validator's public key.
	//
	// The signature needs only the validator's public key and the genesis
	// validators root, neither of which changes when slots are advanced, so it
	// is checked against the un-advanced state. A forged message is rejected
	// here before it can trigger the epoch transition below, and it is never
	// cached, so this is the only place that bounds the work it can cause.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}

	// Sole permitted slot advance: up to the lookahead epoch boundary, which bounds
	// the work a single message can cause. Only a message with a valid signature
	// reaches this point.
	st, err = s.cfg.chain.LookaheadState(ctx, dependentRoot, dependentEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "lookahead state")
	}

	// [REJECT] is_valid_proposal_slot(state, preferences) returns True, where state
	// is the checkpoint state at the epoch compute_epoch_at_slot(proposal_slot) - 1
	// and the root preferences.dependent_root.
	if err := v.VerifyValidProposalSlot(st); err != nil {
		return pubsub.ValidationReject, err
	}

	s.proposerPreferencesCache.Add(cache.ProposerPreference{
		DependentRoot:  dependentRoot,
		ValidatorIndex: signedPreferences.Message.ValidatorIndex,
		FeeRecipient:   bytesutil.ToBytes20(signedPreferences.Message.FeeRecipient),
		TargetGasLimit: signedPreferences.Message.TargetGasLimit,
		Signature:      bytesutil.ToBytes96(signedPreferences.Signature),
	}, slot)
	msg.ValidatorData = signedPreferences
	return pubsub.ValidationAccept, nil
}
