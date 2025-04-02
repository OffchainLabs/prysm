package prysm_api

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// Client defines an interface required to implement all the prysm specific custom endpoints.
type Client interface {
	ValidatorCount(context.Context, string, []validator.Status) ([]beacon.ValidatorCount, error)
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
