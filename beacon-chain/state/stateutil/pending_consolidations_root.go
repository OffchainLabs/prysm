package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func PendingConsolidationsRoot(stateVersion int, slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
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
