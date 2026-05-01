package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// PayloadAttestationData returns payload attestation data for the given slot.
func (vs *Server) PayloadAttestationData(
	ctx context.Context,
	req *ethpb.PayloadAttestationDataRequest,
) (*ethpb.PayloadAttestationData, error) {
	ctx, span := trace.StartSpan(ctx, "grpc.PayloadAttestationData")
	defer span.End()
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload attestation data request is nil")
	}
	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	data, rpcErr := vs.CoreService.PayloadAttestationData(ctx, req.Slot)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}
	return data, nil
}

// SubmitPayloadAttestation submits a payload attestation message to the network
// and applies it locally.
func (vs *Server) SubmitPayloadAttestation(
	ctx context.Context,
	msg *ethpb.PayloadAttestationMessage,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "PTCServer.SubmitPayloadAttestation")
	defer span.End()
	if msg == nil || msg.Data == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload attestation message is nil")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	if slots.ToEpoch(msg.Data.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"payload attestations are not supported before Gloas fork (slot %d)", msg.Data.Slot)
	}

	currentSlot := vs.TimeFetcher.CurrentSlot()
	if msg.Data.Slot != currentSlot {
		return nil, status.Errorf(codes.InvalidArgument,
			"payload attestation message slot must match current slot: got %d, current %d", msg.Data.Slot, currentSlot)
	}

	if err := vs.P2P.Broadcast(ctx, msg); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast payload attestation message: %v", err)
	}

	if err := vs.PayloadAttestationReceiver.ReceivePayloadAttestationMessage(ctx, msg); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not process payload attestation message: %v", err)
	}

	idx, err := vs.payloadAttestationCommitteeIndex(ctx, msg)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Could not determine PTC committee index: %v", err)
	}
	if err := vs.PayloadAttestationPool.InsertPayloadAttestation(msg, idx); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not insert payload attestation into pool: %v", err)
	}

	vs.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.PayloadAttestationMessageReceived,
		Data: &opfeed.PayloadAttestationMessageReceivedData{
			Message: msg,
		},
	})

	log.WithField("slot", msg.Data.Slot).Debug("Submitted payload attestation message")
	return &emptypb.Empty{}, nil
}

func (vs *Server) payloadAttestationCommitteeIndex(ctx context.Context, msg *ethpb.PayloadAttestationMessage) (uint64, error) {
	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return 0, err
	}
	return gloas.PayloadCommitteeIndex(ctx, st, msg.Data.Slot, msg.ValidatorIndex)
}
