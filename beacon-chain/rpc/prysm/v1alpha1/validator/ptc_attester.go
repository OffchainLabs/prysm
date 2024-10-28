package validator

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (vs *Server) GetPayloadAttestationData(ctx context.Context, req *ethpb.GetPayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	return nil, errors.New("not implemented")
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
