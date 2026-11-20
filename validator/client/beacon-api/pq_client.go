package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	pqAttestationsEndpoint        = "/eth/v2/beacon/pool/attestations"
	pqAggregateEndpoint           = "/eth/v2/validator/aggregate_attestation"
	pqAggregateAndProofsEndpoint  = "/eth/v2/validator/aggregate_and_proofs"
	pqProduceBlockEndpoint        = "/eth/v4/validator/blocks/%d"
	pqPublishBlockEndpoint        = "/eth/v2/beacon/blocks"
	pqPublishEnvelopeEndpoint     = "/eth/v1/beacon/execution_payload_envelopes?broadcast_validation=consensus"
	pqPayloadAttestationsEndpoint = "/eth/v1/beacon/pool/payload_attestations"

	// pqEmptyBuilderConfig is the body of a block production request. The beacon node ignores it in the PQ fork.
	pqEmptyBuilderConfig = `{"min_bid":"0","builder_boost_factor":"100","builders":[]}`
)

var _ iface.PQValidatorClient = (*beaconApiValidatorClient)(nil)

type (
	// pqAggregateResponse is the response of the aggregate attestation endpoint in the PQ fork.
	pqAggregateResponse struct {
		Version string               `json:"version"`
		Data    *structs.AggregatePQ `json:"data"`
	}

	// pqProduceBlockResponse is the response of the block production endpoint in the PQ fork.
	pqProduceBlockResponse struct {
		Version string                   `json:"version"`
		Data    *structs.BlockContentsPQ `json:"data"`
	}
)

// pqHeaders returns the headers of a request in the PQ fork.
func pqHeaders() map[string]string {
	return map[string]string{api.VersionHeader: version.String(version.PQ)}
}

// IsAggregatorPQ returns whether the beacon node selected the validator of index `index` as an aggregator at `slot`,
// in its attester duties.
func (c *beaconApiValidatorClient) IsAggregatorPQ(slot primitives.Slot, index primitives.ValidatorIndex) bool {
	if c.pqAggregators == nil {
		return false
	}

	return c.pqAggregators.isAggregator(slot, index)
}

// SubmitAttestationsPQ submits `attestations` to the attestation pool of the beacon node.
func (c *beaconApiValidatorClient) SubmitAttestationsPQ(ctx context.Context, attestations []*ethpb.SingleAttestationPQ) error {
	jsonAttestations := make([]*structs.SingleAttestation, 0, len(attestations))
	for _, attestation := range attestations {
		jsonAttestations = append(jsonAttestations, structs.SingleAttPQFromConsensus(attestation))
	}

	body, err := json.Marshal(jsonAttestations)
	if err != nil {
		return fmt.Errorf("marshal attestations: %w", err)
	}

	if err := c.handler.Post(ctx, pqAttestationsEndpoint, pqHeaders(), bytes.NewBuffer(body), nil); err != nil {
		return fmt.Errorf("post attestations: %w", err)
	}

	return nil
}

// AggregatePQ returns the aggregate of the attestations of `attestationDataRoot` in the committee of index
// `committeeIndex` at `slot`, and its proof, both built by the beacon node.
func (c *beaconApiValidatorClient) AggregatePQ(
	ctx context.Context,
	slot primitives.Slot,
	attestationDataRoot [32]byte,
	committeeIndex primitives.CommitteeIndex,
) (*ethpb.AttestationPQ, []byte, error) {
	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(slot), 10))
	params.Add("attestation_data_root", hexutil.Encode(attestationDataRoot[:]))
	params.Add("committee_index", strconv.FormatUint(uint64(committeeIndex), 10))

	resp := &pqAggregateResponse{}
	if err := c.handler.Get(ctx, pqAggregateEndpoint+"?"+params.Encode(), resp); err != nil {
		return nil, nil, fmt.Errorf("get aggregate: %w", err)
	}

	if resp.Version != version.String(version.PQ) {
		return nil, nil, fmt.Errorf("aggregate of version %q instead of %q", resp.Version, version.String(version.PQ))
	}

	aggregate, proof, err := resp.Data.ToConsensus()
	if err != nil {
		return nil, nil, fmt.Errorf("decode aggregate: %w", err)
	}

	return aggregate, proof, nil
}

// SubmitAggregatesPQ submits the signed aggregates and proofs `aggregates` to the beacon node.
func (c *beaconApiValidatorClient) SubmitAggregatesPQ(ctx context.Context, aggregates []*ethpb.SignedAggregateAttestationAndProofPQ) error {
	jsonAggregates := make([]*structs.SignedAggregateAttestationAndProofPQ, 0, len(aggregates))
	for _, aggregate := range aggregates {
		jsonAggregates = append(jsonAggregates, structs.SignedAggregateAttAndProofPQFromConsensus(aggregate))
	}

	body, err := json.Marshal(jsonAggregates)
	if err != nil {
		return fmt.Errorf("marshal aggregates: %w", err)
	}

	if err := c.handler.Post(ctx, pqAggregateAndProofsEndpoint, pqHeaders(), bytes.NewBuffer(body), nil); err != nil {
		return fmt.Errorf("post aggregates: %w", err)
	}

	return nil
}

