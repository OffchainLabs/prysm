package validator_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
)

func (c *beaconApiValidatorClient) aggregatedSyncSelections(ctx context.Context, selections []beacon.SyncCommitteeSelection) ([]beacon.SyncCommitteeSelection, error) {
	body, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal selections")
	}

	var resp beacon.AggregatedSyncSelectionResponse
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
