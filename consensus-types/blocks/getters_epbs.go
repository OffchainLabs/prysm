package blocks

import (
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
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
func (b *BeaconBlockBody) SignedExecutionPayloadHeader() (*enginev1.SignedExecutionPayloadHeader, error) {
	if b.version < version.EPBS {
		return nil, consensus_types.ErrNotSupported("SignedExecutionPayloadHeader", b.version)
	}
	return b.signedExecutionPayloadHeader, nil
}
