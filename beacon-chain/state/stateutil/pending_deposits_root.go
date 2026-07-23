package stateutil

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func PendingDepositsRoot(slice []*ethpb.PendingDeposit) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingDepositsLimit)
}

func PendingDepositsRootProgressive(slice []*ethpb.PendingDeposit) ([32]byte, error) {
	if uint64(len(slice)) > fieldparams.PendingDepositsLimit {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", fieldparams.PendingDepositsLimit)
	}
	return ssz.SliceRootProgressive(slice)
}
