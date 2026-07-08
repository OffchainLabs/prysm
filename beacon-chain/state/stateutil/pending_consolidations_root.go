package stateutil

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func PendingConsolidationsRoot(slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingConsolidationsLimit)
}

func PendingConsolidationsRootProgressive(slice []*ethpb.PendingConsolidation) ([32]byte, error) {
	if uint64(len(slice)) > fieldparams.PendingConsolidationsLimit {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", fieldparams.PendingConsolidationsLimit)
	}
	return ssz.SliceRootProgressive(slice)
}
