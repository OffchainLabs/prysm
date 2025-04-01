package shared_providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/httputil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type dutiesProvider struct {
	jsonRestHandler client.JsonRestHandler
}

// Committees retrieves the committees for the given epoch
func (c dutiesProvider) Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error) {
	committeeParams := url.Values{}
	committeeParams.Add("epoch", strconv.FormatUint(uint64(epoch), 10))
	committeesRequest := apiutil.BuildURL("/eth/v1/beacon/states/head/committees", committeeParams)

	var stateCommittees structs.GetCommitteesResponse
	if err := c.jsonRestHandler.Get(ctx, committeesRequest, &stateCommittees); err != nil {
		return nil, err
	}

	if stateCommittees.Data == nil {
		return nil, errors.New("state committees data is nil")
	}

	for index, committee := range stateCommittees.Data {
		if committee == nil {
			return nil, errors.Errorf("committee at index `%d` is nil", index)
		}
	}

	return stateCommittees.Data, nil
}

// AttesterDuties retrieves the attester duties for the given epoch and validatorIndices
func (c dutiesProvider) AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.AttesterDuty, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	attesterDuties := &structs.GetAttesterDutiesResponse{}
	if err = c.jsonRestHandler.Post(
		ctx,
		fmt.Sprintf("/eth/v1/validator/duties/attester/%d", epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		attesterDuties,
	); err != nil {
		return nil, err
	}

	for index, attesterDuty := range attesterDuties.Data {
		if attesterDuty == nil {
			return nil, errors.Errorf("attester duty at index `%d` is nil", index)
		}
	}

	return attesterDuties.Data, nil
}

// ProposerDuties retrieves the proposer duties for the given epoch
func (c dutiesProvider) ProposerDuties(ctx context.Context, epoch primitives.Epoch) ([]*structs.ProposerDuty, error) {
	proposerDuties := structs.GetProposerDutiesResponse{}
	if err := c.jsonRestHandler.Get(ctx, fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch), &proposerDuties); err != nil {
		return nil, err
	}

	if proposerDuties.Data == nil {
		return nil, errors.New("proposer duties data is nil")
	}

	for index, proposerDuty := range proposerDuties.Data {
		if proposerDuty == nil {
			return nil, errors.Errorf("proposer duty at index `%d` is nil", index)
		}
	}

	return proposerDuties.Data, nil
}

// SyncDuties retrieves the sync committee duties for the given epoch and validatorIndices
func (c dutiesProvider) SyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	syncDuties := structs.GetSyncCommitteeDutiesResponse{}
	if err = c.jsonRestHandler.Post(
		ctx,
		fmt.Sprintf("/eth/v1/validator/duties/sync/%d", epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		&syncDuties,
	); err != nil {
		return nil, err
	}

	if syncDuties.Data == nil {
		return nil, errors.New("sync duties data is nil")
	}

	for index, syncDuty := range syncDuties.Data {
		if syncDuty == nil {
			return nil, errors.Errorf("sync duty at index `%d` is nil", index)
		}
	}

	return syncDuties.Data, nil
}
