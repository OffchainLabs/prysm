package core

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SubmitSignedProposerPreferences broadcasts proposer preferences and caches them for bid validation.
func (s *Service) SubmitSignedProposerPreferences(ctx context.Context, req *ethpb.SubmitSignedProposerPreferencesRequest) *RpcError {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitSignedProposerPreferences")
	defer span.End()

	if req == nil || len(req.SignedProposerPreferences) == 0 {
		return &RpcError{Reason: BadRequest, Err: errors.New("signed proposer preferences request is empty")}
	}
	if s.SyncChecker.Syncing() {
		return &RpcError{Reason: Unavailable, Err: errors.New("Syncing to latest head, not ready to respond")}
	}

	currentEpoch := slots.ToEpoch(s.GenesisTimeFetcher.CurrentSlot())
	var broadcast, duplicate int
	for _, msg := range req.SignedProposerPreferences {
		if msg == nil || msg.Message == nil {
			return &RpcError{Reason: BadRequest, Err: errors.New("signed proposer preferences message is nil")}
		}

		proposalSlot := msg.Message.ProposalSlot
		if slots.ToEpoch(proposalSlot) < params.BeaconConfig().GloasForkEpoch {
			return &RpcError{Reason: BadRequest, Err: errors.Errorf("signed proposer preferences are not supported before Gloas fork (slot %d)", proposalSlot)}
		}

		proposalEpoch := slots.ToEpoch(proposalSlot)
		if proposalEpoch < currentEpoch || proposalEpoch > currentEpoch.Add(1) {
			return &RpcError{Reason: BadRequest, Err: errors.Errorf(
				"signed proposer preferences proposal slot must be in the current or next epoch: slot %d currentEpoch %d",
				proposalSlot,
				currentEpoch,
			)}
		}

		currentSlot := s.GenesisTimeFetcher.CurrentSlot()
		if proposalSlot <= currentSlot {
			return &RpcError{Reason: BadRequest, Err: errors.Errorf(
				"signed proposer preferences proposal slot has already passed: proposalSlot %d currentSlot %d",
				proposalSlot,
				currentSlot,
			)}
		}

		if len(msg.Message.DependentRoot) != fieldparams.RootLength {
			return &RpcError{Reason: BadRequest, Err: errors.Errorf(
				"signed proposer preferences dependent_root must be %d bytes (got %d)",
				fieldparams.RootLength,
				len(msg.Message.DependentRoot),
			)}
		}
		dependentRoot := bytesutil.ToBytes32(msg.Message.DependentRoot)

		if s.ProposerPreferencesCache.Has(dependentRoot, proposalSlot) {
			duplicate++
			continue
		}

		if err := s.P2P.BroadcastForEpoch(ctx, msg, slots.ToEpoch(proposalSlot)); err != nil {
			return &RpcError{Reason: Internal, Err: errors.Wrapf(
				err,
				"Could not broadcast signed proposer preferences (broadcast %d/%d)",
				broadcast,
				len(req.SignedProposerPreferences),
			)}
		}

		s.ProposerPreferencesCache.Add(cache.ProposerPreference{
			DependentRoot:  dependentRoot,
			ValidatorIndex: msg.Message.ValidatorIndex,
			FeeRecipient:   bytesutil.ToBytes20(msg.Message.FeeRecipient),
			TargetGasLimit: msg.Message.TargetGasLimit,
		}, proposalSlot)

		s.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: opfeed.ProposerPreferencesReceived,
			Data: &opfeed.ProposerPreferencesReceivedData{Data: msg},
		})
		broadcast++
	}

	log.WithFields(logrus.Fields{
		"total":     len(req.SignedProposerPreferences),
		"broadcast": broadcast,
		"duplicate": duplicate,
	}).Debug("Processed signed proposer preferences")
	return nil
}
