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

type beaconApiChainClient struct {
	handler rest.Handler
}

func (c beaconApiChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	// Note: ValidatorPerformance is only supported on Prysm nodes,
	// So we should check whether the node is Prysm by node version.
	var versionResponse structs.GetVersionResponse
	if err := c.handler.Get(ctx, "/eth/v1/node/version", &versionResponse); err != nil {
		return nil, errors.Wrap(err, "failed to get node version")
	}

	if versionResponse.Data == nil || versionResponse.Data.Version == "" {
		return nil, errors.New("empty version response")
	}

	if !strings.Contains(strings.ToLower(versionResponse.Data.Version), "prysm") {
		return nil, iface.ErrNotSupported
	}

	// Now confirmed that the node is Prysm, call Prysm-specific performace endpoint.
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

func NewChainClient(provider rest.RestConnectionProvider) iface.ChainClient {
	handler := provider.Handler()
	return &beaconApiChainClient{
		handler: handler,
	}
}
