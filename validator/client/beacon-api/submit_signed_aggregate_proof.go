package beacon_api

import (
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

const aggregateAndProofsEndpoint = "/eth/v2/validator/aggregate_and_proofs"

func (c *beaconApiValidatorClient) submitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	headers := map[string]string{"Eth-Consensus-Version": version.String(in.SignedAggregateAndProof.Version())}
	jsonFn := func() ([]byte, error) {
		return json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(in.SignedAggregateAndProof)})
	}
	sszFn := func() ([]byte, error) {
		elem, err := in.SignedAggregateAndProof.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		return ssz.MarshalVariableList(elem), nil
	}

	if err := c.handler.PostSSZWithFallback(ctx, aggregateAndProofsEndpoint, headers, sszFn, jsonFn); err != nil {
		return nil, err
	}

	attestationDataRoot, err := in.SignedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

func (c *beaconApiValidatorClient) submitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	dataSlot := in.SignedAggregateAndProof.Message.Aggregate.Data.Slot
	consensusVersion := version.String(slots.ToForkVersion(dataSlot))
	headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
	jsonFn := func() ([]byte, error) {
		return json.Marshal([]*structs.SignedAggregateAttestationAndProofElectra{jsonifySignedAggregateAndProofElectra(in.SignedAggregateAndProof)})
	}
	sszFn := func() ([]byte, error) {
		elem, err := in.SignedAggregateAndProof.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		return ssz.MarshalVariableList(elem), nil
	}

	if err := c.handler.PostSSZWithFallback(ctx, aggregateAndProofsEndpoint, headers, sszFn, jsonFn); err != nil {
		return nil, err
	}

	attestationDataRoot, err := in.SignedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}
