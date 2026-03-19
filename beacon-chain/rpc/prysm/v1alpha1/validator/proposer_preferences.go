package validator

import (
	"context"

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

	var broadcast, duplicate int
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

		if slots.ToEpoch(proposalSlot) != currentEpoch+1 {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"signed proposer preferences proposal slot must be in the next epoch: slot %d currentEpoch %d",
				proposalSlot,
				currentEpoch,
			)
		}

		if vs.ProposerPreferencesCache.Has(proposalSlot) {
			duplicate++
			continue
		}

		if err := vs.P2P.Broadcast(ctx, msg); err != nil {
			return nil, status.Errorf(codes.Internal,
				"Could not broadcast signed proposer preferences (broadcast %d/%d): %v",
				broadcast, len(req.SignedProposerPreferences), err)
		}

		vs.ProposerPreferencesCache.Add(proposalSlot, msg.Message.FeeRecipient, msg.Message.GasLimit)
		broadcast++
	}

	log.WithFields(logrus.Fields{
		"total":     len(req.SignedProposerPreferences),
		"broadcast": broadcast,
		"duplicate": duplicate,
	}).Debug("Processed signed proposer preferences")
	return &emptypb.Empty{}, nil
}
