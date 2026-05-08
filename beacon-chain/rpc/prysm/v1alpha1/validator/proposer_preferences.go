package validator

import (
	"context"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitSignedProposerPreferences broadcasts signed proposer preferences and
// caches them locally for subsequent bid validation.
// Local submissions intentionally bypass full gossip verification (proposer
// lookahead, signature) because the validator client is trusted.
func (vs *Server) SubmitSignedProposerPreferences(
	ctx context.Context,
	req *ethpb.SubmitSignedProposerPreferencesRequest,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitSignedProposerPreferences")
	defer span.End()

	if req == nil || len(req.SignedProposerPreferences) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "signed proposer preferences request is empty")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())

	var broadcast int
	for _, msg := range req.SignedProposerPreferences {
		if msg == nil || msg.Message == nil {
			return nil, status.Errorf(codes.InvalidArgument, "signed proposer preferences message is nil")
		}

		proposalSlot := msg.Message.ProposalSlot
		if slots.ToEpoch(proposalSlot) < params.BeaconConfig().GloasForkEpoch {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"signed proposer preferences are not supported before Gloas fork (slot %d)",
				proposalSlot,
			)
		}

		proposalEpoch := slots.ToEpoch(proposalSlot)
		if proposalEpoch < currentEpoch || proposalEpoch > currentEpoch.Add(1) {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"signed proposer preferences proposal slot must be in the current or next epoch: slot %d currentEpoch %d",
				proposalSlot,
				currentEpoch,
			)
		}

		valIdx := msg.Message.ValidatorIndex
		currentSlot := vs.TimeFetcher.CurrentSlot()
		if proposalSlot <= currentSlot {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"signed proposer preferences proposal slot has already passed: proposalSlot %d currentSlot %d",
				proposalSlot,
				currentSlot,
			)
		}

		if len(msg.Message.DependentRoot) != fieldparams.RootLength {
			return nil, status.Errorf(codes.InvalidArgument,
				"signed proposer preferences dependent_root must be %d bytes (got %d)",
				fieldparams.RootLength, len(msg.Message.DependentRoot),
			)
		}
		var dependentRoot [fieldparams.RootLength]byte
		copy(dependentRoot[:], msg.Message.DependentRoot)

		if err := vs.P2P.BroadcastForEpoch(ctx, msg, slots.ToEpoch(proposalSlot)); err != nil {
			return nil, status.Errorf(codes.Internal,
				"Could not broadcast signed proposer preferences (broadcast %d/%d): %v",
				broadcast, len(req.SignedProposerPreferences), err)
		}

		// Write to both stores: external (so FCU lookups by
		// (slot, dependent_root) honor the spec-aligned preference) and
		// owned (so Validating()/Indices() reflect this validator). The
		// fee/gas data is duplicated intentionally — see AddOwned.
		vs.ProposerPreferencesCache.AddOwned(dependentRoot, proposalSlot, valIdx, msg.Message.FeeRecipient, msg.Message.GasLimit)
		broadcast++
	}

	log.WithFields(logrus.Fields{
		"total":     len(req.SignedProposerPreferences),
		"broadcast": broadcast,
	}).Debug("Processed signed proposer preferences")
	return &emptypb.Empty{}, nil
}
