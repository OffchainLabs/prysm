package stateutil

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuilderPendingWithdrawalsRoot computes the SSZ root of a slice of BuilderPendingWithdrawal.
func BuilderPendingWithdrawalsRoot(slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	return ssz.SliceRoot(slice, fieldparams.BuilderPendingWithdrawalsLimit)
}

// BuilderPendingWithdrawalsRootProgressive computes the progressive SSZ root of
// a slice of BuilderPendingWithdrawal.
func BuilderPendingWithdrawalsRootProgressive(slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	if uint64(len(slice)) > fieldparams.BuilderPendingWithdrawalsLimit {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", fieldparams.BuilderPendingWithdrawalsLimit)
	}
	return ssz.SliceRootProgressive(slice)
}
