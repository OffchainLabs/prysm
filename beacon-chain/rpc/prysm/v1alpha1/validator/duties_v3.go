package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetAttesterDuties returns attester duties for the requested validators at the given epoch.
func (vs *Server) GetAttesterDuties(ctx context.Context, req *ethpb.AttesterDutiesRequest) (*ethpb.AttesterDutiesResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.GetAttesterDuties")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())
	if req.Epoch > currentEpoch+1 {
		return nil, status.Errorf(codes.InvalidArgument, "Request epoch %d can not be greater than next epoch %d", req.Epoch, currentEpoch+1)
	}

	s, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	s, err = vs.stateForEpoch(ctx, s, req.Epoch)
	if err != nil {
		return nil, err
	}

	duties, rpcErr := vs.CoreService.AttesterDuties(ctx, s, req.Epoch, req.ValidatorIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}

	var dependentRoot []byte
	if req.Epoch <= 1 {
		r, err := vs.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get genesis block root: %v", err)
		}
		dependentRoot = r[:]
	} else {
		dependentRoot, err = core.AttestationDependentRoot(s, req.Epoch)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get dependent root: %v", err)
		}
	}

	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine optimistic status: %v", err)
	}

	resp := &ethpb.AttesterDutiesResponse{
		DependentRoot:       dependentRoot,
		ExecutionOptimistic: optimistic,
		Duties:              make([]*ethpb.AttesterDuty, len(duties)),
	}
	for i, d := range duties {
		resp.Duties[i] = &ethpb.AttesterDuty{
			Pubkey:                  d.Pubkey[:],
			ValidatorIndex:          d.ValidatorIndex,
			CommitteeIndex:          d.CommitteeIndex,
			CommitteeLength:         d.CommitteeLength,
			CommitteesAtSlot:        d.CommitteesAtSlot,
			ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
			Slot:                    d.Slot,
		}
	}

	return resp, nil
}

// GetProposerDutiesV2 returns proposer duties for the given epoch.
func (vs *Server) GetProposerDutiesV2(ctx context.Context, req *ethpb.ProposerDutiesRequest) (*ethpb.ProposerDutiesResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.GetProposerDutiesV2")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())
	if req.Epoch > currentEpoch+1 {
		return nil, status.Errorf(codes.InvalidArgument, "Request epoch %d can not be greater than next epoch %d", req.Epoch, currentEpoch+1)
	}

	s, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	s, err = vs.stateForEpoch(ctx, s, req.Epoch)
	if err != nil {
		return nil, err
	}

	duties, rpcErr := vs.CoreService.ProposerDuties(ctx, s, req.Epoch)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}

	var dependentRoot []byte
	if req.Epoch == 0 {
		r, err := vs.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get genesis block root: %v", err)
		}
		dependentRoot = r[:]
	} else {
		dependentRoot, err = core.ProposalDependentRootV2(s, req.Epoch)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get dependent root: %v", err)
		}
	}

	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine optimistic status: %v", err)
	}

	resp := &ethpb.ProposerDutiesResponse{
		DependentRoot:       dependentRoot,
		ExecutionOptimistic: optimistic,
		Duties:              make([]*ethpb.ProposerDutyV2, len(duties)),
	}
	for i, d := range duties {
		resp.Duties[i] = &ethpb.ProposerDutyV2{
			Pubkey:         d.Pubkey[:],
			ValidatorIndex: d.ValidatorIndex,
			Slot:           d.Slot,
		}
	}

	return resp, nil
}

// GetSyncCommitteeDuties returns sync committee duties for the requested validators at the given epoch.
func (vs *Server) GetSyncCommitteeDuties(ctx context.Context, req *ethpb.SyncCommitteeDutiesRequest) (*ethpb.SyncCommitteeDutiesResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.GetSyncCommitteeDuties")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())
	lastValidEpoch := core.SyncCommitteeDutiesLastValidEpoch(currentEpoch)
	if req.Epoch > lastValidEpoch {
		return nil, status.Errorf(codes.InvalidArgument, "Request epoch %d can not be greater than last valid epoch %d for sync committee duties", req.Epoch, lastValidEpoch)
	}

	s, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	s, err = vs.stateForEpoch(ctx, s, req.Epoch)
	if err != nil {
		return nil, err
	}

	duties, rpcErr := vs.CoreService.SyncCommitteeDuties(ctx, s, req.Epoch, currentEpoch, req.ValidatorIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}

	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine optimistic status: %v", err)
	}

	resp := &ethpb.SyncCommitteeDutiesResponse{
		ExecutionOptimistic: optimistic,
		Duties:              make([]*ethpb.SyncCommitteeDuty, len(duties)),
	}
	for i, d := range duties {
		resp.Duties[i] = &ethpb.SyncCommitteeDuty{
			Pubkey:                        d.Pubkey[:],
			ValidatorIndex:                d.ValidatorIndex,
			ValidatorSyncCommitteeIndices: d.ValidatorSyncCommitteeIndices,
		}
	}

	return resp, nil
}
