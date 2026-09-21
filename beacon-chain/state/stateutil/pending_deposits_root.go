package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func PendingDepositsRoot(stateVersion int, slice []*ethpb.PendingDeposit) ([32]byte, error) {
	if stateVersion >= version.Gloas {
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
