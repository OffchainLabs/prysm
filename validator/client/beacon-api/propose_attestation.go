package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

func (c *beaconApiValidatorClient) proposeAttestation(ctx context.Context, attestation *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	if err := validateNilAttestation(attestation); err != nil {
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
	errJson := &httputil.DefaultJsonError{}
	if err != nil {
		// TODO: remove this when v2 becomes default
		if !errors.As(err, &errJson) {
			return nil, err
		}
		if errJson.Code != http.StatusNotFound {
			return nil, errJson
		}
		log.Debug("Endpoint /eth/v2/beacon/pool/attestations is not supported, falling back to older endpoints for submit attestation.")
		if err = c.jsonRestHandler.Post(
			ctx,
			"/eth/v1/beacon/pool/attestations",
			nil,
			bytes.NewBuffer(marshalledAttestation),
			nil,
		); err != nil {
			return nil, err
		}
	}

	attestationDataRoot, err := attestation.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

func (c *beaconApiValidatorClient) proposeAttestationElectra(ctx context.Context, attestation *ethpb.AttestationElectra) (*ethpb.AttestResponse, error) {
	if err := validateNilAttestation(attestation); err != nil {
		return nil, err
	}
	marshalledAttestation, err := json.Marshal(jsonifyAttestationsElectra([]*ethpb.AttestationElectra{attestation}))
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Eth-Consensus-Version": version.String(attestation.Version())}
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

func validateNilAttestation(attestation ethpb.Att) error {
	if attestation == nil || attestation.IsNil() {
		return errors.New("attestation can't be nil")
	}
	if attestation.GetData().Source == nil {
		return errors.New("attestation's source can't be nil")
	}
	if attestation.GetData().Target == nil {
		return errors.New("attestation's target can't be nil")
	}
	v := attestation.Version()
	if len(attestation.GetAggregationBits()) == 0 {
		return errors.New("attestation's bitfield can't be nil")
	}
	if len(attestation.GetSignature()) == 0 {
		return errors.New("attestation signature can't be nil")
	}
	if v >= version.Electra {
		if len(attestation.CommitteeBitsVal().BitIndices()) == 0 {
			return errors.New("attestation committee bits can't be nil")
		}
	}
	return nil
}
