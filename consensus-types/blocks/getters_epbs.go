package blocks

import (
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// PayloadAttestations returns the payload attestations in the block.
func (b *BeaconBlockBody) PayloadAttestations() ([]*ethpb.PayloadAttestation, error) {
	if b.version < version.EPBS {
		return nil, consensus_types.ErrNotSupported("PayloadAttestations", b.version)
	}
	return b.payloadAttestations, nil
}

// SignedExecutionPayloadHeader returns the signed execution payload header in the block.
func (b *BeaconBlockBody) SignedExecutionPayloadHeader() (interfaces.ROSignedExecutionPayloadHeader, error) {
	if b.version < version.EPBS {
		return nil, consensus_types.ErrNotSupported("SignedExecutionPayloadHeader", b.version)
	}
	return WrappedROSignedExecutionPayloadHeader(b.signedExecutionPayloadHeader)
}
