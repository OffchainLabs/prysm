package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuildersRoot computes the SSZ root of a slice of Builder.
func BuildersRoot(slice []*ethpb.Builder) ([32]byte, error) {
	return ssz.SliceRoot(slice, uint64(fieldparams.BuilderRegistryLimit))
}
