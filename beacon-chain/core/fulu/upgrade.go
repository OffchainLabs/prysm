package fulu

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// UpgradeToFulu updates inputs a generic state to return the version Fulu state.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/fork.md#upgrading-the-state
func UpgradeToFulu(ctx context.Context, beaconState state.BeaconState) (state.BeaconState, error) {
	s, err := ConvertToFulu(beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert to fulu")
	}
	proposerLookahead, err := helpers.InitializeProposerLookahead(ctx, beaconState, slots.ToEpoch(beaconState.Slot()))
	if err != nil {
		return nil, err
	}
	pl := make([]primitives.ValidatorIndex, len(proposerLookahead))
	for i, v := range proposerLookahead {
		pl[i] = primitives.ValidatorIndex(v)
	}
	if err := s.SetProposerLookahead(pl); err != nil {
		return nil, errors.Wrap(err, "failed to set proposer lookahead")
	}
	return s, nil
}

func ConvertToFulu(beaconState state.BeaconState) (state.BeaconState, error) {
	currentSyncCommittee, err := beaconState.CurrentSyncCommittee()
	if err != nil {
		return nil, err
	}
	nextSyncCommittee, err := beaconState.NextSyncCommittee()
	if err != nil {
		return nil, err
	}
	prevEpochParticipation, err := beaconState.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	currentEpochParticipation, err := beaconState.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	inactivityScores, err := beaconState.InactivityScores()
	if err != nil {
		return nil, err
	}
	payloadHeader, err := beaconState.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	txRoot, err := payloadHeader.TransactionsRoot()
	if err != nil {
		return nil, err
	}
	wdRoot, err := payloadHeader.WithdrawalsRoot()
	if err != nil {
		return nil, err
	}
	wi, err := beaconState.NextWithdrawalIndex()
	if err != nil {
		return nil, err
	}
	vi, err := beaconState.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, err
	}
	summaries, err := beaconState.HistoricalSummaries()
	if err != nil {
		return nil, err
	}
	excessBlobGas, err := payloadHeader.ExcessBlobGas()
	if err != nil {
		return nil, err
	}
	blobGasUsed, err := payloadHeader.BlobGasUsed()
	if err != nil {
		return nil, err
	}
	depositRequestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return nil, err
	}
	depositBalanceToConsume, err := beaconState.DepositBalanceToConsume()
	if err != nil {
		return nil, err
	}
	exitBalanceToConsume, err := beaconState.ExitBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestExitEpoch, err := beaconState.EarliestExitEpoch()
	if err != nil {
		return nil, err
	}
	consolidationBalanceToConsume, err := beaconState.ConsolidationBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestConsolidationEpoch, err := beaconState.EarliestConsolidationEpoch()
	if err != nil {
		return nil, err
	}
	pendingDeposits, err := beaconState.PendingDeposits()
	if err != nil {
		return nil, err
	}
	pendingPartialWithdrawals, err := beaconState.PendingPartialWithdrawals()
	if err != nil {
		return nil, err
	}
	pendingConsolidations, err := beaconState.PendingConsolidations()
	if err != nil {
		return nil, err
	}
	s := &ethpb.BeaconStateFulu{
		GenesisTime:           uint64(beaconState.GenesisTime().Unix()),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().FuluForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:           beaconState.LatestBlockHeader(),
		BlockRoots:                  beaconState.BlockRoots(),
		StateRoots:                  beaconState.StateRoots(),
		HistoricalRoots:             beaconState.HistoricalRoots(),
		Eth1Data:                    beaconState.Eth1Data(),
		Eth1DataVotes:               beaconState.Eth1DataVotes(),
		Eth1DepositIndex:            beaconState.Eth1DepositIndex(),
		Validators:                  beaconState.Validators(),
		Balances:                    beaconState.Balances(),
		RandaoMixes:                 beaconState.RandaoMixes(),
		Slashings:                   beaconState.Slashings(),
		PreviousEpochParticipation:  prevEpochParticipation,
		CurrentEpochParticipation:   currentEpochParticipation,
		JustificationBits:           beaconState.JustificationBits(),
		PreviousJustifiedCheckpoint: beaconState.PreviousJustifiedCheckpoint(),
		CurrentJustifiedCheckpoint:  beaconState.CurrentJustifiedCheckpoint(),
		FinalizedCheckpoint:         beaconState.FinalizedCheckpoint(),
		InactivityScores:            inactivityScores,
		CurrentSyncCommittee:        currentSyncCommittee,
		NextSyncCommittee:           nextSyncCommittee,
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{
			ParentHash:       payloadHeader.ParentHash(),
			FeeRecipient:     payloadHeader.FeeRecipient(),
			StateRoot:        payloadHeader.StateRoot(),
			ReceiptsRoot:     payloadHeader.ReceiptsRoot(),
			LogsBloom:        payloadHeader.LogsBloom(),
			PrevRandao:       payloadHeader.PrevRandao(),
			BlockNumber:      payloadHeader.BlockNumber(),
			GasLimit:         payloadHeader.GasLimit(),
			GasUsed:          payloadHeader.GasUsed(),
			Timestamp:        payloadHeader.Timestamp(),
			ExtraData:        payloadHeader.ExtraData(),
			BaseFeePerGas:    payloadHeader.BaseFeePerGas(),
			BlockHash:        payloadHeader.BlockHash(),
			TransactionsRoot: txRoot,
			WithdrawalsRoot:  wdRoot,
			ExcessBlobGas:    excessBlobGas,
			BlobGasUsed:      blobGasUsed,
		},
		NextWithdrawalIndex:          wi,
		NextWithdrawalValidatorIndex: vi,
		HistoricalSummaries:          summaries,

		DepositRequestsStartIndex:     depositRequestsStartIndex,
		DepositBalanceToConsume:       depositBalanceToConsume,
		ExitBalanceToConsume:          exitBalanceToConsume,
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: consolidationBalanceToConsume,
		EarliestConsolidationEpoch:    earliestConsolidationEpoch,
		PendingDeposits:               pendingDeposits,
		PendingPartialWithdrawals:     pendingPartialWithdrawals,
		PendingConsolidations:         pendingConsolidations,
	}
	return state_native.InitializeFromProtoUnsafeFulu(s)
}
