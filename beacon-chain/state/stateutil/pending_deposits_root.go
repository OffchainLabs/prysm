package stateutil

import (
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func PendingDepositsRoot(slice []*ethpb.PendingDeposit) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.PendingDepositsLimit)
}
