package iface

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type ChainClient interface {
	ChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error)
	ValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error)
	Validators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error)
	ValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error)
	ValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error)
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
