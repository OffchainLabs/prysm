package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) payloadAttestationData(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, error) {
	endpoint := fmt.Sprintf("/eth/v1/validator/payload_attestation_data/%d", slot)
	var resp structs.GetPayloadAttestationDataResponse
	if err := c.handler.Get(ctx, endpoint, &resp); err != nil {
		return nil, errors.Wrap(err, "could not get execution payload attestation data")
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
	jsonMsg := structs.PayloadAttestationMessageFromConsensus(msg)
	body, err := json.Marshal([]*structs.PayloadAttestationMessage{jsonMsg})
	if err != nil {
		return errors.Wrap(err, "failed to marshal payload attestation message")
	}
	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}
	return c.handler.Post(ctx, "/eth/v1/beacon/pool/payload_attestations", headers, bytes.NewBuffer(body), nil)
}
