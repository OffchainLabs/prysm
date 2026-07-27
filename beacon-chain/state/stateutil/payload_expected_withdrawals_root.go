package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// PayloadExpectedWithdrawalsRoot computes the SSZ root of a slice of Withdrawals.
func PayloadExpectedWithdrawalsRoot(stateVersion int, withdrawals []*enginev1.Withdrawal) ([32]byte, error) {
	if features.ProgressiveSSZEnabled(stateVersion) {
		return ssz.WithdrawalSliceRootProgressive(withdrawals)
	}
	return ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
}
