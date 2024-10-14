package validator

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (vs *Server) GetPayloadAttestationData(ctx context.Context, req *ethpb.GetPayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	return nil, errors.New("not implemented")
}

func (vs *Server) SubmitPayloadAttestation(ctx context.Context, in *ethpb.PayloadAttestationMessage) (*empty.Empty, error) {
	return nil, errors.New("not implemented")
}