// ProduceBlockPQ returns the block at `slot` the beacon node produces with the VRF proof `randaoReveal` and
// `graffiti`, with its execution payload envelope, blobs and KZG proofs.
func (c *beaconApiValidatorClient) ProduceBlockPQ(
	ctx context.Context,
	slot primitives.Slot,
	randaoReveal []byte,
	graffiti []byte,
) (*iface.PQBlockContents, error) {
	params := url.Values{}
	params.Add("randao_reveal", hexutil.Encode(randaoReveal))
	params.Add("include_payload", "true")
	if len(graffiti) > 0 {
		params.Add("graffiti", hexutil.Encode(graffiti))
	}

	endpoint := fmt.Sprintf(pqProduceBlockEndpoint, slot) + "?" + params.Encode()

	headers := pqHeaders()
	headers["Accept"] = api.JsonMediaType

	resp := &pqProduceBlockResponse{}
	if err := c.handler.Post(ctx, endpoint, headers, bytes.NewBufferString(pqEmptyBuilderConfig), resp); err != nil {
		return nil, fmt.Errorf("produce block: %w", err)
	}

	if resp.Version != version.String(version.PQ) {
		return nil, fmt.Errorf("block of version %q instead of %q", resp.Version, version.String(version.PQ))
	}

	block, envelope, kzgProofs, blobs, err := resp.Data.ToConsensus()
	if err != nil {
		return nil, fmt.Errorf("decode block contents: %w", err)
	}

	return &iface.PQBlockContents{Block: block, Envelope: envelope, KzgProofs: kzgProofs, Blobs: blobs}, nil
}

// PublishBlockPQ publishes the signed block `block` through the beacon node.
func (c *beaconApiValidatorClient) PublishBlockPQ(ctx context.Context, block *ethpb.SignedBeaconBlockPQ) error {
	body, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}

	if err := c.handler.PostSSZ(ctx, pqPublishBlockEndpoint, pqHeaders(), bytes.NewBuffer(body)); err != nil {
		return fmt.Errorf("post block: %w", err)
	}

	return nil
}

// PublishEnvelopePQ publishes the signed execution payload envelope `envelope`, with its blobs `blobs` and KZG
// proofs `kzgProofs`, through the beacon node.
func (c *beaconApiValidatorClient) PublishEnvelopePQ(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelopePQ,
	kzgProofs, blobs [][]byte,
) error {
	contents, err := structs.SignedExecutionPayloadEnvelopeContentsPQFromConsensus(envelope, kzgProofs, blobs)
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}

	body, err := json.Marshal(contents)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// The blobs are always included, even if there are none.
	headers := pqHeaders()
	headers[api.BlobDataIncludedHeader] = "true"

	if err := c.handler.Post(ctx, pqPublishEnvelopeEndpoint, headers, bytes.NewBuffer(body), nil); err != nil {
		return fmt.Errorf("post envelope: %w", err)
	}

	return nil
}

// SubmitPayloadAttestationsPQ submits the payload attestations `messages` to the beacon node.
func (c *beaconApiValidatorClient) SubmitPayloadAttestationsPQ(ctx context.Context, messages []*ethpb.PayloadAttestationMessagePQ) error {
	jsonMessages := make([]*structs.PayloadAttestationMessage, 0, len(messages))
	for _, message := range messages {
		jsonMessages = append(jsonMessages, structs.PayloadAttestationMessagePQFromConsensus(message))
	}

	body, err := json.Marshal(jsonMessages)
	if err != nil {
		return fmt.Errorf("marshal payload attestations: %w", err)
	}

	if err := c.handler.Post(ctx, pqPayloadAttestationsEndpoint, pqHeaders(), bytes.NewBuffer(body), nil); err != nil {
		return fmt.Errorf("post payload attestations: %w", err)
	}

	return nil
}

// SubmitProposerPreferencesPQ submits the signed proposer preferences `preferences` to the beacon node.
func (c *beaconApiValidatorClient) SubmitProposerPreferencesPQ(ctx context.Context, preferences []*ethpb.SignedProposerPreferencesPQ) error {
	jsonPreferences := make([]*structs.SignedProposerPreferences, 0, len(preferences))
	for _, preference := range preferences {
		jsonPreferences = append(jsonPreferences, structs.SignedProposerPreferencesPQFromConsensus(preference))
	}

	body, err := json.Marshal(jsonPreferences)
	if err != nil {
		return fmt.Errorf("marshal proposer preferences: %w", err)
	}

	if err := c.handler.Post(ctx, proposerPreferencesEndpoint, pqHeaders(), bytes.NewBuffer(body), nil); err != nil {
		return fmt.Errorf("post proposer preferences: %w", err)
	}

	return nil
}
