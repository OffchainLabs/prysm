package iface

import (
	"context"
	"errors"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var ErrNotSupported = errors.New("endpoint not supported")

type ChainClient interface {
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
