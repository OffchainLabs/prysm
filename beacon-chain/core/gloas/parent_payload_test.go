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

	require.NoError(t, ValidateExecutionRequestLengths(reqs))
}

func TestValidateExecutionRequestLengths_WithdrawalsBounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		Withdrawals: make([]*enginev1.WithdrawalRequest, int(cfg.MaxWithdrawalRequestsPerPayload)+1),
	}

	require.ErrorContains(t, "too many withdrawal requests", ValidateExecutionRequestLengths(reqs))
}

func TestValidateExecutionRequestLengths_ConsolidationsBounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		Consolidations: make([]*enginev1.ConsolidationRequest, int(cfg.MaxConsolidationsRequestsPerPayload)+1),
	}

	require.ErrorContains(t, "too many consolidation requests", ValidateExecutionRequestLengths(reqs))
}

func TestValidateExecutionRequestLengths_BuilderDepositsBounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		BuilderDeposits: make([]*enginev1.BuilderDepositRequest, int(cfg.MaxBuilderDepositRequestsPerPayload)+1),
	}

	require.ErrorContains(t, "too many builder deposit requests", ValidateExecutionRequestLengths(reqs))
}

func TestValidateExecutionRequestLengths_BuilderExitsBounded(t *testing.T) {
	cfg := params.BeaconConfig()
	reqs := &enginev1.ExecutionRequestsGloas{
		BuilderExits: make([]*enginev1.BuilderExitRequest, int(cfg.MaxBuilderExitRequestsPerPayload)+1),
	}

	require.ErrorContains(t, "too many builder exit requests", ValidateExecutionRequestLengths(reqs))
}
