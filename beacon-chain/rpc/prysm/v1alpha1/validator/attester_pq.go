package validator

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (vs *Server) ProposeAttestationPQ(ctx context.Context, att *ethpb.SingleAttestationPQ) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.ProposeAttestationPQ")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	if att == nil || att.Data == nil || att.Data.Source == nil || att.Data.Target == nil {
		return nil, status.Error(codes.InvalidArgument, "Incomplete attestation")
	}
	if slots.ToEpoch(att.Data.Slot) != att.Data.Target.Epoch {
		return nil, status.Error(codes.InvalidArgument, "Attestation slot does not match the target epoch")
	}
	if att.Data.CommitteeIndex >= 2 {
		return nil, status.Error(codes.InvalidArgument, "index must be < 2 post-Gloas")
	}
	targetState, err := vs.AttestationStateFetcher.AttestationTargetState(ctx, att.Data.Target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get target state: %v", err)
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, targetState, att.Data.Slot, att.CommitteeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Could not get committee: %v", err)
	}
	if !slices.Contains(committee, att.AttesterIndex) {
		return nil, status.Error(codes.InvalidArgument, "Attester is not a member of the committee")
	}
	if err := pq.VerifySingleAttestation(targetState, att); err != nil {
		return nil, status.Error(codes.InvalidArgument, "Incorrect attestation signature")
	}
	root, err := att.Data.HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get attestation root: %v", err)
	}

	count, err := helpers.ActiveValidatorCount(ctx, targetState, att.Data.Target.Epoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get active validator count: %v", err)
	}
	subnet := helpers.ComputeSubnetFromCommitteeAndSlot(count, att.CommitteeId, att.Data.Slot)
	if err := vs.P2P.BroadcastAttestation(ctx, subnet, att); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast attestation: %v", err)
	}
	vs.PQAttPool.Save(att.Copy())

	return &ethpb.AttestResponse{AttestationDataRoot: root[:]}, nil
}
