package shared

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PQValidator is the PQ logic the REST handlers need that the gRPC validator service does not expose.
type PQValidator interface {
	ExecutionPayloadContentsPQ(slot primitives.Slot) (envelope *eth.ExecutionPayloadEnvelopePQ, kzgProofs, blobs [][]byte, err error)
	PublishExecutionPayloadEnvelopeContentsPQ(ctx context.Context, signed *eth.SignedExecutionPayloadEnvelopePQ, kzgProofs, blobs [][]byte) error
	RegisterAggregationDutyPQ(slot primitives.Slot, index primitives.CommitteeIndex, validator primitives.ValidatorIndex)
	AggregateAttestationPQ(ctx context.Context, slot primitives.Slot, index primitives.CommitteeIndex, dataRoot [32]byte) (*eth.AttestationPQ, []byte, error)
}
