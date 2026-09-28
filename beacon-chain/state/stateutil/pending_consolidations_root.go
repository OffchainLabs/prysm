package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func PendingConsolidationsRoot(stateVersion int, slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	if stateVersion >= version.Gloas {
		return pendingConsolidationsRootProgressive(slice)
	}
	return pendingConsolidationsRoot(slice)
}

func pendingConsolidationsRoot(slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingConsolidationsLimit)
}

func pendingConsolidationsRootProgressive(slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
