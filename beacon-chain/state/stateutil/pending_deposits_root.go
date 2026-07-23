package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func PendingDepositsRoot(stateVersion int, slice []*ethpb.PendingDeposit) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
		return pendingDepositsRootProgressive(slice)
	}
	return pendingDepositsRoot(slice)
}

func pendingDepositsRoot(slice []*ethpb.PendingDeposit) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingDepositsLimit)
}

func pendingDepositsRootProgressive(slice []*ethpb.PendingDeposit) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
