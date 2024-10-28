package blocks

import (
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SetPayloadAttestations sets the payload attestations in the block.
func (b *SignedBeaconBlock) SetPayloadAttestations(p []*eth.PayloadAttestation) error {
	if b.version < version.EPBS {
		return consensus_types.ErrNotSupported("PayloadAttestations", b.version)
	}
	b.block.body.payloadAttestations = p
	return nil
}

// SetSignedExecutionPayloadHeader sets the signed execution payload header of the block body.
func (b *SignedBeaconBlock) SetSignedExecutionPayloadHeader(h *enginev1.SignedExecutionPayloadHeader) error {
	if b.version < version.EPBS {
		return consensus_types.ErrNotSupported("SetSignedExecutionPayloadHeader", b.version)
	}
	b.block.body.signedExecutionPayloadHeader = h
	return nil
}
