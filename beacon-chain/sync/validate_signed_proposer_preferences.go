package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
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
	expected, err := s.cfg.chain.DependentRootForEpoch(bytesutil.ToBytes32(headRoot), dependentEpoch)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "head dependent root")
	}
	if expected != dependentRoot {
		return pubsub.ValidationIgnore, errors.Errorf("dependent_root %#x does not match head %#x", dependentRoot, expected)
	}

	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "head state")
	}
	stateEpoch := slots.ToEpoch(st.Slot())

	// Sole permitted slot advance: next-epoch preference at the boundary before
	// the head processes a block in the new epoch (proposalEpoch == stateEpoch+2).
	if proposalEpoch == stateEpoch.AddEpoch(2) {
		boundarySlot, err := slots.EpochStart(dependentEpoch)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "compute boundary slot")
		}
		st, err = transition.ProcessSlotsIfNeeded(ctx, st, headRoot, boundarySlot)
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "advance head state to boundary")
		}
	} else if proposalEpoch > stateEpoch.AddEpoch(1) {
		return pubsub.ValidationIgnore, errors.Errorf("head epoch %d cannot verify proposal epoch %d", stateEpoch, proposalEpoch)
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
		ProposalSlot:   slot,
		ValidatorIndex: signedPreferences.Message.ValidatorIndex,
		FeeRecipient:   bytesutil.ToBytes20(signedPreferences.Message.FeeRecipient),
		TargetGasLimit: signedPreferences.Message.TargetGasLimit,
		Signature:      bytesutil.ToBytes96(signedPreferences.Signature),
	}, slot)
	msg.ValidatorData = signedPreferences
	return pubsub.ValidationAccept, nil
}

func (s *Service) signedProposerPreferencesSubscriber(_ context.Context, msg proto.Message) error {
	signedPreferences, ok := msg.(*ethpb.SignedProposerPreferences)
	if !ok {
		return errWrongMessage
	}
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ProposerPreferencesReceived,
		Data: &opfeed.ProposerPreferencesReceivedData{
			SignedProposerPreferences: signedPreferences,
		},
	})
	return nil
}
