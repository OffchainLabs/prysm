package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuildersRoot computes the SSZ root of a slice of Builder.
func BuildersRoot(stateVersion int, slice []*ethpb.Builder) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
		return buildersRootProgressive(slice)
	}
	return ssz.SliceRoot(slice, uint64(fieldparams.BuilderRegistryLimit))
}

// buildersRootProgressive computes the progressive SSZ root of a slice of Builder.
func buildersRootProgressive(slice []*ethpb.Builder) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
