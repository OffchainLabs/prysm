package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

func (s *Service) isValidDependentRoot(root [32]byte, epoch primitives.Epoch) bool {
	tips, _ := s.cfg.chain.ChainHeads()
	for _, tip := range tips {
		if tip == root {
			continue
		}
		depRoot, err := s.cfg.chain.DependentRootForEpoch(tip, epoch)
		if err == nil && depRoot == root {
			return true
		}
	}
	return false
}

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
	dependentEpoch := proposalEpoch
	if dependentEpoch > 0 {
		dependentEpoch--
	}
	headRoot, err := s.cfg.chain.HeadRoot(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "head root")
	}
	headDependentRoot, err := s.cfg.chain.DependentRootForEpoch(bytesutil.ToBytes32(headRoot), dependentEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "head dependent root")
	}

	var st state.ReadOnlyBeaconState
	stateRoot := headRoot
	if headDependentRoot == dependentRoot {
		st, err = s.cfg.chain.HeadStateReadOnly(ctx)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "head state")
		}
	} else {
		// [IGNORE] is_valid_dependent_root(store, preferences.dependent_root, lookahead_epoch).
		if !s.isValidDependentRoot(dependentRoot, dependentEpoch) {
			return pubsub.ValidationIgnore, errors.Errorf("dependent_root %#x is not a possible dependent block", dependentRoot)
		}
		// Off the head branch the lookahead comes from the dependent block's own state, never from a regeneration.
		st = s.cfg.stateGen.StateByRootIfCachedNoCopy(dependentRoot)
		if st == nil {
			return pubsub.ValidationIgnore, errors.Errorf("state for dependent_root %#x is not cached", dependentRoot)
		}
		stateRoot = dependentRoot[:]
	}
	stateEpoch := slots.ToEpoch(st.Slot())

	// Sole permitted slot advance: up to the lookahead epoch boundary, which bounds
	// the work a single message can cause.
	if proposalEpoch == stateEpoch.AddEpoch(2) {
		boundarySlot, err := slots.EpochStart(dependentEpoch)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "compute boundary slot")
		}
		st, err = transition.ProcessSlotsIfNeeded(ctx, st, stateRoot, boundarySlot)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "advance state to boundary")
		}
	} else if proposalEpoch > stateEpoch.AddEpoch(1) {
		return pubsub.ValidationIgnore, errors.Errorf("state epoch %d cannot verify proposal epoch %d", stateEpoch, proposalEpoch)
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

	s.proposerPreferencesCache.Add(cache.ProposerPreference{
		DependentRoot:  dependentRoot,
		ValidatorIndex: signedPreferences.Message.ValidatorIndex,
		FeeRecipient:   bytesutil.ToBytes20(signedPreferences.Message.FeeRecipient),
		TargetGasLimit: signedPreferences.Message.TargetGasLimit,
	}, slot)
	msg.ValidatorData = signedPreferences
	return pubsub.ValidationAccept, nil
}
