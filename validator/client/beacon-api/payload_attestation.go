package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const payloadAttestationsEndpoint = "/eth/v1/beacon/pool/payload_attestations"

func (c *beaconApiValidatorClient) payloadAttestationData(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, error) {
	endpoint := fmt.Sprintf("/eth/v1/validator/payload_attestation_data?slot=%d", slot)
	// Prefer SSZ; GetSSZ negotiates and the server may answer JSON, which we decode below.
	// Freshness options steer the read toward a node that already imported the announced head.
	data, header, err := c.handler.GetSSZ(ctx, endpoint, payloadAttestationFreshnessOptions(ctx)...)
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution payload attestation data")
	}
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		d := &ethpb.PayloadAttestationData{}
		if err := d.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal ssz payload attestation data")
		}
		return d, nil
	}
	var resp structs.GetPayloadAttestationDataResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, errors.Wrap(err, "could not decode payload attestation data")
	}
	if resp.Data == nil {
		return nil, errors.New("payload attestation data is nil")
	}
	return resp.Data.ToConsensus()
}

func (c *beaconApiValidatorClient) submitPayloadAttestation(ctx context.Context, msg *ethpb.PayloadAttestationMessage) error {
	if msg == nil || msg.Data == nil {
		return errors.New("payload attestation message is nil")
	}
	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	jsonFn := func() ([]byte, error) {
		return json.Marshal([]*structs.PayloadAttestationMessage{structs.PayloadAttestationMessageFromConsensus(msg)})
	}

	return c.handler.PostSSZWithFallback(
		ctx,
		payloadAttestationsEndpoint,
		headers,
		msg.MarshalSSZ,
		jsonFn,
	)
}
