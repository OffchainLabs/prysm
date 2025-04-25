package grpc_api

import (
	"context"
	"fmt"
	"sort"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/helpers"
	statenative "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/validator"
	eth "github.com/OffchainLabs/prysm/v6/proto/eth/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type grpcPrysmChainClient struct {
	chainClient iface.ChainClient
}

func (g grpcPrysmChainClient) ValidatorCount(ctx context.Context, _ string, statuses []validator.Status) ([]iface.ValidatorCount, error) {
	resp, err := g.chainClient.Validators(ctx, &ethpb.ListValidatorsRequest{PageSize: 0})
	if err != nil {
		return nil, errors.Wrap(err, "list validators failed")
	}

	var vals []*ethpb.Validator
	for _, val := range resp.ValidatorList {
		vals = append(vals, val.Validator)
	}

	head, err := g.chainClient.ChainHead(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "get chain head")
	}

	if len(statuses) == 0 {
		for _, val := range eth.ValidatorStatus_value {
			statuses = append(statuses, validator.Status(val))
		}
	}

	valCount, err := validatorCountByStatus(vals, statuses, head.HeadEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "validator count by status")
	}

	return valCount, nil
}

// validatorCountByStatus returns a slice of validator count for each status in the given epoch.
func validatorCountByStatus(validators []*ethpb.Validator, statuses []validator.Status, epoch primitives.Epoch) ([]iface.ValidatorCount, error) {
	countByStatus := make(map[validator.Status]uint64)
	for _, val := range validators {
		readOnlyVal, err := statenative.NewValidator(val)
		if err != nil {
			return nil, fmt.Errorf("could not convert validator: %w", err)
		}
		valStatus, err := helpers.ValidatorStatus(readOnlyVal, epoch)
		if err != nil {
			return nil, fmt.Errorf("could not get validator status: %w", err)
		}
		valSubStatus, err := helpers.ValidatorSubStatus(readOnlyVal, epoch)
		if err != nil {
			return nil, fmt.Errorf("could not get validator sub status: %w", err)
		}

		for _, status := range statuses {
			if valStatus == status || valSubStatus == status {
				countByStatus[status]++
			}
		}
	}

	var resp []iface.ValidatorCount
	for status, count := range countByStatus {
		resp = append(resp, iface.ValidatorCount{
			Status: status.String(),
			Count:  count,
		})
	}

	// Sort the response slice according to status strings for deterministic ordering of validator count response.
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Status < resp[j].Status
	})

	return resp, nil
}

func (c *grpcPrysmChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	return c.chainClient.ValidatorPerformance(ctx, in)
}

func NewGrpcPrysmChainClient(cc grpc.ClientConnInterface) iface.PrysmChainClient {
	return &grpcPrysmChainClient{chainClient: &grpcChainClient{ethpb.NewBeaconChainClient(cc)}}
}
