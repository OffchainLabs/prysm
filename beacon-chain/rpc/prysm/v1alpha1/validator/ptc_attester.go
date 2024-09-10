package validator

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetPayloadAttestationData returns the payload attestation data for a given slot.
// The request slot must be the current slot and there must exist a block from the current slot or the request will fail.
func (vs *Server) GetPayloadAttestationData(ctx context.Context, req *ethpb.GetPayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	reqSlot := req.Slot
	currentSlot := vs.TimeFetcher.CurrentSlot()
	if reqSlot != currentSlot {
		return nil, status.Errorf(codes.InvalidArgument, "Payload attestation request slot %d != current slot %d", reqSlot, currentSlot)
	}

	highestSlot, root := vs.ForkchoiceFetcher.HighestReceivedBlockSlotRoot()
	if reqSlot != highestSlot {
		return nil, status.Errorf(codes.Unavailable, "Did not receive current slot %d block ", reqSlot)
	}

	payloadStatus := vs.ForkchoiceFetcher.GetPTCVote(root)

	return &ethpb.PayloadAttestationData{
		BeaconBlockRoot: root[:],
		Slot:            highestSlot,
		PayloadStatus:   payloadStatus,
	}, nil
}

// SubmitPayloadAttestation broadcasts a payload attestation message to the network and saves the payload attestation to the cache.
// This handler does not validate the payload attestation message before broadcasting and saving it to the cache.
// The caller should be responsible for validating the message, as it assumes a trusted relationship between the caller and the server.
func (vs *Server) SubmitPayloadAttestation(ctx context.Context, a *ethpb.PayloadAttestationMessage) (*empty.Empty, error) {
	// Broadcast the payload attestation message to the network.
	if err := vs.P2P.Broadcast(ctx, a); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast payload attestation message: %v", err)
	}

	// Save the payload attestation to the cache.
	if err := vs.PayloadAttestationReceiver.ReceivePayloadAttestationMessage(ctx, a); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not save payload attestation to cache: %v", err)
	}

	return &empty.Empty{}, nil
}
