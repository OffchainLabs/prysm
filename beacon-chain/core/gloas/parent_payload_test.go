package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestValidateExecutionRequestLengths_DepositsUnbounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		Deposits: make([]*enginev1.DepositRequest, int(cfg.MaxDepositRequestsPerPayload)+1),
	}

	require.NoError(t, validateExecutionRequestLengths(reqs))
}

func TestValidateExecutionRequestLengths_WithdrawalsBounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		Withdrawals: make([]*enginev1.WithdrawalRequest, int(cfg.MaxWithdrawalRequestsPerPayload)+1),
	}

	require.ErrorContains(t, "too many withdrawal requests", validateExecutionRequestLengths(reqs))
}
