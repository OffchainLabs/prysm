package stateutil

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuildersRoot computes the SSZ root of a slice of Builder.
func BuildersRoot(slice []*ethpb.Builder) ([32]byte, error) {
	return ssz.SliceRoot(slice, uint64(fieldparams.BuilderRegistryLimit))
}

// BuildersRootProgressive computes the progressive SSZ root of a slice of Builder.
func BuildersRootProgressive(slice []*ethpb.Builder) ([32]byte, error) {
	if uint64(len(slice)) > uint64(fieldparams.BuilderRegistryLimit) {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", fieldparams.BuilderRegistryLimit)
	}
	return ssz.SliceRootProgressive(slice)
}
