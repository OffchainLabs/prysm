package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
)

// NewPrysmChainClient returns implementation of iface.PrysmChainClient.
func NewPrysmChainClient(handler rest.Handler, nodeClient iface.NodeClient) iface.PrysmChainClient {
	return prysmChainClient{
		handler:    handler,
		nodeClient: nodeClient,
	}
}

type prysmChainClient struct {
	handler    rest.Handler
	nodeClient iface.NodeClient
}

func (c prysmChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	// Check node version for prysm beacon node as it is a custom endpoint for prysm beacon node.
	nodeVersion, err := c.nodeClient.Version(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node version")
	}

	if !strings.Contains(strings.ToLower(nodeVersion.Version), "prysm") {
		return nil, iface.ErrNotSupported
	}

	request, err := json.Marshal(structs.GetValidatorPerformanceRequest{
		PublicKeys: in.PublicKeys,
		Indices:    in.Indices,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}
	resp := &structs.GetValidatorPerformanceResponse{}
	if err = c.handler.Post(ctx, "/prysm/validators/performance", nil, bytes.NewBuffer(request), resp); err != nil {
		return nil, err
	}

	return &ethpb.ValidatorPerformanceResponse{
		CurrentEffectiveBalances:      resp.CurrentEffectiveBalances,
		CorrectlyVotedSource:          resp.CorrectlyVotedSource,
		CorrectlyVotedTarget:          resp.CorrectlyVotedTarget,
		CorrectlyVotedHead:            resp.CorrectlyVotedHead,
		BalancesBeforeEpochTransition: resp.BalancesBeforeEpochTransition,
		BalancesAfterEpochTransition:  resp.BalancesAfterEpochTransition,
		MissingValidators:             resp.MissingValidators,
		PublicKeys:                    resp.PublicKeys,
		InactivityScores:              resp.InactivityScores,
	}, nil
}
