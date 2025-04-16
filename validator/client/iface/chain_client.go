package iface

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
)

type ChainClient interface {
	ChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error)
	ValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error)
	Validators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error)
	ValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error)
	ValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error)
	ValidatorPerformance(context.Context, *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
}
