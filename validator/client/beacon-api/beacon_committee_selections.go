package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/pkg/errors"
)

type aggregatedSelectionResponse struct {
	Data []iface.BeaconCommitteeSelection `json:"data"`
}

func (c *beaconApiValidatorClient) aggregatedSelection(ctx context.Context, selections []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
	body, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal selections")
	}

	var resp aggregatedSelectionResponse
	err = c.jsonRestHandler.Post(ctx, "/eth/v1/validator/beacon_committee_selections", nil, bytes.NewBuffer(body), &resp)
	if err != nil {
		return nil, errors.Wrap(err, "error calling post endpoint")
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("no aggregated selection returned")
	}
	if len(selections) != len(resp.Data) {
		return nil, errors.New("mismatching number of selections")
	}

	return resp.Data, nil
}
