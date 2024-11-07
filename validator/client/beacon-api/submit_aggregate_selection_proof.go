package beacon_api

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (c *beaconApiValidatorClient) submitAggregateSelectionProof(
	ctx context.Context,
	in *ethpb.AggregateSelectionRequest,
	index primitives.ValidatorIndex,
	committeeLength uint64,
) (*ethpb.AggregateSelectionResponse, error) {
	isOptimistic, err := c.isOptimistic(ctx)
	if err != nil {
		return nil, err
	}

	// An optimistic validator MUST NOT participate in attestation. (i.e., sign across the DOMAIN_BEACON_ATTESTER, DOMAIN_SELECTION_PROOF or DOMAIN_AGGREGATE_AND_PROOF domains).
	if isOptimistic {
		return nil, errors.New("the node is currently optimistic and cannot serve validators")
	}

	isAggregator, err := helpers.IsAggregator(committeeLength, in.SlotSignature)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get aggregator status")
	}
	if !isAggregator {
		return nil, errors.New("validator is not an aggregator")
	}

	attestationData, err := c.attestationData(ctx, in.Slot, in.CommitteeIndex)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get attestation data for slot=%d and committee_index=%d", in.Slot, in.CommitteeIndex)
	}

	attestationDataRoot, err := attestationData.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate attestation data root")
	}

	aggregateAttestationResponse, err := c.aggregateAttestation(ctx, in.Slot, attestationDataRoot[:])
	if err != nil {
		return nil, err
	}

	var attData *structs.Attestation
	if err := json.Unmarshal(aggregateAttestationResponse.Data, &attData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal aggregate attestation data")
	}

	aggregatedAttestation, err := convertAttestationToProto(attData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert aggregate attestation json to proto")
	}

	return &ethpb.AggregateSelectionResponse{
		AggregateAndProof: &ethpb.AggregateAttestationAndProof{
			AggregatorIndex: index,
			Aggregate:       aggregatedAttestation,
			SelectionProof:  in.SlotSignature,
		},
	}, nil
}

func (c *beaconApiValidatorClient) aggregateAttestation(
	ctx context.Context,
	slot primitives.Slot,
	attestationDataRoot []byte,
) (*structs.AggregateAttestationResponse, error) {
	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(slot), 10))
	params.Add("attestation_data_root", hexutil.Encode(attestationDataRoot))
	endpoint := buildURL("/eth/v1/validator/aggregate_attestation", params)

	var aggregateAttestationResponse structs.AggregateAttestationResponse
	if err := c.jsonRestHandler.Get(ctx, endpoint, &aggregateAttestationResponse); err != nil {
		return nil, err
	}

	return &aggregateAttestationResponse, nil
}
