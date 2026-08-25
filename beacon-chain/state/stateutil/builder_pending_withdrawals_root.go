package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuilderPendingWithdrawalsRoot computes the SSZ root of a slice of BuilderPendingWithdrawal.
func BuilderPendingWithdrawalsRoot(stateVersion int, slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
		return builderPendingWithdrawalsRootProgressive(slice)
	}
	return ssz.SliceRoot(slice, fieldparams.BuilderPendingWithdrawalsLimit)
}

// builderPendingWithdrawalsRootProgressive computes the progressive SSZ root of
// a slice of BuilderPendingWithdrawal.
func builderPendingWithdrawalsRootProgressive(slice []*ethpb.BuilderPendingWithdrawal) ([32]byte, error) {
	return ssz.SliceRootProgressive(slice)
}
