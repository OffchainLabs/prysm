package iface

import (
	"context"
	"errors"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
)

var ErrNotSupported = errors.New("endpoint not supported")

type ChainClient interface {
	ChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error)
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
