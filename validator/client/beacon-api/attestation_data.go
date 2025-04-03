package beacon_api

import (
	"context"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/apiutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (c *beaconApiValidatorClient) attestationData(
	ctx context.Context,
	reqSlot primitives.Slot,
	reqCommitteeIndex primitives.CommitteeIndex,
) (*ethpb.AttestationData, error) {
	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(reqSlot), 10))
	params.Add("committee_index", strconv.FormatUint(uint64(reqCommitteeIndex), 10))

	query := apiutil.BuildURL("/eth/v1/validator/attestation_data", params)
	produceAttestationDataResponseJson := structs.GetAttestationDataResponse{}

	if err := c.jsonRestHandler.Get(ctx, query, &produceAttestationDataResponseJson); err != nil {
		return nil, err
	}

	if produceAttestationDataResponseJson.Data == nil {
		return nil, errors.New("attestation data is nil")
	}

	attestationData := produceAttestationDataResponseJson.Data
	committeeIndex, err := strconv.ParseUint(attestationData.CommitteeIndex, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse attestation committee index: %s", attestationData.CommitteeIndex)
	}

	beaconBlockRoot, err := hexutil.Decode(attestationData.BeaconBlockRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode beacon block root: %s", attestationData.BeaconBlockRoot)
	}

	if !bytesutil.IsValidRoot(beaconBlockRoot) {
		return nil, errors.Errorf("invalid beacon block root: %s", attestationData.BeaconBlockRoot)
	}

	slot, err := strconv.ParseUint(attestationData.Slot, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse attestation slot: %s", attestationData.Slot)
	}

	if attestationData.Source == nil {
		return nil, errors.New("attestation source is nil")
	}

	sourceEpoch, err := strconv.ParseUint(attestationData.Source.Epoch, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse attestation source epoch: %s", attestationData.Source.Epoch)
	}

	sourceRoot, err := hexutil.Decode(attestationData.Source.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode attestation source root: %s", attestationData.Source.Root)
	}

	if !bytesutil.IsValidRoot(sourceRoot) {
		return nil, errors.Errorf("invalid attestation source root: %s", attestationData.Source.Root)
	}

	if attestationData.Target == nil {
		return nil, errors.New("attestation target is nil")
	}

	targetEpoch, err := strconv.ParseUint(attestationData.Target.Epoch, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse attestation target epoch: %s", attestationData.Target.Epoch)
	}

	targetRoot, err := hexutil.Decode(attestationData.Target.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode attestation target root: %s", attestationData.Target.Root)
	}

	if !bytesutil.IsValidRoot(targetRoot) {
		return nil, errors.Errorf("invalid attestation target root: %s", attestationData.Target.Root)
	}

	response := &ethpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot,
		CommitteeIndex:  primitives.CommitteeIndex(committeeIndex),
		Slot:            primitives.Slot(slot),
		Source: &ethpb.Checkpoint{
			Epoch: primitives.Epoch(sourceEpoch),
			Root:  sourceRoot,
		},
		Target: &ethpb.Checkpoint{
			Epoch: primitives.Epoch(targetEpoch),
			Root:  targetRoot,
		},
	}

	return response, nil
}
