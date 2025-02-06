package beacon_api

import (
	"context"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v6/api/server/structs"
	validator2 "github.com/prysmaticlabs/prysm/v6/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v6/validator/client/iface"
)

// NewPrysmChainClient returns implementation of iface.PrysmChainClient.
func NewPrysmChainClient(jsonRestHandler JsonRestHandler, nodeClient iface.NodeClient) iface.PrysmChainClient {
	return prysmChainClient{
		jsonRestHandler: jsonRestHandler,
		nodeClient:      nodeClient,
	}
}

type prysmChainClient struct {
	jsonRestHandler JsonRestHandler
	nodeClient      iface.NodeClient
}

func (c prysmChainClient) ValidatorCount(ctx context.Context, stateID string, statuses []validator2.Status) ([]iface.ValidatorCount, error) {
	// Check node version for prysm beacon node as it is a custom endpoint for prysm beacon node.
	nodeVersion, err := c.nodeClient.Version(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node version")
	}

	if !strings.Contains(strings.ToLower(nodeVersion.Version), "prysm") {
		return nil, iface.ErrNotSupported
	}

	queryParams := neturl.Values{}
	for _, status := range statuses {
		queryParams.Add("status", status.String())
	}

	queryUrl := buildURL(fmt.Sprintf("/eth/v1/beacon/states/%s/validator_count", stateID), queryParams)

	var validatorCountResponse structs.GetValidatorCountResponse
	if err = c.jsonRestHandler.Get(ctx, queryUrl, &validatorCountResponse); err != nil {
		return nil, err
	}

	if validatorCountResponse.Data == nil {
		return nil, errors.New("validator count data is nil")
	}

	if len(statuses) != 0 && len(statuses) != len(validatorCountResponse.Data) {
		return nil, errors.New("mismatch between validator count data and the number of statuses provided")
	}

	var resp []iface.ValidatorCount
	for _, vc := range validatorCountResponse.Data {
		count, err := strconv.ParseUint(vc.Count, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator count %s", vc.Count)
		}

		resp = append(resp, iface.ValidatorCount{
			Status: vc.Status,
			Count:  count,
		})
	}

	return resp, nil
}
