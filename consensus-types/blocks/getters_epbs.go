package blocks

import (
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
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
