package gloas

import (
	"bytes"
	"context"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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

func TestApplyParentExecutionPayload_SettlesPaymentBeforeBuilderExit(t *testing.T) {
	cfg := params.BeaconConfig()
	addr := bytes.Repeat([]byte{0x44}, 20)
	builder, _ := activeBuilder(t, addr)
	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Slot:                         2 * cfg.SlotsPerEpoch,
		DepositRequestsStartIndex:    cfg.UnsetDepositRequestsStartIndex,
		ExecutionPayloadAvailability: make([]byte, cfg.SlotsPerHistoricalRoot/8),
		Builders:                     []*ethpb.Builder{builder},
		FinalizedCheckpoint:          &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             bytes.Repeat([]byte{0x01}, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          addr,
			Value:                 10,
			ExecutionRequestsRoot: make([]byte, 32),
		},
	})
	require.NoError(t, err)

	reqs := &enginev1.ExecutionRequestsGloas{
		BuilderExits: []*enginev1.BuilderExitRequest{{SourceAddress: addr, Pubkey: builder.Pubkey}},
	}
	require.NoError(t, ApplyParentExecutionPayload(context.Background(), st, reqs))

	withdrawals, err := st.BuilderPendingWithdrawals()
	require.NoError(t, err)
	require.Equal(t, 1, len(withdrawals))
	require.Equal(t, primitives.Gwei(10), withdrawals[0].Amount)

	got, err := st.Builder(0)
	require.NoError(t, err)
	require.Equal(t, cfg.FarFutureEpoch, got.WithdrawableEpoch)
}
