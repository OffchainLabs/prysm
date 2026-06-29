package stateutil

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func PendingPartialWithdrawalsRoot(slice []*ethpb.PendingPartialWithdrawal) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingPartialWithdrawalsLimit)
}

func PendingPartialWithdrawalsRootProgressive(slice []*ethpb.PendingPartialWithdrawal) ([32]byte, error) {
	if uint64(len(slice)) > fieldparams.PendingPartialWithdrawalsLimit {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", fieldparams.PendingPartialWithdrawalsLimit)
	}
	return ssz.SliceRootProgressive(slice)
}
