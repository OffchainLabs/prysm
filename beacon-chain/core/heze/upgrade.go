package heze

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToHeze upgrades a Gloas state to Heze and rebuilds latest_execution_payload_bid with zero inclusion_list_bits.
// TODO(heze): The eth1 fields stay until EIP-8015.
func UpgradeToHeze(beaconState state.BeaconState) (state.BeaconState, error) {
	pre, err := state_native.ProtobufBeaconStateGloas(beaconState.ToProto())
	if err != nil {
		return nil, errors.Wrap(err, "could not convert state to Gloas protobuf")
	}
	bid := pre.LatestExecutionPayloadBid
	if bid == nil {
		return nil, errors.New("nil latest execution payload bid")
	}

	s := &ethpb.BeaconStateHeze{
		GenesisTime:           pre.GenesisTime,
		GenesisValidatorsRoot: pre.GenesisValidatorsRoot,
		Slot:                  pre.Slot,
		Fork: &ethpb.Fork{
			PreviousVersion: pre.Fork.CurrentVersion,
			CurrentVersion:  params.BeaconConfig().HezeForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:             pre.LatestBlockHeader,
		BlockRoots:                    pre.BlockRoots,
		StateRoots:                    pre.StateRoots,
		HistoricalRoots:               pre.HistoricalRoots,
		Eth1Data:                      pre.Eth1Data,
		Eth1DataVotes:                 pre.Eth1DataVotes,
		Eth1DepositIndex:              pre.Eth1DepositIndex,
		Validators:                    pre.Validators,
		Balances:                      pre.Balances,
		RandaoMixes:                   pre.RandaoMixes,
		Slashings:                     pre.Slashings,
		PreviousEpochParticipation:    pre.PreviousEpochParticipation,
		CurrentEpochParticipation:     pre.CurrentEpochParticipation,
		JustificationBits:             pre.JustificationBits,
		PreviousJustifiedCheckpoint:   pre.PreviousJustifiedCheckpoint,
		CurrentJustifiedCheckpoint:    pre.CurrentJustifiedCheckpoint,
		FinalizedCheckpoint:           pre.FinalizedCheckpoint,
		InactivityScores:              pre.InactivityScores,
		CurrentSyncCommittee:          pre.CurrentSyncCommittee,
		NextSyncCommittee:             pre.NextSyncCommittee,
		LatestBlockHash:               pre.LatestBlockHash,
		NextWithdrawalIndex:           pre.NextWithdrawalIndex,
		NextWithdrawalValidatorIndex:  pre.NextWithdrawalValidatorIndex,
		HistoricalSummaries:           pre.HistoricalSummaries,
		DepositRequestsStartIndex:     pre.DepositRequestsStartIndex,
		DepositBalanceToConsume:       pre.DepositBalanceToConsume,
		ExitBalanceToConsume:          pre.ExitBalanceToConsume,
		EarliestExitEpoch:             pre.EarliestExitEpoch,
		ConsolidationBalanceToConsume: pre.ConsolidationBalanceToConsume,
		EarliestConsolidationEpoch:    pre.EarliestConsolidationEpoch,
		PendingDeposits:               pre.PendingDeposits,
		PendingPartialWithdrawals:     pre.PendingPartialWithdrawals,
		PendingConsolidations:         pre.PendingConsolidations,
		ProposerLookahead:             pre.ProposerLookahead,
		Builders:                      pre.Builders,
		NextWithdrawalBuilderIndex:    pre.NextWithdrawalBuilderIndex,
		ExecutionPayloadAvailability:  pre.ExecutionPayloadAvailability,
		BuilderPendingPayments:        pre.BuilderPendingPayments,
		BuilderPendingWithdrawals:     pre.BuilderPendingWithdrawals,
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       bid.ParentBlockHash,
			ParentBlockRoot:       bid.ParentBlockRoot,
			BlockHash:             bid.BlockHash,
			PrevRandao:            bid.PrevRandao,
			FeeRecipient:          bid.FeeRecipient,
			GasLimit:              bid.GasLimit,
			BuilderIndex:          bid.BuilderIndex,
			Slot:                  bid.Slot,
			Value:                 bid.Value,
			ExecutionPayment:      bid.ExecutionPayment,
			BlobKzgCommitments:    bid.BlobKzgCommitments,
			ExecutionRequestsRoot: bid.ExecutionRequestsRoot,
			InclusionListBits:     bitfield.NewBitvector16(),
		},
		PayloadExpectedWithdrawals: pre.PayloadExpectedWithdrawals,
		PtcWindow:                  pre.PtcWindow,
	}
	return state_native.InitializeFromProtoUnsafeHeze(s)
}
