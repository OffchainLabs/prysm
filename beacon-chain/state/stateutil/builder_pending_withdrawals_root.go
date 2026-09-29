package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// BuilderPendingWithdrawalsRoot computes the SSZ root of a slice of BuilderPendingWithdrawal.
func BuilderPendingWithdrawalsRoot(stateVersion int, slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	if stateVersion >= version.Gloas {
		return builderPendingWithdrawalsRootProgressive(slice)
	}
	return ssz.SliceRoot(slice, fieldparams.BuilderPendingWithdrawalsLimit)
}

// builderPendingWithdrawalsRootProgressive computes the progressive SSZ root of
// a slice of BuilderPendingWithdrawal.
func builderPendingWithdrawalsRootProgressive(slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
