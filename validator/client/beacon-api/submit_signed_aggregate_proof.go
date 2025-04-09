package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

func (c *beaconApiValidatorClient) submitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	body, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(in.SignedAggregateAndProof)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal SignedAggregateAttestationAndProof")
	}
	headers := map[string]string{"Eth-Consensus-Version": version.String(in.SignedAggregateAndProof.Version())}
	err = c.jsonRestHandler.Post(ctx, "/eth/v2/validator/aggregate_and_proofs", headers, bytes.NewBuffer(body), nil)
	errJson := &httputil.DefaultJsonError{}
	if err != nil {
		// TODO: remove this when v2 becomes default
		if !errors.As(err, &errJson) {
			return nil, err
		}
		if errJson.Code != http.StatusNotFound {
			return nil, errJson
		}
		log.Debug("Endpoint /eth/v2/validator/aggregate_and_proofs is not supported, falling back to older endpoints for publish aggregate and proofs.")
		if err = c.jsonRestHandler.Post(
			ctx,
			"/eth/v1/validator/aggregate_and_proofs",
			nil,
			bytes.NewBuffer(body),
			nil,
		); err != nil {
			return nil, err
		}
	}

	attestationDataRoot, err := in.SignedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

func (c *beaconApiValidatorClient) submitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	body, err := json.Marshal([]*structs.SignedAggregateAttestationAndProofElectra{jsonifySignedAggregateAndProofElectra(in.SignedAggregateAndProof)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal SignedAggregateAttestationAndProofElectra")
	}
	headers := map[string]string{"Eth-Consensus-Version": version.String(in.SignedAggregateAndProof.Version())}
	if err = c.jsonRestHandler.Post(ctx, "/eth/v2/validator/aggregate_and_proofs", headers, bytes.NewBuffer(body), nil); err != nil {
		return nil, err
	}

	attestationDataRoot, err := in.SignedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}
