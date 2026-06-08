//go:build !minimal

package testing

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// MakeSyncContributionsFromBitVector creates list of sync contributions from list of bitvector.
func MakeSyncContributionsFromBitVector(bl []bitfield.Bitvector128) []*ethpb.SyncCommitteeContribution {
	c := make([]*ethpb.SyncCommitteeContribution, len(bl))
	for i, b := range bl {
		c[i] = &ethpb.SyncCommitteeContribution{
			Slot:              primitives.Slot(1),
			SubcommitteeIndex: 2,
			AggregationBits:   b,
			Signature:         bls.NewAggregateSignature().Marshal(),
		}
	}
	return c
}
