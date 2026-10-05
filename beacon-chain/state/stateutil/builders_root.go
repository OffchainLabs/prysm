package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// BuildersRoot computes the SSZ root of a slice of Builder.
func BuildersRoot(stateVersion int, slice []*ethpb.Builder) ([32]byte, error) {
	if stateVersion >= version.Gloas {
		return buildersRootProgressive(slice)
	}
	return ssz.SliceRoot(slice, uint64(fieldparams.BuilderRegistryLimit))
}

// buildersRootProgressive computes the progressive SSZ root of a slice of Builder.
func buildersRootProgressive(slice []*ethpb.Builder) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
