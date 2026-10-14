package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/decoupled"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Spec: goldfish_vote, served from the head captured at the slot's Δ. Any other slot, or an unjoined node, gets Unavailable rather than a fresh calculation.
func (vs *Server) AvailableAttestationData(_ context.Context, req *ethpb.DecoupledAttestationDataRequest) (*ethpb.AvailableAttestationData, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	root, ok := vs.ForkchoiceFetcher.DecoupledVote(req.Slot)
	if !ok {
		return nil, status.Errorf(codes.Unavailable, "no vote captured for slot %d", req.Slot)
	}
	return &ethpb.AvailableAttestationData{Slot: req.Slot, BeaconBlockRoot: root[:]}, nil
}

func (vs *Server) SubmitAvailableAttestation(ctx context.Context, msg *ethpb.AvailableAttestationMessage) (*emptypb.Empty, error) {
	if msg == nil || msg.Data == nil {
		return nil, status.Error(codes.InvalidArgument, "available attestation is nil")
	}
	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	if err := decoupled.VerifyAvailableAttestation(st, msg); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid available attestation: %v", err)
	}
	// Local receipt first, the engine stamps the vote and a slow broadcast must not push it past a cutoff.
	if err := vs.DecoupledReceiver.ReceiveAvailableAttestation(ctx, msg); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not process available attestation: %v", err)
	}
	if err := vs.P2P.Broadcast(ctx, msg); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast available attestation: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// Spec: fg_attest's inputs, served from the source and head captured at the round's action time.
func (vs *Server) FinalityAttestationData(_ context.Context, req *ethpb.DecoupledAttestationDataRequest) (*ethpb.FinalityAttestationData, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	data, ok := vs.ForkchoiceFetcher.DecoupledFinalityData(req.Slot)
	if !ok {
		return nil, status.Errorf(codes.Unavailable, "no finality source captured for slot %d", req.Slot)
	}
	return data, nil
}

func (vs *Server) SubmitFinalityAttestation(ctx context.Context, msg *ethpb.FinalityAttestationMessage) (*emptypb.Empty, error) {
	if msg == nil || msg.Data == nil {
		return nil, status.Error(codes.InvalidArgument, "finality attestation is nil")
	}
	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	if err := decoupled.VerifyFinalityAttestation(st, msg); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid finality attestation: %v", err)
	}
	if err := vs.DecoupledFGPool.InsertSingle(msg.ValidatorIndex, msg.Data, msg.Signature); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not pool finality attestation: %v", err)
	}
	if err := vs.P2P.Broadcast(ctx, msg); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast finality attestation: %v", err)
	}
	return &emptypb.Empty{}, nil
}
