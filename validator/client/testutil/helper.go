package testutil

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// GenerateMultipleValidatorStatusResponse prepares a response from the passed in keys.
func GenerateMultipleValidatorStatusResponse(pubkeys [][]byte) *ethpb.MultipleValidatorStatusResponse {
	resp := &ethpb.MultipleValidatorStatusResponse{
		PublicKeys: make([][]byte, len(pubkeys)),
		Statuses:   make([]*ethpb.ValidatorStatusResponse, len(pubkeys)),
		Indices:    make([]primitives.ValidatorIndex, len(pubkeys)),
	}
	for i, key := range pubkeys {
		resp.PublicKeys[i] = key
		resp.Statuses[i] = &ethpb.ValidatorStatusResponse{
			Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
		}
		resp.Indices[i] = primitives.ValidatorIndex(i)
	}

	return resp
}
