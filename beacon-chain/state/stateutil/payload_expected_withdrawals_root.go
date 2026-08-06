package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/wrappers"
)

// PayloadExpectedWithdrawalsRoot computes the SSZ root of a slice of Withdrawals.
func PayloadExpectedWithdrawalsRoot(stateVersion int, withdrawals []*enginev1.Withdrawal) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
		return wrappers.WithdrawalSliceRootProgressive(withdrawals)
	}
	return wrappers.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
}
