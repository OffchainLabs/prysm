package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) submitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	body, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(in.SignedAggregateAndProof)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal SignedAggregateAttestationAndProof")
	}
	headers := map[string]string{"Eth-Consensus-Version": version.String(in.SignedAggregateAndProof.Version())}
	err = c.jsonRestHandler.Post(ctx, "/eth/v2/validator/aggregate_and_proofs", headers, bytes.NewBuffer(body), nil)
	if err != nil {
		return nil, err
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
	dataSlot := in.SignedAggregateAndProof.Message.Aggregate.Data.Slot
	consensusVersion := version.String(slots.ToForkVersion(dataSlot))
	headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
	if err = c.jsonRestHandler.Post(ctx, "/eth/v2/validator/aggregate_and_proofs", headers, bytes.NewBuffer(body), nil); err != nil {
		return nil, err
	}

	attestationDataRoot, err := in.SignedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}
