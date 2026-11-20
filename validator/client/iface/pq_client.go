package iface

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type (
	// PQBlockContents is a block of the PQ fork to sign, with the envelope of its payload and the data of its blobs.
	PQBlockContents struct {
		Block     *ethpb.BeaconBlockPQ
		Envelope  *ethpb.ExecutionPayloadEnvelopePQ
		KzgProofs [][]byte
		Blobs     [][]byte
	}

	// PQValidatorClient is the part of the validator client specific to the PQ fork.
	// Only the REST validator client implements it, when it is built with `WithPQKeys`.
	PQValidatorClient interface {
		// IsAggregatorPQ returns whether the beacon node selected the validator of index `index` as an aggregator
		// at `slot`, in its attester duties. In the PQ fork, there is no selection proof.
		IsAggregatorPQ(slot primitives.Slot, index primitives.ValidatorIndex) bool

		// SubmitAttestationsPQ submits signed attestations.
		SubmitAttestationsPQ(ctx context.Context, attestations []*ethpb.SingleAttestationPQ) error

		// AggregatePQ returns the aggregate of the attestations of committee `committeeIndex` at `slot` whose data root
		// is `attestationDataRoot`, with its proof.
		AggregatePQ(
			ctx context.Context,
			slot primitives.Slot,
			attestationDataRoot [32]byte,
			committeeIndex primitives.CommitteeIndex,
		) (*ethpb.AttestationPQ, []byte, error)

		// SubmitAggregatesPQ submits signed aggregates and proofs.
		SubmitAggregatesPQ(ctx context.Context, aggregates []*ethpb.SignedAggregateAttestationAndProofPQ) error

		// ProduceBlockPQ returns the block to propose at `slot`, with the envelope of its payload.
		// `randaoReveal` is the SSZ encoded VRF proof of `slot`.
		ProduceBlockPQ(ctx context.Context, slot primitives.Slot, randaoReveal []byte, graffiti []byte) (*PQBlockContents, error)

		// PublishBlockPQ publishes a signed block.
		PublishBlockPQ(ctx context.Context, block *ethpb.SignedBeaconBlockPQ) error

		// PublishEnvelopePQ publishes the signed envelope of the payload of a block, with the data of its blobs.
		PublishEnvelopePQ(ctx context.Context, envelope *ethpb.SignedExecutionPayloadEnvelopePQ, kzgProofs, blobs [][]byte) error

		// SubmitPayloadAttestationsPQ submits signed payload attestations.
		SubmitPayloadAttestationsPQ(ctx context.Context, messages []*ethpb.PayloadAttestationMessagePQ) error

		// SubmitProposerPreferencesPQ submits signed proposer preferences.
		SubmitProposerPreferencesPQ(ctx context.Context, preferences []*ethpb.SignedProposerPreferencesPQ) error
	}
)
