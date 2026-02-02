package payloadattestation

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolManager maintains pending payload attestations.
// This pool is used by proposers to insert payload attestations into new blocks.
type PoolManager interface {
	PendingPayloadAttestations() []*ethpb.PayloadAttestation
	InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage) error
	MarkIncluded(att *ethpb.PayloadAttestation)
}

// PayloadStatusFetcher determines the payload presence and blob data availability
// for a given slot. This is used by PTC validators to produce PayloadAttestationData.
type PayloadStatusFetcher interface {
	PayloadStatus(slot primitives.Slot) (payloadPresent bool, blobDataAvailable bool, err error)
}
