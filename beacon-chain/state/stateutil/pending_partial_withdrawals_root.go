package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func PendingPartialWithdrawalsRoot(stateVersion int, slice []*ethpb.PendingPartialWithdrawal) ([32]byte, error) {
	if stateVersion >= version.Gloas {
		return pendingPartialWithdrawalsRootProgressive(slice)
	}
	return pendingPartialWithdrawalsRoot(slice)
}

func pendingPartialWithdrawalsRoot(slice []*ethpb.PendingPartialWithdrawal) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingPartialWithdrawalsLimit)
}

func pendingPartialWithdrawalsRootProgressive(slice []*ethpb.PendingPartialWithdrawal) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
