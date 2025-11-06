package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) proposeAttestation(ctx context.Context, attestation *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	if err := helpers.ValidateNilAttestation(attestation); err != nil {
		return nil, err
	}
	marshalledAttestation, err := json.Marshal(jsonifyAttestations([]*ethpb.Attestation{attestation}))
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"Eth-Consensus-Version": version.String(attestation.Version())}
	err = c.jsonRestHandler.Post(
		ctx,
		"/eth/v2/beacon/pool/attestations",
		headers,
		bytes.NewBuffer(marshalledAttestation),
		nil,
	)
	if err != nil {
		return nil, err
	}

	attestationDataRoot, err := attestation.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

func (c *beaconApiValidatorClient) proposeAttestationElectra(ctx context.Context, attestation *ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	if err := helpers.ValidateNilAttestation(attestation); err != nil {
		return nil, err
	}
	marshalledAttestation, err := json.Marshal(jsonifySingleAttestations([]*ethpb.SingleAttestation{attestation}))
	if err != nil {
		return nil, err
	}
	consensusVersion := version.String(slots.ToForkVersion(attestation.Data.Slot))
	headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
	if err = c.jsonRestHandler.Post(
		ctx,
		"/eth/v2/beacon/pool/attestations",
		headers,
		bytes.NewBuffer(marshalledAttestation),
		nil,
	); err != nil {
		return nil, err
	}

	attestationDataRoot, err := attestation.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}
