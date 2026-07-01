package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const payloadAttestationsEndpoint = "/eth/v1/beacon/pool/payload_attestations"

func (c *beaconApiValidatorClient) payloadAttestationData(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, error) {
	endpoint := fmt.Sprintf("/eth/v1/validator/payload_attestation_data/%d", slot)
	// Prefer SSZ; GetSSZ negotiates and the server may answer JSON, which we decode below.
	data, header, err := c.handler.GetSSZ(ctx, endpoint)
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

	// Prefer SSZ; fall back to JSON if the beacon node does not accept octet-stream request bodies.
	// The SSZ body is the List[PayloadAttestationMessage] encoding, here a single fixed-size element.
	sszBody, err := msg.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "failed to marshal payload attestation message ssz")
	}
	if _, _, err = c.handler.PostSSZ(ctx, payloadAttestationsEndpoint, headers, bytes.NewBuffer(sszBody)); err == nil {
		return nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) || errJson.Code != http.StatusUnsupportedMediaType {
		return err
	}
	log.WithError(err).Warn("Beacon node does not accept SSZ payload attestations, falling back to JSON")

	jsonBody, err := json.Marshal([]*structs.PayloadAttestationMessage{structs.PayloadAttestationMessageFromConsensus(msg)})
	if err != nil {
		return errors.Wrap(err, "failed to marshal payload attestation message")
	}
	return c.handler.Post(ctx, payloadAttestationsEndpoint, headers, bytes.NewBuffer(jsonBody), nil)
}
