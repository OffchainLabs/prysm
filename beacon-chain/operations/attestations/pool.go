package attestations

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/attestations/kv"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// Pool defines the necessary methods for Prysm attestations pool to serve
// fork choice and validators. In the current design, aggregated attestations
// are used by proposer actor. Unaggregated attestations are used by
// aggregator actor.
type Pool interface {
	// For Aggregated attestations
	AggregateUnaggregatedAttestations(ctx context.Context) error
	SaveAggregatedAttestation(att ethpb.Att) error
	SaveAggregatedAttestations(atts []ethpb.Att) error
	AggregatedAttestations() []ethpb.Att
	AggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*ethpb.Attestation
	AggregatedAttestationsBySlotIndexElectra(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*ethpb.AttestationElectra
	DeleteAggregatedAttestation(att ethpb.Att) error
	HasAggregatedAttestation(att ethpb.Att) (bool, error)
	AggregatedAttestationCount() int
	// For unaggregated attestations.
	SaveUnaggregatedAttestation(att ethpb.Att) error
	SaveUnaggregatedAttestations(atts []ethpb.Att) error
	UnaggregatedAttestations() ([]ethpb.Att, error)
	UnaggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*ethpb.Attestation
	UnaggregatedAttestationsBySlotIndexElectra(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*ethpb.AttestationElectra
	DeleteUnaggregatedAttestation(att ethpb.Att) error
	DeleteSeenUnaggregatedAttestations() (int, error)
	UnaggregatedAttestationCount() int
	// For attestations that were included in the block.
	SaveBlockAttestation(att ethpb.Att) error
	BlockAttestations() []ethpb.Att
	DeleteBlockAttestation(att ethpb.Att) error
	// For attestations to be passed to fork choice.
	SaveForkchoiceAttestations(atts []ethpb.Att) error
	ForkchoiceAttestations() []ethpb.Att
	DeleteForkchoiceAttestation(att ethpb.Att) error
	ForkchoiceAttestationCount() int
}

// NewPool initializes a new attestation pool.
func NewPool() *kv.AttCaches {
	return kv.NewAttCaches()
}
