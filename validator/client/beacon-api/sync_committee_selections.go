package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
)

type aggregatedSyncSelectionResponse struct {
	Data []iface.SyncCommitteeSelection `json:"data"`
}

func (c *beaconApiValidatorClient) aggregatedSyncSelections(ctx context.Context, selections []iface.SyncCommitteeSelection) ([]iface.SyncCommitteeSelection, error) {
	body, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal selections")
	}

	var resp aggregatedSyncSelectionResponse
	err = c.jsonRestHandler.Post(ctx, "/eth/v1/validator/sync_committee_selections", nil, bytes.NewBuffer(body), &resp)
	if err != nil {
		return nil, errors.Wrap(err, "error calling post endpoint")
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("no aggregated sync selections returned")
	}
	if len(selections) != len(resp.Data) {
		return nil, errors.New("mismatching number of sync selections")
	}

	return resp.Data, nil
}
