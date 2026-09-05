package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// proposeAttestation submits pre-signed attestations of the same fork version in one request.
func (c *beaconApiValidatorClient) proposeAttestation(ctx context.Context, attestations []*ethpb.Attestation) (*ethpb.AttestResponse, error) {
	if len(attestations) == 0 {
		return nil, nil
	}
	for i, att := range attestations {
		if err := helpers.ValidateNilAttestation(att); err != nil {
			return nil, fmt.Errorf("attestation at index %d is invalid: %w", i, err)
		}
	}
	marshalledAttestations, err := json.Marshal(jsonifyAttestations(attestations))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attestations: %w", err)
	}
	headers := map[string]string{"Eth-Consensus-Version": version.String(attestations[0].Version())}
	if err := c.handler.Post(ctx, "/eth/v2/beacon/pool/attestations", headers, bytes.NewBuffer(marshalledAttestations), nil); err != nil {
		return nil, parseIndexedError(err)
	}
	attestationDataRoot, err := attestations[0].Data.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to compute attestation data root: %w", err)
	}
	return &ethpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

// proposeAttestationElectra submits pre-signed single attestations of the same slot in one
// request, as an SSZ list of fixed-size items, falling back to JSON on a 415 response.
func (c *beaconApiValidatorClient) proposeAttestationElectra(ctx context.Context, attestations []*ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	if len(attestations) == 0 {
		return nil, nil
	}
	for i, att := range attestations {
		if err := helpers.ValidateNilAttestation(att); err != nil {
			return nil, fmt.Errorf("attestation at index %d is invalid: %w", i, err)
		}
	}
	attestation := attestations[0]
	headers := map[string]string{"Eth-Consensus-Version": version.String(slots.ToForkVersion(attestation.Data.Slot))}

	sszFn := func() ([]byte, error) {
		sszBytes := make([]byte, 0, attestation.SizeSSZ()*len(attestations))
		for _, att := range attestations {
			var err error
			sszBytes, err = att.MarshalSSZTo(sszBytes)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal attestations to SSZ: %w", err)
			}
		}
		return sszBytes, nil
	}
	jsonFn := func() ([]byte, error) {
		b, err := json.Marshal(jsonifySingleAttestations(attestations))
		if err != nil {
			return nil, fmt.Errorf("failed to marshal attestations: %w", err)
		}
		return b, nil
	}
	if err := c.handler.PostSSZWithFallback(ctx, "/eth/v2/beacon/pool/attestations", headers, sszFn, jsonFn); err != nil {
		return nil, parseIndexedError(err)
	}
	attestationDataRoot, err := attestation.Data.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to compute attestation data root: %w", err)
	}
	return &ethpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

func parseIndexedError(err error) error {
	if err == nil {
		return nil
	}
	var indexedErr server.IndexedErrorContainer
	var defaultJsonErr *httputil.DefaultJsonError
	if errors.As(err, &defaultJsonErr) {
		if json.Unmarshal([]byte(defaultJsonErr.Message), &indexedErr) == nil && len(indexedErr.Failures) > 0 {
			return &indexedErr
		}
	}
	if json.Unmarshal([]byte(err.Error()), &indexedErr) == nil && len(indexedErr.Failures) > 0 {
		return &indexedErr
	}
	return err
}
