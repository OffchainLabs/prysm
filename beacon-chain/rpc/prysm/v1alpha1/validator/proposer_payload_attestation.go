package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// getPayloadAttestations returns payload attestations for inclusion in a Gloas block.
// PTC members broadcast PayloadAttestationMessages via P2P gossip during slot N.
// All nodes collect these in a pool. The slot N+1 proposer retrieves and aggregates
// them into PayloadAttestations for block inclusion.
func (vs *Server) getPayloadAttestations(ctx context.Context, head state.BeaconState) []*ethpb.PayloadAttestation {
	if slots.ToEpoch(head.Slot()) < params.BeaconConfig().GloasForkEpoch {
		return nil
	}
	// TODO: Retrieve and aggregate PayloadAttestationMessages from the pool
	// for the previous slot. Blocks are valid without payload attestations.
	return []*ethpb.PayloadAttestation{}
}
