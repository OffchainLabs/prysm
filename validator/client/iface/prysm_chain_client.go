package iface

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var ErrNotSupported = errors.New("endpoint not supported")

// PrysmChainClient defines an interface required to implement all the prysm specific custom endpoints.
type PrysmChainClient interface {
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
