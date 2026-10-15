package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

const inclusionListEndpoint = "/eth/v1/validator/inclusion_list"

func (c *beaconApiValidatorClient) produceInclusionList(ctx context.Context, slot primitives.Slot) ([][]byte, error) {
	var resp structs.ProduceInclusionListResponse
	if err := c.handler.Get(ctx, fmt.Sprintf("%s?slot=%d", inclusionListEndpoint, slot), &resp); err != nil {
		return nil, errors.Wrap(err, "could not get inclusion list")
	}
	txs := make([][]byte, len(resp.Data))
	for i, tx := range resp.Data {
		b, err := hexutil.Decode(tx)
		if err != nil {
			return nil, errors.Wrapf(err, "could not decode inclusion list transaction %d", i)
		}
		txs[i] = b
	}
	return txs, nil
}

func (c *beaconApiValidatorClient) submitInclusionList(ctx context.Context, signed *ethpb.SignedInclusionList) error {
	if signed == nil || signed.Message == nil {
		return errors.New("signed inclusion list is nil")
	}
	body, err := json.Marshal(&structs.PublishInclusionListRequest{Data: structs.SignedInclusionListFromConsensus(signed)})
	if err != nil {
		return errors.Wrap(err, "could not marshal signed inclusion list")
	}
	headers := map[string]string{api.VersionHeader: version.String(version.Heze)}
	return c.handler.Post(ctx, inclusionListEndpoint, headers, bytes.NewBuffer(body), nil)
}

func (c *beaconApiValidatorClient) inclusionListDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*iface.InclusionListDutiesResponse, error) {
	jsonIndices := make([]string, len(validatorIndices))
	for i, idx := range validatorIndices {
		jsonIndices[i] = strconv.FormatUint(uint64(idx), 10)
	}
	body, err := json.Marshal(jsonIndices)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal validator indices")
	}
	var resp structs.GetInclusionListDutiesResponse
	if err := c.handler.Post(ctx, fmt.Sprintf("/eth/v1/validator/duties/inclusion_list/%d", epoch), nil, bytes.NewBuffer(body), &resp); err != nil {
		return nil, errors.Wrap(err, "could not get inclusion list duties")
	}
	dependentRoot, err := hexutil.Decode(resp.DependentRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "could not decode dependent root %s", resp.DependentRoot)
	}
	duties := make([]*iface.InclusionListDuty, len(resp.Data))
	for i, d := range resp.Data {
		if d == nil {
			return nil, errors.Errorf("inclusion list duty %d is nil", i)
		}
		pubkey, err := hexutil.Decode(d.Pubkey)
		if err != nil {
			return nil, errors.Wrapf(err, "could not decode pubkey %s", d.Pubkey)
		}
		valIdx, err := strconv.ParseUint(d.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "could not parse validator index %s", d.ValidatorIndex)
		}
		slot, err := strconv.ParseUint(d.Slot, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "could not parse slot %s", d.Slot)
		}
		duties[i] = &iface.InclusionListDuty{
			Pubkey:         pubkey,
			ValidatorIndex: primitives.ValidatorIndex(valIdx),
			Slot:           primitives.Slot(slot),
		}
	}
	return &iface.InclusionListDutiesResponse{
		DependentRoot:       dependentRoot,
		ExecutionOptimistic: resp.ExecutionOptimistic,
		Duties:              duties,
	}, nil
}
